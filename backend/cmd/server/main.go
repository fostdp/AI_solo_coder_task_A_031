package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	influxdb2 "sewage-plant-system/pkg/influxdb"
	"sewage-plant-system/pkg/models"
	mqtt2 "sewage-plant-system/pkg/mqtt"
	ws "sewage-plant-system/pkg/websocket"

	ac "sewage-plant-system/pkg/aeration_controller"
	alarm "sewage-plant-system/pkg/alarm_router"
	co "sewage-plant-system/pkg/carbon_optimizer"
	sc "sewage-plant-system/pkg/sensor_collector"
)

type Server struct {
	influxClient    *influxdb2.Client
	mqttClient      *mqtt2.Client
	wsServer        *ws.Server
	ginEngine       *gin.Engine
	httpServer      *http.Server

	sensorCollector *sc.SensorCollector
	aerationCtrl    *ac.AerationController
	carbonOpt       *co.CarbonOptimizer
	alarmRouter     *alarm.AlarmRouter

	sensorConfigs   []*models.SensorConfig
	processSections []*models.ProcessSection
	flowRate        float64

	validDataChan      chan *sc.ValidatedSensorData
	sensorStatusChan   chan *sc.SensorStatusEvent
	aerationCmdChan    chan *ac.ControlOutput
	carbonCmdChan      chan *co.ControlOutput
	plcStatusChan      chan *models.PLCStatus
	alertOutChan       chan *models.Alert
	aerationStatusChan chan map[string]interface{}
	carbonStatusChan   chan *co.Status
	alertStatusChan    chan map[string]interface{}
}

func main() {
	if err := loadConfig(); err != nil {
		log.Printf("Warning: Failed to load config: %v", err)
	}

	server := &Server{
		flowRate: 300000,
	}

	if err := server.init(); err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}

	server.setupRoutes()

	if err := server.start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}

	server.waitForShutdown()
}

func loadConfig() error {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("../")
	viper.AddConfigPath("/etc/sewage-plant/")
	return viper.ReadInConfig()
}

func (s *Server) init() error {
	var err error

	s.sensorConfigs = models.GenerateSensorConfigs()
	s.processSections = models.GetProcessSections()

	s.initChannels()

	if err = s.initInfluxDB(); err != nil {
		log.Printf("Warning: %v", err)
	}

	s.initWebSocket()
	s.initMQTT()
	s.initModules()
	s.initCoordinators()
	s.initGinEngine()

	log.Println("Server initialized successfully")
	return nil
}

func (s *Server) initChannels() {
	s.validDataChan = make(chan *sc.ValidatedSensorData, 1000)
	s.sensorStatusChan = make(chan *sc.SensorStatusEvent, 100)
	s.aerationCmdChan = make(chan *ac.ControlOutput, 100)
	s.carbonCmdChan = make(chan *co.ControlOutput, 100)
	s.plcStatusChan = make(chan *models.PLCStatus, 100)
	s.alertOutChan = make(chan *models.Alert, 100)
	s.aerationStatusChan = make(chan map[string]interface{}, 10)
	s.carbonStatusChan = make(chan *co.Status, 10)
	s.alertStatusChan = make(chan map[string]interface{}, 10)
}

func (s *Server) initInfluxDB() error {
	influxAddr := viper.GetString("influxdb.addr")
	if influxAddr == "" {
		influxAddr = "http://localhost:8086"
	}

	var err error
	s.influxClient, err = influxdb2.New(
		influxAddr,
		viper.GetString("influxdb.username"),
		viper.GetString("influxdb.password"),
		viper.GetString("influxdb.database"),
		viper.GetString("influxdb.retention_policy"),
		10*time.Second,
	)
	if err != nil {
		return fmt.Errorf("failed to connect to InfluxDB: %v, continuing without InfluxDB connection", err)
	}
	return nil
}

func (s *Server) initWebSocket() {
	s.wsServer = ws.NewServer()
}

func (s *Server) initMQTT() {
	mqttBroker := viper.GetString("mqtt.broker")
	if mqttBroker == "" {
		mqttBroker = "tcp://localhost:1883"
	}

	s.mqttClient = mqtt2.New(
		mqttBroker,
		viper.GetString("mqtt.client_id"),
		viper.GetString("mqtt.username"),
		viper.GetString("mqtt.password"),
	)

	if err := s.mqttClient.Connect(); err != nil {
		log.Printf("Warning: Failed to connect to MQTT broker: %v, continuing without MQTT connection", err)
	} else {
		s.setupMQTTSubscriptions()
	}
}

func (s *Server) initModules() {
	scCfg := sc.Config{
		OfflineTimeout:    viper.GetDuration("sensor_collector.offline_timeout"),
		MaxDeviationRatio: viper.GetFloat64("sensor_collector.max_deviation_ratio"),
		SensorConfigs:     s.sensorConfigs,
	}
	if scCfg.OfflineTimeout == 0 {
		scCfg.OfflineTimeout = 5 * time.Minute
	}
	if scCfg.MaxDeviationRatio == 0 {
		scCfg.MaxDeviationRatio = 10.0
	}
	s.sensorCollector = sc.New(scCfg, s.validDataChan, s.sensorStatusChan)

	var acCfg ac.Config
	if err := viper.UnmarshalKey("control.aeration", &acCfg); err != nil {
		log.Printf("Warning: Failed to unmarshal aeration config: %v, using defaults", err)
	}
	s.aerationCtrl = ac.New(acCfg, s.validDataChan, s.aerationCmdChan, s.aerationStatusChan)
	s.aerationCtrl.SetFlowRate(s.flowRate)

	var coCfg co.CarbonConfig
	if err := viper.UnmarshalKey("control.carbon", &coCfg); err != nil {
		log.Printf("Warning: Failed to unmarshal carbon config: %v, using defaults", err)
	}
	s.carbonOpt = co.New(coCfg, s.validDataChan, s.sensorStatusChan, s.carbonCmdChan, s.carbonStatusChan)
	s.carbonOpt.SetFlowRate(s.flowRate)

	var alarmCfg alarm.Config
	if err := viper.UnmarshalKey("alert", &alarmCfg); err != nil {
		log.Printf("Warning: Failed to unmarshal alert config: %v, using defaults", err)
	}
	s.alarmRouter = alarm.New(alarmCfg, s.wsServer, s.validDataChan, s.sensorStatusChan, s.plcStatusChan, s.alertOutChan, s.alertStatusChan)
}

func (s *Server) initCoordinators() {
	s.aerationCtrl.Start()
	s.carbonOpt.Start()
	s.alarmRouter.Start()

	go s.coordinateAerationCommands()
	go s.coordinateCarbonCommands()
	go s.coordinateAlerts()
	go s.coordinateSensorStatus()
	go s.startKPICalculator()
	go s.startStatusBroadcaster()
}

func (s *Server) initGinEngine() {
	s.ginEngine = gin.Default()

	s.ginEngine.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	port := viper.GetInt("server.port")
	if port == 0 {
		port = 8080
	}

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: s.ginEngine,
	}
}

func (s *Server) setupMQTTSubscriptions() {
	s.mqttClient.SetHandler("sensor/+/data", func(topic string, payload []byte) {
		data, err := s.mqttClient.ParseSensorData(payload)
		if err != nil {
			log.Printf("Failed to parse sensor data: %v", err)
			return
		}
		s.sensorCollector.ProcessSensorData(data)

		if s.influxClient != nil {
			if err := s.influxClient.WriteSensorData(data); err != nil {
				log.Printf("Failed to write sensor data to InfluxDB: %v", err)
			}
		}

		s.wsServer.BroadcastSensorData(data)
	})

	s.mqttClient.SetHandler("plc/+/status", func(topic string, payload []byte) {
		status, err := s.mqttClient.ParsePLCStatus(payload)
		if err != nil {
			log.Printf("Failed to parse PLC status: %v", err)
			return
		}

		if s.influxClient != nil {
			if err := s.influxClient.WritePLCStatus(status); err != nil {
				log.Printf("Failed to write PLC status to InfluxDB: %v", err)
			}
		}

		select {
		case s.plcStatusChan <- status:
		default:
		}

		s.wsServer.BroadcastPLCStatus(status)
	})

	if err := s.mqttClient.Subscribe("sensor/+/data", 1); err != nil {
		log.Printf("Failed to subscribe to sensor data: %v", err)
	}

	if err := s.mqttClient.Subscribe("plc/+/status", 1); err != nil {
		log.Printf("Failed to subscribe to PLC status: %v", err)
	}
}

func (s *Server) coordinateAerationCommands() {
	for cmd := range s.aerationCmdChan {
		controlCmd := cmd.Command

		if s.mqttClient != nil && s.mqttClient.IsConnected() {
			if err := s.mqttClient.PublishControlCommand(controlCmd); err != nil {
				log.Printf("Failed to publish aeration control command: %v", err)
			}
		}

		if s.influxClient != nil {
			if err := s.influxClient.WriteControlCommand(controlCmd); err != nil {
				log.Printf("Failed to write aeration control command: %v", err)
			}

			zoneID := cmd.ZoneID
			if zone := s.aerationCtrl.GetAllZones()[zoneID]; zone != nil {
				acData := &models.AerationControl{
					ZoneID:            zoneID,
					DOActual:          zone.DOActual,
					DOSetpoint:        zone.DOSetpoint,
					NH3Actual:         zone.NH3Actual,
					NH3Setpoint:       zone.NH3Setpoint,
					AirFlowSetpoint:   zone.AirFlowSetpoint,
					ValveOpening:      zone.ValveOpening,
					FanSpeed:          zone.FanSpeed,
					PIDOutput:         zone.PIDOutput,
					FeedforwardOutput: zone.FFOutput,
					TotalOutput:       zone.TotalOutput,
				}
				if err := s.influxClient.WriteAerationControl(acData, time.Now()); err != nil {
					log.Printf("Failed to write aeration control data: %v", err)
				}
			}
		}

		s.wsServer.BroadcastControlCommand(controlCmd)
	}
}

func (s *Server) coordinateCarbonCommands() {
	for cmd := range s.carbonCmdChan {
		controlCmd := cmd.Command

		if s.mqttClient != nil && s.mqttClient.IsConnected() {
			if err := s.mqttClient.PublishControlCommand(controlCmd); err != nil {
				log.Printf("Failed to publish carbon control command: %v", err)
			}
		}

		if s.influxClient != nil {
			if err := s.influxClient.WriteControlCommand(controlCmd); err != nil {
				log.Printf("Failed to write carbon control command: %v", err)
			}

			carbonStatus := s.carbonOpt.GetStatus()
			ccData := &models.CarbonControl{
				NO3Actual:        carbonStatus.NO3Actual,
				CODInfluent:      carbonStatus.CODInfluent,
				TNEstimate:       carbonStatus.TNEffluent,
				DosageSetpoint:   carbonStatus.DosageSetpoint,
				CarbonSourceType: carbonStatus.CarbonSourceType,
				RemovalRate:      carbonStatus.TNRemovalRate,
			}
			if err := s.influxClient.WriteCarbonControl(ccData, time.Now()); err != nil {
				log.Printf("Failed to write carbon control data: %v", err)
			}
		}

		s.wsServer.BroadcastControlCommand(controlCmd)

		if status := s.carbonOpt.GetStatus(); status != nil {
			s.wsServer.BroadcastCarbonControl(s.carbonStatusToMap(status))
		}
	}
}

func (s *Server) coordinateAlerts() {
	for alert := range s.alertOutChan {
		if s.influxClient != nil {
			if err := s.influxClient.WriteAlert(alert); err != nil {
				log.Printf("Failed to write alert to InfluxDB: %v", err)
			}
		}
	}
}

func (s *Server) coordinateSensorStatus() {
	for event := range s.sensorStatusChan {
		if event.EventType == "offline" {
			s.sensorCollector.CheckOffline(time.Now())
		}
	}
}

func (s *Server) startKPICalculator() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.calculateKPI()
	}
}

func (s *Server) calculateKPI() {
	energyPerTon := s.aerationCtrl.CalculateEnergyPerTon(s.flowRate)
	carbonPerTon := s.carbonOpt.CalculateCarbonPerTon(s.flowRate)

	influentNH3 := s.getMockSensorValue(models.SensorTypeNH3, "influent")
	effluentNH3 := s.getMockSensorValue(models.SensorTypeNH3, "effluent")
	influentNO3 := s.getMockSensorValue(models.SensorTypeNO3, "influent")
	effluentNO3 := s.getMockSensorValue(models.SensorTypeNO3, "effluent")
	influentPO4 := s.getMockSensorValue(models.SensorTypePO4, "influent")
	effluentPO4 := s.getMockSensorValue(models.SensorTypePO4, "effluent")

	if influentNH3 <= 0 {
		influentNH3 = 35.0
	}
	if effluentNH3 <= 0 {
		effluentNH3 = 1.5
	}
	if influentNO3 <= 0 {
		influentNO3 = 2.0
	}
	if effluentNO3 <= 0 {
		effluentNO3 = 8.0
	}
	if influentPO4 <= 0 {
		influentPO4 = 5.0
	}
	if effluentPO4 <= 0 {
		effluentPO4 = 0.3
	}

	nh3RemovalRate := (influentNH3 - effluentNH3) / influentNH3 * 100
	tnRemovalRate := ((influentNH3 + influentNO3) - (effluentNH3 + effluentNO3)) / (influentNH3 + influentNO3) * 100
	tpRemovalRate := (influentPO4 - effluentPO4) / influentPO4 * 100

	kpi := &models.KPIData{
		Timestamp:      time.Now(),
		EnergyPerTon:   energyPerTon,
		CarbonPerTon:   carbonPerTon,
		NH3RemovalRate: nh3RemovalRate,
		TNRemovalRate:  tnRemovalRate,
		TPRemovalRate:  tpRemovalRate,
		WaterQuality:   s.calculateWaterQuality(effluentNH3, effluentNO3+effluentNH3, effluentPO4),
	}

	if s.influxClient != nil {
		if err := s.influxClient.WriteKPI(kpi); err != nil {
			log.Printf("Failed to write KPI data: %v", err)
		}
	}

	s.wsServer.BroadcastKPI(kpi)
}

func (s *Server) getMockSensorValue(sensorType models.SensorType, location string) float64 {
	if s.influxClient == nil {
		switch sensorType {
		case models.SensorTypeDO:
			return 2.0 + (randFloat()-0.5)*0.5
		case models.SensorTypeNH3:
			return 1.5 + (randFloat()-0.5)*0.8
		case models.SensorTypeNO3:
			return 8.0 + (randFloat()-0.5)*2.0
		case models.SensorTypePO4:
			return 0.5 + (randFloat()-0.5)*0.2
		case models.SensorTypeCOD:
			return 300.0 + (randFloat()-0.5)*50
		}
		return 0
	}

	start := time.Now().Add(-5 * time.Minute)
	end := time.Now()

	trends, err := s.influxClient.QuerySensorsByType(sensorType, start, end)
	if err != nil || len(trends) == 0 {
		return 0
	}

	var sum float64
	var count int

	for sensorID, trend := range trends {
		if len(trend.Values) == 0 {
			continue
		}

		if s.sensorCollector.SensorBelongsToLocation(sensorID, location) {
			lastValue := trend.Values[len(trend.Values)-1]
			sum += lastValue
			count++
		}
	}

	if count == 0 {
		return 0
	}

	return sum / float64(count)
}

func (s *Server) calculateWaterQuality(nh3, tn, tp float64) float64 {
	score := 100.0

	if nh3 > 5 {
		score -= 30
	} else if nh3 > 2 {
		score -= 15
	}

	if tn > 15 {
		score -= 25
	} else if tn > 10 {
		score -= 10
	}

	if tp > 0.5 {
		score -= 20
	} else if tp > 0.3 {
		score -= 10
	}

	if score < 0 {
		score = 0
	}

	return score
}

func (s *Server) startStatusBroadcaster() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		aerationStatus := s.aerationCtrl.GetStatus()
		carbonStatus := s.carbonOpt.GetStatus()
		alertStatus := s.alarmRouter.GetStatus()

		status := map[string]interface{}{
			"timestamp":       time.Now(),
			"alert_status":    alertStatus,
			"aeration_status": aerationStatus,
			"carbon_status":   s.carbonStatusToMap(carbonStatus),
			"flow_rate":       s.flowRate,
			"ws_clients":      s.wsServer.GetClientCount(),
			"mqtt_connected":  s.mqttClient.IsConnected(),
		}
		s.wsServer.BroadcastSystemStatus(status)

		if len(aerationStatus) > 0 {
			s.wsServer.BroadcastAerationControl(aerationStatus)
		}
	}
}

func (s *Server) carbonStatusToMap(status *co.Status) map[string]interface{} {
	if status == nil {
		return nil
	}
	return map[string]interface{}{
		"no3_actual":                    status.NO3Actual,
		"no3_setpoint":                  status.NO3Setpoint,
		"cod_influent":                  status.CODInfluent,
		"last_cod_influent":             status.LastCODInfluent,
		"cod_required":                  status.CODRequired,
		"cod_available":                 status.CODAvailable,
		"tn_influent":                   status.TNInfluent,
		"tn_effluent":                   status.TNEffluent,
		"tn_removal_rate":               status.TNRemovalRate,
		"dosage_setpoint":               status.DosageSetpoint,
		"carbon_source_type":            status.CarbonSourceType,
		"carbon_source_concentration":   status.CarbonSourceConcentration,
		"last_update":                   status.LastUpdate,
		"last_calculation_time":         status.LastCalculationTime,
		"last_significant_cod_change":   status.LastCODSignificantChange,
	}
}

func (s *Server) setupRoutes() {
	api := s.ginEngine.Group("/api")

	api.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
			"time":   time.Now(),
		})
	})

	api.GET("/sensors", func(c *gin.Context) {
		c.JSON(200, s.sensorConfigs)
	})

	api.GET("/sensors/:id/trend", func(c *gin.Context) {
		sensorID := c.Param("id")
		hours := 6
		if h := c.Query("hours"); h != "" {
			if val, err := strconv.Atoi(h); err == nil {
				hours = val
			}
		}

		if s.influxClient == nil {
			c.JSON(200, s.generateMockTrend(sensorID, hours))
			return
		}

		start := time.Now().Add(-time.Duration(hours) * time.Hour)
		end := time.Now()

		trend, err := s.influxClient.QuerySensorTrend(sensorID, start, end)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, trend)
	})

	api.GET("/sensors/:id/current", func(c *gin.Context) {
		sensorID := c.Param("id")

		if s.influxClient == nil {
			c.JSON(200, gin.H{
				"value":     s.getMockValue(sensorID),
				"timestamp": time.Now(),
			})
			return
		}

		value, timestamp, err := s.influxClient.QueryLatestSensorData(sensorID)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{
			"value":     value,
			"timestamp": timestamp,
		})
	})

	api.GET("/process-sections", func(c *gin.Context) {
		c.JSON(200, s.processSections)
	})

	api.GET("/biological-profile", func(c *gin.Context) {
		c.JSON(200, models.GenerateBiologicalTankProfile())
	})

	api.GET("/aeration-control", func(c *gin.Context) {
		c.JSON(200, s.aerationCtrl.GetStatus())
	})

	api.GET("/carbon-control", func(c *gin.Context) {
		c.JSON(200, s.carbonStatusToMap(s.carbonOpt.GetStatus()))
	})

	api.GET("/alerts", func(c *gin.Context) {
		level := 0
		if l := c.Query("level"); l != "" {
			if val, err := strconv.Atoi(l); err == nil {
				level = val
			}
		}

		start := time.Now().Add(-24 * time.Hour)
		end := time.Now()

		if s.influxClient == nil {
			c.JSON(200, s.alarmRouter.GetAlertHistory(100))
			return
		}

		alerts, err := s.influxClient.QueryAlerts(start, end, level)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, alerts)
	})

	api.GET("/alerts/active", func(c *gin.Context) {
		c.JSON(200, s.alarmRouter.GetActiveAlerts())
	})

	api.POST("/alerts/:id/acknowledge", func(c *gin.Context) {
		alertID := c.Param("id")
		if s.alarmRouter.AcknowledgeAlert(alertID) {
			c.JSON(200, gin.H{"status": "ok"})
		} else {
			c.JSON(404, gin.H{"error": "alert not found"})
		}
	})

	api.GET("/kpi", func(c *gin.Context) {
		hours := 24
		if h := c.Query("hours"); h != "" {
			if val, err := strconv.Atoi(h); err == nil {
				hours = val
			}
		}

		if s.influxClient == nil {
			c.JSON(200, s.generateMockKPI(hours))
			return
		}

		start := time.Now().Add(-time.Duration(hours) * time.Hour)
		end := time.Now()

		kpis, err := s.influxClient.QueryKPI(start, end)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, kpis)
	})

	api.GET("/status", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"timestamp":       time.Now(),
			"alert_status":    s.alarmRouter.GetStatus(),
			"aeration_status": s.aerationCtrl.GetStatus(),
			"carbon_status":   s.carbonStatusToMap(s.carbonOpt.GetStatus()),
			"flow_rate":       s.flowRate,
			"ws_clients":      s.wsServer.GetClientCount(),
			"mqtt_connected":  s.mqttClient.IsConnected(),
		})
	})

	s.ginEngine.GET("/ws", func(c *gin.Context) {
		s.wsServer.HandleConnection(c.Writer, c.Request)
	})

	s.ginEngine.Static("/", "./frontend")
}

func (s *Server) generateMockTrend(sensorID string, hours int) *models.TrendData {
	trend := &models.TrendData{
		Timestamps: make([]time.Time, 0),
		Values:     make([]float64, 0),
	}

	baseValue := s.getMockValue(sensorID)
	points := hours * 30

	for i := 0; i < points; i++ {
		trend.Timestamps = append(trend.Timestamps, time.Now().Add(-time.Duration(points-i)*2*time.Minute))
		variation := (randFloat() - 0.5) * baseValue * 0.2
		trend.Values = append(trend.Values, baseValue+variation)
	}

	return trend
}

func (s *Server) getMockValue(sensorID string) float64 {
	for _, cfg := range s.sensorConfigs {
		if cfg.SensorID == sensorID {
			return cfg.Setpoint + (randFloat()-0.5)*cfg.Setpoint*0.3
		}
	}
	return 0
}

func (s *Server) generateMockKPI(hours int) []*models.KPIData {
	kpis := make([]*models.KPIData, 0)
	points := hours

	for i := 0; i < points; i++ {
		kpis = append(kpis, &models.KPIData{
			Timestamp:      time.Now().Add(-time.Duration(points-i) * time.Hour),
			EnergyPerTon:   0.35 + (randFloat()-0.5)*0.1,
			CarbonPerTon:   0.25 + (randFloat()-0.5)*0.1,
			NH3RemovalRate: 92 + (randFloat()-0.5)*5,
			TNRemovalRate:  85 + (randFloat()-0.5)*5,
			TPRemovalRate:  90 + (randFloat()-0.5)*5,
			WaterQuality:   85 + (randFloat()-0.5)*10,
		})
	}

	return kpis
}

func (s *Server) start() error {
	log.Printf("Server starting on %s", s.httpServer.Addr)
	log.Printf("API: http://localhost%s", s.httpServer.Addr)
	log.Printf("WebSocket: ws://localhost%s/ws", s.httpServer.Addr)

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	s.wsServer.Start()

	return nil
}

func (s *Server) waitForShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("Shutdown signal received")

	s.stop()
}

func (s *Server) stop() {
	log.Println("Stopping server...")

	if s.httpServer != nil {
		if err := s.httpServer.Close(); err != nil {
			log.Printf("Error closing HTTP server: %v", err)
		}
	}

	if s.mqttClient != nil {
		s.mqttClient.Disconnect()
	}

	if s.wsServer != nil {
		s.wsServer.Stop()
	}

	if s.influxClient != nil {
		if err := s.influxClient.Close(); err != nil {
			log.Printf("Error closing InfluxDB client: %v", err)
		}
	}

	close(s.validDataChan)
	close(s.sensorStatusChan)
	close(s.aerationCmdChan)
	close(s.carbonCmdChan)
	close(s.plcStatusChan)
	close(s.alertOutChan)
	close(s.aerationStatusChan)
	close(s.carbonStatusChan)
	close(s.alertStatusChan)

	log.Println("Server stopped gracefully")
}

func randFloat() float64 {
	return float64(time.Now().UnixNano()%1000) / 1000.0
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	gin.SetMode(gin.ReleaseMode)
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

var _ = json.Marshal
var _ = math.Abs
