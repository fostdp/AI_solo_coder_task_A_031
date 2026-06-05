package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"sewage-plant-system/pkg/models"
	"sewage-plant-system/pkg/websocket"
)

type AlertLevel int

const (
	Level1 AlertLevel = 1
	Level2 AlertLevel = 2
)

type AlertType string

const (
	AlertTypeEffluentNH3    AlertType = "effluent_nh3"
	AlertTypeEffluentTN     AlertType = "effluent_tn"
	AlertTypeFanFault       AlertType = "fan_fault"
	AlertTypeSensorOffline  AlertType = "sensor_offline"
	AlertTypeDOAbnormal     AlertType = "do_abnormal"
	AlertTypeNH3Abnormal    AlertType = "nh3_abnormal"
)

type ThresholdConfig struct {
	NH3Threshold  float64
	TNThreshold   float64
	Duration      time.Duration
	OfflineTimeout time.Duration
	FanFaultTimeout time.Duration
}

type SensorStatus struct {
	SensorID    string
	LastValue   float64
	LastUpdate  time.Time
	IsOnline    bool
	ExceedStart *time.Time
}

type AlertManager struct {
	thresholds      ThresholdConfig
	alerts          []*models.Alert
	sensorStatus    map[string]*SensorStatus
	fanStatus       map[string]*FanStatus
	mu              sync.RWMutex
	wsServer        *websocket.Server
	smsConfig       SMSConfig
	activeAlerts    map[string]*models.Alert
	alertHistory    []*models.Alert
}

type FanStatus struct {
	FanID       string
	Status      string
	LastUpdate  time.Time
	FaultStart  *time.Time
}

type SMSConfig struct {
	Enabled    bool
	APIURL     string
	APIKey     string
	Recipients []string
}

func NewAlertManager(ws *websocket.Server, smsCfg SMSConfig) *AlertManager {
	return &AlertManager{
		thresholds: ThresholdConfig{
			NH3Threshold:    5.0,
			TNThreshold:     15.0,
			Duration:        30 * time.Minute,
			OfflineTimeout:  5 * time.Minute,
			FanFaultTimeout: 60 * time.Second,
		},
		sensorStatus: make(map[string]*SensorStatus),
		fanStatus:    make(map[string]*FanStatus),
		activeAlerts: make(map[string]*models.Alert),
		alertHistory: make([]*models.Alert, 0, 1000),
		wsServer:     ws,
		smsConfig:    smsCfg,
	}
}

func (am *AlertManager) UpdateSensorData(sensorID string, value float64, sensorType models.SensorType, timestamp time.Time) {
	am.mu.Lock()
	defer am.mu.Unlock()

	status, exists := am.sensorStatus[sensorID]
	if !exists {
		status = &SensorStatus{
			SensorID: sensorID,
			IsOnline: true,
		}
		am.sensorStatus[sensorID] = status
	}

	status.LastValue = value
	status.LastUpdate = timestamp

	if !status.IsOnline {
		status.IsOnline = true
		am.resolveAlert(sensorID, AlertTypeSensorOffline)
	}

	am.checkLevel1Alerts(sensorID, value, sensorType, timestamp)
}

func (am *AlertManager) checkLevel1Alerts(sensorID string, value float64, sensorType models.SensorType, timestamp time.Time) {
	var alertType AlertType
	var threshold float64
	var exceed bool

	switch sensorType {
	case models.SensorTypeNH3:
		alertType = AlertTypeEffluentNH3
		threshold = am.thresholds.NH3Threshold
		exceed = value > threshold
	case models.SensorTypeNO3:
		alertType = AlertTypeEffluentTN
		threshold = am.thresholds.TNThreshold
		exceed = value > threshold
	default:
		return
	}

	status := am.sensorStatus[sensorID]

	if exceed {
		if status.ExceedStart == nil {
			start := timestamp
			status.ExceedStart = &start
		} else if timestamp.Sub(*status.ExceedStart) >= am.thresholds.Duration {
			alertKey := fmt.Sprintf("%s_%s", sensorID, alertType)
			if _, exists := am.activeAlerts[alertKey]; !exists {
				am.createAlert(
					Level1,
					alertType,
					fmt.Sprintf("出水%s超标", getSensorTypeName(sensorType)),
					fmt.Sprintf("传感器%s检测到%s浓度%.2f mg/L，已连续超过阈值%.2f mg/L达30分钟",
						sensorID, getSensorTypeName(sensorType), value, threshold),
					sensorID,
					value,
					threshold,
					timestamp,
				)
			}
		}
	} else {
		status.ExceedStart = nil
		alertKey := fmt.Sprintf("%s_%s", sensorID, alertType)
		am.resolveAlert(sensorID, alertType)
	}
}

func (am *AlertManager) CheckSensorOffline(timestamp time.Time) {
	am.mu.Lock()
	defer am.mu.Unlock()

	for sensorID, status := range am.sensorStatus {
		if status.IsOnline && timestamp.Sub(status.LastUpdate) > am.thresholds.OfflineTimeout {
			status.IsOnline = false
			alertKey := fmt.Sprintf("%s_%s", sensorID, AlertTypeSensorOffline)
			if _, exists := am.activeAlerts[alertKey]; !exists {
				am.createAlert(
					Level2,
					AlertTypeSensorOffline,
					"传感器离线",
					fmt.Sprintf("传感器%s已离线超过%d秒", sensorID, int(am.thresholds.OfflineTimeout.Seconds())),
					sensorID,
					0,
					0,
					timestamp,
				)
			}
		}
	}
}

func (am *AlertManager) UpdateFanStatus(fanID string, status string, timestamp time.Time) {
	am.mu.Lock()
	defer am.mu.Unlock()

	fs, exists := am.fanStatus[fanID]
	if !exists {
		fs = &FanStatus{FanID: fanID}
		am.fanStatus[fanID] = fs
	}

	fs.Status = status
	fs.LastUpdate = timestamp

	if status == "fault" || status == "error" || status == "offline" {
		if fs.FaultStart == nil {
			start := timestamp
			fs.FaultStart = &start
		} else if timestamp.Sub(*fs.FaultStart) >= am.thresholds.FanFaultTimeout {
			alertKey := fmt.Sprintf("%s_%s", fanID, AlertTypeFanFault)
			if _, exists := am.activeAlerts[alertKey]; !exists {
				am.createAlert(
					Level2,
					AlertTypeFanFault,
					"曝气风机故障",
					fmt.Sprintf("风机%s发生故障，状态: %s", fanID, status),
					fanID,
					0,
					0,
					timestamp,
				)
			}
		}
	} else {
		fs.FaultStart = nil
		alertKey := fmt.Sprintf("%s_%s", fanID, AlertTypeFanFault)
		delete(am.activeAlerts, alertKey)
	}
}

func (am *AlertManager) createAlert(level AlertLevel, alertType AlertType, title, message, sensorID string, value, threshold float64, timestamp time.Time) {
	alert := &models.Alert{
		AlertID:      fmt.Sprintf("alert_%d_%s", time.Now().UnixNano(), sensorID),
		Level:        int(level),
		Type:         string(alertType),
		Title:        title,
		Message:      message,
		SensorID:     sensorID,
		Value:        value,
		Threshold:    threshold,
		Timestamp:    timestamp,
		Acknowledged: false,
	}

	alertKey := fmt.Sprintf("%s_%s", sensorID, alertType)
	am.activeAlerts[alertKey] = alert
	am.alertHistory = append(am.alertHistory, alert)
	if len(am.alertHistory) > 1000 {
		am.alertHistory = am.alertHistory[1:]
	}

	log.Printf("[ALERT] Level %d - %s: %s", level, title, message)

	if am.wsServer != nil {
		am.wsServer.BroadcastAlert(alert)
	}

	if am.smsConfig.Enabled {
		go am.sendSMS(alert)
	}
}

func (am *AlertManager) resolveAlert(sensorID string, alertType AlertType) {
	alertKey := fmt.Sprintf("%s_%s", sensorID, alertType)
	delete(am.activeAlerts, alertKey)
}

func (am *AlertManager) sendSMS(alert *models.Alert) {
	if !am.smsConfig.Enabled {
		return
	}

	for _, recipient := range am.smsConfig.Recipients {
		message := fmt.Sprintf("[污水处理厂告警] 级别%d: %s - %s", alert.Level, alert.Title, alert.Message)
		payload := map[string]interface{}{
			"api_key":  am.smsConfig.APIKey,
			"to":       recipient,
			"message":  message,
			"priority": alert.Level,
		}

		go func(recipient string) {
			data, _ := json.Marshal(payload)
			resp, err := http.Post(am.smsConfig.APIURL, "application/json", bytes.NewBuffer(data))
			if err != nil {
				log.Printf("Failed to send SMS to %s: %v", recipient, err)
				return
			}
			defer resp.Body.Close()
			log.Printf("SMS sent to %s, status: %d", recipient, resp.StatusCode)
		}(recipient)
	}
}

func (am *AlertManager) GetActiveAlerts() []*models.Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()

	alerts := make([]*models.Alert, 0, len(am.activeAlerts))
	for _, alert := range am.activeAlerts {
		alerts = append(alerts, alert)
	}
	return alerts
}

func (am *AlertManager) GetAlertHistory(limit int) []*models.Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()

	start := 0
	if len(am.alertHistory) > limit {
		start = len(am.alertHistory) - limit
	}
	return am.alertHistory[start:]
}

func (am *AlertManager) AcknowledgeAlert(alertID string) bool {
	am.mu.Lock()
	defer am.mu.Unlock()

	for key, alert := range am.activeAlerts {
		if alert.AlertID == alertID {
			alert.Acknowledged = true
			delete(am.activeAlerts, key)
			return true
		}
	}

	for _, alert := range am.alertHistory {
		if alert.AlertID == alertID {
			alert.Acknowledged = true
			return true
		}
	}

	return false
}

func (am *AlertManager) GetStatus() map[string]interface{} {
	am.mu.RLock()
	defer am.mu.RUnlock()

	level1Count := 0
	level2Count := 0
	for _, alert := range am.activeAlerts {
		if alert.Level == int(Level1) {
			level1Count++
		} else if alert.Level == int(Level2) {
			level2Count++
		}
	}

	onlineSensors := 0
	for _, status := range am.sensorStatus {
		if status.IsOnline {
			onlineSensors++
		}
	}

	return map[string]interface{}{
		"active_alerts":    len(am.activeAlerts),
		"level1_alerts":    level1Count,
		"level2_alerts":    level2Count,
		"total_sensors":    len(am.sensorStatus),
		"online_sensors":   onlineSensors,
		"offline_sensors":  len(am.sensorStatus) - onlineSensors,
		"total_fans":       len(am.fanStatus),
	}
}

func getSensorTypeName(sensorType models.SensorType) string {
	switch sensorType {
	case models.SensorTypeDO:
		return "溶解氧(DO)"
	case models.SensorTypeNH3:
		return "氨氮(NH3-N)"
	case models.SensorTypeNO3:
		return "硝态氮(NO3-N)"
	case models.SensorTypePO4:
		return "磷酸盐(PO4-P)"
	case models.SensorTypeCOD:
		return "化学需氧量(COD)"
	default:
		return string(sensorType)
	}
}

func (am *AlertManager) GetDeviationLevel(value, setpoint float64) string {
	if setpoint <= 0 {
		return "green"
	}
	deviation := abs((value - setpoint) / setpoint * 100)
	if deviation < 10 {
		return "green"
	} else if deviation < 20 {
		return "yellow"
	} else {
		return "red"
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
