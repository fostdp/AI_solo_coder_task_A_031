package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"sewage-treatment/backend/alarm"
	"sewage-treatment/backend/control"
	"sewage-treatment/backend/influxdb"
	"sewage-treatment/backend/models"
)

type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func GetSensors(c *gin.Context) {
	sensors := models.SensorConfigs

	c.JSON(http.StatusOK, APIResponse{
		Code:    0,
		Message: "success",
		Data:    sensors,
	})
}

func GetSensorData(c *gin.Context) {
	sensorID := c.Param("id")
	if sensorID == "" {
		c.JSON(http.StatusBadRequest, APIResponse{
			Code:    400,
			Message: "sensor id is required",
		})
		return
	}

	data, err := influxdb.InfluxClient.QueryLatestSensorData(sensorID)
	if err != nil {
		c.JSON(http.StatusNotFound, APIResponse{
			Code:    404,
			Message: "sensor data not found",
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

func GetSensorTrend(c *gin.Context) {
	sensorID := c.Param("id")
	hoursStr := c.DefaultQuery("hours", "6")
	hours, err := strconv.Atoi(hoursStr)
	if err != nil {
		hours = 6
	}

	end := time.Now()
	start := end.Add(time.Duration(-hours) * time.Hour)

	trend, err := influxdb.InfluxClient.QuerySensorTrend(sensorID, start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Code:    500,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Code:    0,
		Message: "success",
		Data:    trend,
	})
}

func GetAllSensorStatus(c *gin.Context) {
	dataList, err := influxdb.InfluxClient.QueryAllLatestSensorData()
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Code:    500,
			Message: err.Error(),
		})
		return
	}

	statusList := make([]*models.SensorStatus, 0, len(dataList))

	for _, data := range dataList {
		cfg := models.GetSensorConfig(data.ID)
		if cfg == nil {
			continue
		}

		statusList = append(statusList, calculateStatus(&data, cfg))
	}

	c.JSON(http.StatusOK, APIResponse{
		Code:    0,
		Message: "success",
		Data:    statusList,
	})
}

func calculateStatus(data *models.SensorData, cfg *models.SensorConfig) *models.SensorStatus {
	target := (cfg.TargetMin + cfg.TargetMax) / 2
	rangeVal := cfg.TargetMax - cfg.TargetMin
	if rangeVal == 0 {
		rangeVal = 1
	}

	deviation := 0.0
	if data.Value > target {
		deviation = (data.Value - target) / rangeVal
	} else {
		deviation = (target - data.Value) / rangeVal
	}

	deviationPercent := deviation * 100

	color := "#4CAF50"
	if deviationPercent > 20 {
		color = "#f44336"
	} else if deviationPercent > 10 {
		color = "#ff9800"
	}

	offlineThreshold := 5 * time.Minute
	online := time.Since(data.Timestamp) < offlineThreshold

	return &models.SensorStatus{
		ID:         data.ID,
		Type:       string(data.Type),
		Stage:      string(data.Stage),
		Value:      data.Value,
		Deviation:  deviationPercent,
		Color:      color,
		LastUpdate: data.Timestamp,
		Online:     online,
	}
}

func GetAerationStatus(c *gin.Context) {
	if control.AerationCtl == nil {
		c.JSON(http.StatusOK, APIResponse{
			Code:    0,
			Message: "success",
			Data:    []interface{}{},
		})
		return
	}

	status := control.AerationCtl.GetAllStatus()
	c.JSON(http.StatusOK, APIResponse{
		Code:    0,
		Message: "success",
		Data:    status,
	})
}

func GetCarbonStatus(c *gin.Context) {
	if control.CarbonOpt == nil {
		c.JSON(http.StatusOK, APIResponse{
			Code:    0,
			Message: "success",
			Data:    nil,
		})
		return
	}

	status := control.CarbonOpt.GetStatus()
	c.JSON(http.StatusOK, APIResponse{
		Code:    0,
		Message: "success",
		Data:    status,
	})
}

func GetKPIs(c *gin.Context) {
	energy, carbon, removal := control.CalculateKPIValues()

	c.JSON(http.StatusOK, APIResponse{
		Code:    0,
		Message: "success",
		Data: map[string]float64{
			"energy_consumption": energy,
			"carbon_consumption": carbon,
			"removal_rate":    removal,
		},
	})
}

func GetKPITrend(c *gin.Context) {
	kpiType := c.Param("type")
	daysStr := c.DefaultQuery("days", "7")
	days, err := strconv.Atoi(daysStr)
	if err != nil {
		days = 7
	}

	end := time.Now()
	start := end.AddDate(0, 0, -days)

	trend, err := influxdb.InfluxClient.QueryKPITrend(kpiType, start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Code:    500,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Code:    0,
		Message: "success",
		Data:    trend,
	})
}

func GetAlarms(c *gin.Context) {
	activeOnly := c.DefaultQuery("active", "true")

	var alarms []*models.Alarm
	if activeOnly == "true" {
		alarms = alarm.AlarmMgr.GetActiveAlarms()
	} else {
		limitStr := c.DefaultQuery("limit", "100")
		limit, _ := strconv.Atoi(limitStr)
		alarms = alarm.GetAlarmHistory(limit)
	}

	c.JSON(http.StatusOK, APIResponse{
		Code:    0,
		Message: "success",
		Data:    alarms,
	})
}

func AcknowledgeAlarm(c *gin.Context) {
	alarmID := c.Param("id")
	err := alarm.AlarmMgr.AcknowledgeAlarm(alarmID)
	if err != nil {
		c.JSON(http.StatusNotFound, APIResponse{
			Code:    404,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Code:    0,
		Message: "alarm acknowledged",
	})
}

func GetProcessStatus(c *gin.Context) {
	stages := []models.ProcessStage{
		models.StageCoarseScreen,
		models.StageFineScreen,
		models.StageGritChamber,
		models.StagePrimary,
		models.StageAnaerobic,
		models.StageAnoxic,
		models.StageAerobic,
		models.StageSecondary,
		models.StageAdvanced,
		models.StageEffluent,
	}

	dataList, err := influxdb.InfluxClient.QueryAllLatestSensorData()
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{
			Code:    500,
			Message: err.Error(),
		})
		return
	}

	sensorMap := make(map[string]*models.SensorData)
	for i := range dataList {
		sensorMap[dataList[i].ID] = &dataList[i]
	}

	processStatus := make([]models.ProcessStatus, 0)

	for _, stage := range stages {
		sensors := make([]models.SensorStatus, 0)
		hasAlarm := false

		for _, cfg := range models.SensorConfigs {
			if cfg.Stage == stage {
				data, exists := sensorMap[cfg.ID]
				if exists {
					status := calculateStatus(data, &cfg)
					sensors = append(sensors, *status)
					if status.Color == "#f44336" || !status.Online {
						hasAlarm = true
					}
				}
			}
		}

		status := "normal"
		if hasAlarm {
			status = "alarm"
		} else if len(sensors) > 0 {
			for _, s := range sensors {
				if s.Color == "#ff9800" {
					status = "warning"
					break
				}
			}
		}

		processStatus = append(processStatus, models.ProcessStatus{
			Stage:   string(stage),
			Sensors: sensors,
			Status:  status,
		})
	}

	c.JSON(http.StatusOK, APIResponse{
		Code:    0,
		Message: "success",
		Data:    processStatus,
	})
}

func GetOverview(c *gin.Context) {
	energy, carbon, removal := control.CalculateKPIValues()

	activeAlarms := alarm.AlarmMgr.GetActiveAlarms()

	nh3Eff, _ := control.AerationCtl.GetEffluentQuality()

	c.JSON(http.StatusOK, APIResponse{
		Code:    0,
		Message: "success",
		Data: map[string]interface{}{
			"kpi": map[string]float64{
				"energy_consumption": energy,
				"carbon_consumption": carbon,
				"removal_rate":    removal,
			},
			"active_alarms_count": len(activeAlarms),
			"effluent_quality": map[string]float64{
				"nh3": nh3Eff,
			},
			"system_status": "running",
		},
	})
}

func SetupRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		api.GET("/overview", GetOverview)

		sensors := api.Group("/sensors")
		{
			sensors.GET("", GetSensors)
			sensors.GET("/status", GetAllSensorStatus)
			sensors.GET("/:id", GetSensorData)
			sensors.GET("/:id/trend", GetSensorTrend)
		}

		control := api.Group("/control")
		{
			control.GET("/aeration", GetAerationStatus)
			control.GET("/carbon", GetCarbonStatus)
		}

		kpi := api.Group("/kpi")
		{
			kpi.GET("", GetKPIs)
			kpi.GET("/:type/trend", GetKPITrend)
		}

		alarms := api.Group("/alarms")
		{
			alarms.GET("", GetAlarms)
			alarms.POST("/:id/ack", AcknowledgeAlarm)
		}

		api.GET("/process", GetProcessStatus)
	}
}
