package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"sewage-plant-system/pkg/alert"
	"sewage-plant-system/pkg/control"
	influxdb2 "sewage-plant-system/pkg/influxdb"
	"sewage-plant-system/pkg/models"
	mqtt2 "sewage-plant-system/pkg/mqtt"
	ws "sewage-plant-system/pkg/websocket"
)

type Server struct {
	influxClient   *influxdb2.Client
	mqttClient     *mqtt2.Client
	wsServer       *ws.Server
	alertManager   *alert.AlertManager
	aerationCtrl   *control.AerationControlSystem
	carbonCtrl     *control.CarbonControlSystem
	carbonOptimizer *control.CarbonOptimizer
	aerationOptimizer *control.AerationOptimizer
	sensorConfigs  []*models.SensorConfig
	processSections []*models.ProcessSection
	ginEngine      *gin.Engine
	httpServer     *http.Server
	flowRate       float64
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

	go server.startSensorDataCollector()
	go server.startControlLoop()
	go server.startKPICalculator()
	go server.startAlertChecker()
	go server.startStatusBroadcaster()

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
	influxAddr := viper.GetString("influxdb.addr")
	if influxAddr == "" {
		influxAddr = "http://localhost:8086"
	}

	s.influxClient, err = influxdb2.New(
		influxAddr,
		viper.GetString("influxdb.username"),
		viper.GetString("influxdb.password"),
		viper.GetString("influxdb.database"),
		viper.GetString("influxdb.retention_policy"),
		10*time.Second,
	)
	if err != nil {
		log.Printf("Warning: Failed to connect to InfluxDB: %v", err)
		log.Println("Continuing without InfluxDB connection...")
	}

	s.wsServer = ws.NewServer()

	smsConfig := alert.SMSConfig{
		Enabled:    viper.GetBool("alert.sms.enabled"),
		APIURL:     viper.GetString("alert.sms.api_url"),
		APIKey:     viper.GetString("alert.sms.api_key"),
		Recipients: viper.GetStringSlice("alert.sms.recipients"),
	}
	s.alertManager = alert.NewAlertManager(s.wsServer, smsConfig)

	s.aerationCtrl = control.NewAerationControlSystem()
	s.carbonCtrl = control.NewCarbonControlSystem()
	s.carbonOptimizer = control.NewCarbonOptimizer()
	s.aerationOptimizer = control.NewAerationOptimizer()

	s.sensorConfigs = models.GenerateSensorConfigs()
	s.processSections = models.GetProcessSections()

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
		log.Printf("Warning: Failed to connect to MQTT broker: %v", err)
		log.Println("Continuing without MQTT connection...")
	} else {
		s.setupMQTTSubscriptions()
	}

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

	log.Println("Server initialized successfully")
	return nil
}

func (s *Server) setupMQTTSubscriptions() {
	s.mqttClient.SetHandler("sensor/+/data", func(topic string, payload []byte) {
		data, err := s.mqttClient.ParseSensorData(payload)
		if err != nil {
			log.Printf("Failed to parse sensor data: %v", err)
			return
		}
		s.handleSensorData(data)
	})

	s.mqttClient.SetHandler("plc/+/status", func(topic string, payload []byte) {
		status, err := s.mqttClient.ParsePLCStatus(payload)
		if err != nil {
			log.Printf("Failed to parse PLC status: %v", err)
			return
		}
		s.handlePLCStatus(status)
	})

	if err := s.mqttClient.Subscribe("sensor/+/data", 1); err != nil {
		log.Printf("Failed to subscribe to sensor data: %v", err)
	}

	if err := s.mqttClient.Subscribe("plc/+/status", 1); err != nil {
		log.Printf("Failed to subscribe to PLC status: %v", err)
	}
}

func (s *Server) handleSensorData(data *models.SensorData) {
	if s.influxClient != nil {
		if err := s.influxClient.WriteSensorData(data); err != nil {
			log.Printf("Failed to write sensor data to InfluxDB: %v", err)
		}
	}

	s.alertManager.UpdateSensorData(data.SensorID, data.Value, data.Type, data.Timestamp)

	s.wsServer.BroadcastSensorData(data)

	if data.Type == models.SensorTypeCOD && s.sensorBelongsToLocation(data.SensorID, "influent") {
		go s.checkAndTriggerCarbonOptimization(data.Value)
	}
}

func (s *Server) checkAndTriggerCarbonOptimization(codValue float64) {
	no3Value := s.getAverageSensorValue(models.SensorTypeNO3, "anoxic")
	if no3Value <= 0 {
		no3Value = 8.0
	}

	tnInfluent := 40.0
	tnEffluent := no3Value + 2.0

	triggered := s.carbonCtrl.OnCODUpdate(codValue, no3Value, tnInfluent, tnEffluent, s.flowRate)
	if triggered {
		optimizedDosage := s.carbonOptimizer.Optimize(s.carbonCtrl, s.flowRate)
		if optimizedDosage != s.carbonCtrl.DosageSetpoint {
			log.Printf("[CARBON] Event-driven optimization adjusted dosage: %.3f -> %.3f",
				s.carbonCtrl.DosageSetpoint, optimizedDosage)
			s.carbonCtrl.SetDosageLimits(s.carbonCtrl.MinDosage, s.carbonCtrl.MaxDosage)
		}

		cmdData := s.carbonCtrl.GetControlCommand()
		controlCmd := &models.ControlCommand{
			CommandID:  fmt.Sprintf("cmd_%d", time.Now().UnixNano()),
			TargetType: cmdData["target_type"].(string),
			TargetID:   cmdData["target_id"].(string),
			Action:     cmdData["action"].(string),
			Value:      cmdData["value"].(float64),
			Unit:       cmdData["unit"].(string),
			Timestamp:  time.Now(),
			Source:     "carbon_control_event_driven",
		}

		if s.mqttClient != nil && s.mqttClient.IsConnected() {
			if err := s.mqttClient.PublishControlCommand(controlCmd); err != nil {
				log.Printf("Failed to publish event-driven carbon control command: %v", err)
			}
		}

		if s.influxClient != nil {
			if err := s.influxClient.WriteControlCommand(controlCmd); err != nil {
				log.Printf("Failed to write event-driven carbon control command: %v", err)
			}
		}

		s.wsServer.BroadcastControlCommand(controlCmd)
		s.wsServer.BroadcastCarbonControl(s.carbonCtrl.GetStatus())
	}
}

func (s *Server) handlePLCStatus(status *models.PLCStatus) {
	if s.influxClient != nil {
		if err := s.influxClient.WritePLCStatus(status); err != nil {
			log.Printf("Failed to write PLC status to InfluxDB: %v", err)
		}
	}

	if status.DeviceType == "fan" || status.DeviceType == "blower" {
		s.alertManager.UpdateFanStatus(status.DeviceID, status.Status, status.Timestamp)
	}

	s.wsServer.BroadcastPLCStatus(status)
}

func (s *Server) startSensorDataCollector() {
	log.Println("Sensor data collector started")
}

func (s *Server) startControlLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.runAerationControl()
		s.runCarbonControl()
	}
}

func (s *Server) runAerationControl() {
	zones := []string{"aerobic1", "aerobic2", "aerobic3"}

	for _, zone := range zones {
		doValue := s.getAverageSensorValue(models.SensorTypeDO, zone)
		nh3Value := s.getAverageSensorValue(models.SensorTypeNH3, zone)

		if doValue > 0 && nh3Value > 0 {
			if err := s.aerationCtrl.UpdateZone(zone, doValue, nh3Value, s.flowRate); err != nil {
				log.Printf("Failed to update aeration zone %s: %v", zone, err)
			}
		}
	}

	effluentNH3 := s.getAverageSensorValue(models.SensorTypeNH3, "effluent")
	if effluentNH3 > 0 {
		s.aerationOptimizer.Optimize(s.aerationCtrl.GetAllZones(), effluentNH3)
	}

	commands := s.aerationCtrl.GetControlCommands()
	for _, cmd := range commands {
		controlCmd := &models.ControlCommand{
			CommandID:  fmt.Sprintf("cmd_%d", time.Now().UnixNano()),
			TargetType: cmd["target_type"].(string),
			TargetID:   cmd["target_id"].(string),
			Action:     cmd["action"].(string),
			Value:      cmd["value"].(float64),
			Unit:       cmd["unit"].(string),
			Timestamp:  time.Now(),
			Source:     "aeration_control",
		}

		if s.mqttClient != nil && s.mqttClient.IsConnected() {
			if err := s.mqttClient.PublishControlCommand(controlCmd); err != nil {
				log.Printf("Failed to publish control command: %v", err)
			}
		}

		if s.influxClient != nil {
			if err := s.influxClient.WriteControlCommand(controlCmd); err != nil {
				log.Printf("Failed to write control command: %v", err)
			}
		}

		s.wsServer.BroadcastControlCommand(controlCmd)
	}

	if s.influxClient != nil {
		for zoneID, zone := range s.aerationCtrl.GetAllZones() {
			ac := &models.AerationControl{
				ZoneID:           zoneID,
				DOActual:         zone.DOActual,
				DOSetpoint:       zone.DOSetpoint,
				NH3Actual:        zone.NH3Actual,
				NH3Setpoint:      zone.NH3Setpoint,
				AirFlowSetpoint:  zone.AirFlowSetpoint,
				ValveOpening:     zone.ValveOpening,
				FanSpeed:         zone.FanSpeed,
				PIDOutput:        zone.PIDOutput,
				FeedforwardOutput: zone.FFOutput,
				TotalOutput:      zone.TotalOutput,
			}
			if err := s.influxClient.WriteAerationControl(ac, time.Now()); err != nil {
				log.Printf("Failed to write aeration control data: %v", err)
			}
		}
	}

	s.wsServer.BroadcastAerationControl(s.aerationCtrl.GetStatus())
}

func (s *Server) runCarbonControl() {
	no3Value := s.getAverageSensorValue(models.SensorTypeNO3, "anoxic")
	codValue := s.getAverageSensorValue(models.SensorTypeCOD, "influent")

	if no3Value <= 0 {
		no3Value = 8.0
	}
	if codValue <= 0 {
		codValue = 300.0
	}

	tnInfluent := 40.0
	tnEffluent := no3Value + 2.0

	s.carbonCtrl.Update(no3Value, codValue, tnInfluent, tnEffluent, s.flowRate)

	optimizedDosage := s.carbonOptimizer.Optimize(s.carbonCtrl, s.flowRate)
	if optimizedDosage != s.carbonCtrl.DosageSetpoint {
		s.carbonCtrl.SetDosageLimits(s.carbonCtrl.MinDosage, s.carbonCtrl.MaxDosage)
	}

	cmdData := s.carbonCtrl.GetControlCommand()
	controlCmd := &models.ControlCommand{
		CommandID:  fmt.Sprintf("cmd_%d", time.Now().UnixNano()),
		TargetType: cmdData["target_type"].(string),
		TargetID:   cmdData["target_id"].(string),
		Action:     cmdData["action"].(string),
		Value:      cmdData["value"].(float64),
		Unit:       cmdData["unit"].(string),
		Timestamp:  time.Now(),
		Source:     "carbon_control",
	}

	if s.mqttClient != nil && s.mqttClient.IsConnected() {
		if err := s.mqttClient.PublishControlCommand(controlCmd); err != nil {
			log.Printf("Failed to publish carbon control command: %v", err)
		}
	}

	if s.influxClient != nil {
		if err := s.influxClient.WriteControlCommand(controlCmd); err != nil {
			log.Printf("Failed to write carbon control command: %v", err)
		}

		cc := &models.CarbonControl{
			NO3Actual:       s.carbonCtrl.NO3Actual,
			CODInfluent:     s.carbonCtrl.CODInfluent,
			TNEstimate:      s.carbonCtrl.TNEffluent,
			DosageSetpoint:  s.carbonCtrl.DosageSetpoint,
			CarbonSourceType: s.carbonCtrl.CarbonSourceType,
			RemovalRate:     s.carbonCtrl.TNRemovalRate,
		}
		if err := s.influxClient.WriteCarbonControl(cc, time.Now()); err != nil {
			log.Printf("Failed to write carbon control data: %v", err)
		}
	}

	s.wsServer.BroadcastControlCommand(controlCmd)
	s.wsServer.BroadcastCarbonControl(s.carbonCtrl.GetStatus())
}

func (s *Server) getAverageSensorValue(sensorType models.SensorType, location string) float64 {
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

		if s.sensorBelongsToLocation(sensorID, location) {
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

func (s *Server) sensorBelongsToLocation(sensorID, location string) bool {
	for _, cfg := range s.sensorConfigs {
		if cfg.SensorID == sensorID {
			return cfg.Location == location
		}
	}
	return false
}

func randFloat() float64 {
	return float64(time.Now().UnixNano()%1000) / 1000.0
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
	carbonPerTon := s.carbonCtrl.CalculateCarbonPerTon(s.flowRate)

	influentNH3 := s.getAverageSensorValue(models.SensorTypeNH3, "influent")
	effluentNH3 := s.getAverageSensorValue(models.SensorTypeNH3, "effluent")
	influentNO3 := s.getAverageSensorValue(models.SensorTypeNO3, "influent")
	effluentNO3 := s.getAverageSensorValue(models.SensorTypeNO3, "effluent")
	influentPO4 := s.getAverageSensorValue(models.SensorTypePO4, "influent")
	effluentPO4 := s.getAverageSensorValue(models.SensorTypePO4, "effluent")

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

func (s *Server) startAlertChecker() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.alertManager.CheckSensorOffline(time.Now())
	}
}

func (s *Server) startStatusBroadcaster() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		status := map[string]interface{}{
			"timestamp":       time.Now(),
			"alert_status":    s.alertManager.GetStatus(),
			"aeration_status": s.aerationCtrl.GetStatus(),
			"carbon_status":   s.carbonCtrl.GetStatus(),
			"flow_rate":       s.flowRate,
			"ws_clients":      s.wsServer.GetClientCount(),
			"mqtt_connected":  s.mqttClient.IsConnected(),
		}
		s.wsServer.BroadcastSystemStatus(status)
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
		c.JSON(200, s.carbonCtrl.GetStatus())
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
			c.JSON(200, s.alertManager.GetAlertHistory(100))
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
		c.JSON(200, s.alertManager.GetActiveAlerts())
	})

	api.POST("/alerts/:id/acknowledge", func(c *gin.Context) {
		alertID := c.Param("id")
		if s.alertManager.AcknowledgeAlert(alertID) {
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
			"alert_status":    s.alertManager.GetStatus(),
			"aeration_status": s.aerationCtrl.GetStatus(),
			"carbon_status":   s.carbonCtrl.GetStatus(),
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

	log.Println("Server stopped gracefully")
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	gin.SetMode(gin.ReleaseMode)
}

var _ = json.Marshal
