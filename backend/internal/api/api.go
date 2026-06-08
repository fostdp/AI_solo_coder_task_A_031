package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"sewage-treatment-system/internal/alarm"
	"sewage-treatment-system/internal/collector"
	"sewage-treatment-system/internal/controller"
	"sewage-treatment-system/internal/influxdb"
	"sewage-treatment-system/internal/models"
	"sewage-treatment-system/internal/websocket"
)

type Server struct {
	router              *gin.Engine
	logger              *zap.Logger
	influxClient        *influxdb.Client
	wsServer            *websocket.Server
	sensorCollector     *collector.SensorCollector
	aerationController  *controller.AerationController
	carbonOptimizer     *controller.CarbonOptimizer
	alarmRouter         *alarm.AlarmRouter
	sensorInfo          map[string]models.SensorInfo
}

func NewServer(
	influxClient *influxdb.Client,
	wsServer *websocket.Server,
	aerationController *controller.AerationController,
	carbonOptimizer *controller.CarbonOptimizer,
	alarmRouter *alarm.AlarmRouter,
	sensorCollector *collector.SensorCollector,
	sensorInfo map[string]models.SensorInfo,
	logger *zap.Logger,
	mode string,
) *Server {
	gin.SetMode(mode)
	router := gin.Default()

	s := &Server{
		router:              router,
		logger:              logger,
		influxClient:        influxClient,
		wsServer:            wsServer,
		sensorCollector:     sensorCollector,
		aerationController:  aerationController,
		carbonOptimizer:     carbonOptimizer,
		alarmRouter:         alarmRouter,
		sensorInfo:          sensorInfo,
	}

	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.router.Use(CORS())

	api := s.router.Group("/api/v1")
	{
		sensors := api.Group("/sensors")
		{
			sensors.GET("", s.GetAllSensors)
			sensors.GET("/info", s.GetSensorInfo)
			sensors.GET("/:id", s.GetSensorData)
			sensors.GET("/:id/trend", s.GetSensorTrend)
			sensors.POST("/data", s.ReceiveSensorData)
		}

		control := api.Group("/control")
		{
			control.GET("/aeration", s.GetAerationStatus)
			control.GET("/aeration/:section", s.GetAerationSectionStatus)
			control.POST("/aeration/setpoint", s.SetAerationSetpoint)
			control.POST("/aeration/tuning", s.SetAerationTuning)
			control.GET("/carbon", s.GetCarbonStatus)
			control.POST("/carbon/target", s.SetCarbonTarget)
		}

		alarm := api.Group("/alerts")
		{
			alarm.GET("", s.GetActiveAlarms)
			alarm.GET("/level/:level", s.GetAlarmsByLevel)
			alarm.POST("/:id/ack", s.AcknowledgeAlarm)
		}

		metrics := api.Group("/metrics")
		{
			metrics.GET("", s.GetKeyMetrics)
			metrics.GET("/trend/:metric", s.GetMetricsTrend)
		}

		api.GET("/process/stages", s.GetProcessStages)
		api.GET("/ws", s.HandleWebSocket)
	}

	s.router.Static("/", "./frontend")
}

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func (s *Server) Run(addr string) error {
	return s.router.Run(addr)
}

func (s *Server) GetAllSensors(c *gin.Context) {
	data, err := s.influxClient.QueryAllLatestSensors()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]gin.H, 0)
	for id, sensor := range data {
		info, hasInfo := s.sensorInfo[id]
		deviation := 0.0
		if sensor.Setpoint > 0 {
			deviation = abs((sensor.Value - sensor.Setpoint) / sensor.Setpoint * 100)
		}

		color := "green"
		if deviation > 20 {
			color = "red"
		} else if deviation > 10 {
			color = "yellow"
		}

		item := gin.H{
			"id":        id,
			"type":      sensor.Type,
			"value":     sensor.Value,
			"setpoint":  sensor.Setpoint,
			"deviation": deviation,
			"color":     color,
			"timestamp": sensor.Timestamp,
			"status":    sensor.Status,
		}

		if hasInfo {
			item["x"] = info.X
			item["y"] = info.Y
			item["stage"] = info.Stage
			item["section"] = info.Section
		}

		result = append(result, item)
	}

	c.JSON(http.StatusOK, gin.H{"sensors": result})
}

func (s *Server) GetSensorInfo(c *gin.Context) {
	infoList := make([]models.SensorInfo, 0, len(s.sensorInfo))
	for _, info := range s.sensorInfo {
		infoList = append(infoList, info)
	}
	c.JSON(http.StatusOK, gin.H{"sensors": infoList})
}

func (s *Server) GetSensorData(c *gin.Context) {
	id := c.Param("id")
	data, err := s.influxClient.QueryLatestSensor(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "sensor not found"})
		return
	}

	info, hasInfo := s.sensorInfo[id]
	result := gin.H{
		"id":        id,
		"type":      data.Type,
		"value":     data.Value,
		"setpoint":  data.Setpoint,
		"timestamp": data.Timestamp,
		"status":    data.Status,
	}

	if hasInfo {
		result["x"] = info.X
		result["y"] = info.Y
		result["stage"] = info.Stage
		result["section"] = info.Section
	}

	c.JSON(http.StatusOK, result)
}

func (s *Server) GetSensorTrend(c *gin.Context) {
	id := c.Param("id")
	hoursStr := c.DefaultQuery("hours", "6")
	hours, err := strconv.Atoi(hoursStr)
	if err != nil || hours <= 0 {
		hours = 6
	}

	data, err := s.influxClient.QuerySensorTrend(id, time.Duration(hours)*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"sensor_id": id,
		"data":      data,
	})
}

func (s *Server) ReceiveSensorData(c *gin.Context) {
	var data models.SensorData
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.sensorCollector.ProcessSensorData(&data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (s *Server) GetAerationStatus(c *gin.Context) {
	controls := s.aerationController.GetAllStatus()
	c.JSON(http.StatusOK, gin.H{"sections": controls})
}

func (s *Server) GetAerationSectionStatus(c *gin.Context) {
	section, err := strconv.Atoi(c.Param("section"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid section"})
		return
	}

	ctrl := s.aerationController.GetSectionStatus(section)
	if ctrl == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "section not found"})
		return
	}

	c.JSON(http.StatusOK, ctrl)
}

func (s *Server) SetAerationSetpoint(c *gin.Context) {
	var req struct {
		DOSetpoint  float64 `json:"do_setpoint"`
		NH3Setpoint float64 `json:"nh3_setpoint"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for i := 1; i <= 30; i++ {
		if req.DOSetpoint > 0 {
			s.aerationController.ResetSection(i)
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (s *Server) SetAerationTuning(c *gin.Context) {
	var req struct {
		Kp float64 `json:"kp"`
		Ki float64 `json:"ki"`
		Kd float64 `json:"kd"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	s.aerationController.UpdateTuning(req.Kp, req.Ki, req.Kd)
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (s *Server) GetCarbonStatus(c *gin.Context) {
	status := s.carbonOptimizer.GetLatestData()
	c.JSON(http.StatusOK, status)
}

func (s *Server) SetCarbonTarget(c *gin.Context) {
	var req struct {
		TNRemovalTarget float64 `json:"tn_removal_target"`
		DosingMax       float64 `json:"dosing_max"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	dosingMax := 150.0
	if req.DosingMax > 0 {
		dosingMax = req.DosingMax
	}

	s.carbonOptimizer.UpdateConfig(dosingMax, req.TNRemovalTarget)
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (s *Server) GetActiveAlarms(c *gin.Context) {
	alarms := s.alarmRouter.GetActiveAlarms()
	c.JSON(http.StatusOK, gin.H{"alarms": alarms})
}

func (s *Server) GetAlarmsByLevel(c *gin.Context) {
	level, err := strconv.Atoi(c.Param("level"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid level"})
		return
	}

	allAlarms := s.alarmRouter.GetActiveAlarms()
	filtered := make([]*models.Alarm, 0)
	for _, a := range allAlarms {
		if a.Level == level {
			filtered = append(filtered, a)
		}
	}
	c.JSON(http.StatusOK, gin.H{"alarms": filtered})
}

func (s *Server) AcknowledgeAlarm(c *gin.Context) {
	id := c.Param("id")
	if s.alarmRouter.AcknowledgeAlarm(id) {
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	} else {
		c.JSON(http.StatusNotFound, gin.H{"error": "alarm not found"})
	}
}

func (s *Server) GetKeyMetrics(c *gin.Context) {
	duration := 24 * time.Hour

	power, _ := s.influxClient.QueryAggregatedValue("key_metrics", "power_consumption", "", duration)
	carbon, _ := s.influxClient.QueryAggregatedValue("key_metrics", "carbon_usage", "", duration)
	tnRemoval, _ := s.influxClient.QueryAggregatedValue("key_metrics", "tn_removal_rate", "", duration)
	tpRemoval, _ := s.influxClient.QueryAggregatedValue("key_metrics", "tp_removal_rate", "", duration)
	codRemoval, _ := s.influxClient.QueryAggregatedValue("key_metrics", "cod_removal_rate", "", duration)
	flow, _ := s.influxClient.QueryAggregatedValue("key_metrics", "flow_rate", "", duration)

	c.JSON(http.StatusOK, gin.H{
		"power_consumption": power,
		"carbon_usage":      carbon,
		"tn_removal_rate":   tnRemoval,
		"tp_removal_rate":   tpRemoval,
		"cod_removal_rate":  codRemoval,
		"flow_rate":         flow,
		"timestamp":         time.Now(),
	})
}

func (s *Server) GetMetricsTrend(c *gin.Context) {
	metric := c.Param("metric")
	daysStr := c.DefaultQuery("days", "7")
	days, err := strconv.Atoi(daysStr)
	if err != nil || days <= 0 {
		days = 7
	}

	validMetrics := map[string]bool{
		"power_consumption": true,
		"carbon_usage":      true,
		"tn_removal_rate":   true,
		"tp_removal_rate":   true,
		"cod_removal_rate":  true,
		"flow_rate":         true,
	}

	if !validMetrics[metric] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid metric"})
		return
	}

	data, err := s.influxClient.QueryMetricsTrend(metric, time.Duration(days)*24*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"metric": metric,
		"data":   data,
	})
}

func (s *Server) GetProcessStages(c *gin.Context) {
	stages := []map[string]interface{}{
		{"id": "coarse_grate", "name": "粗格栅", "status": "normal"},
		{"id": "fine_grate", "name": "细格栅", "status": "normal"},
		{"id": "grit_chamber", "name": "沉砂池", "status": "normal"},
		{"id": "primary_settling", "name": "初沉池", "status": "normal"},
		{"id": "anaerobic", "name": "厌氧池", "status": "normal"},
		{"id": "anoxic", "name": "缺氧池", "status": "normal"},
		{"id": "aerobic", "name": "好氧池", "status": "normal"},
		{"id": "secondary_settling", "name": "二沉池", "status": "normal"},
		{"id": "advanced_treatment", "name": "深度处理", "status": "normal"},
		{"id": "effluent", "name": "出水", "status": "normal"},
	}

	c.JSON(http.StatusOK, gin.H{"stages": stages})
}

func (s *Server) HandleWebSocket(c *gin.Context) {
	s.wsServer.HandleWebSocket(c.Writer, c.Request)
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
