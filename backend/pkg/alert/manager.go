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
	thresholds          ThresholdConfig
	alerts              []*models.Alert
	sensorStatus        map[string]*SensorStatus
	fanStatus           map[string]*FanStatus
	mu                  sync.RWMutex
	wsServer            *websocket.Server
	smsConfig           SMSConfig
	activeAlerts        map[string]*models.Alert
	alertHistory        []*models.Alert
	wsAvailable         bool
	wsClientCount       int
	wsLastStateChange   time.Time
	alertBuffer         []*BufferedAlert
	bufferMaxSize       int
	smsFallbackEnabled  bool
	smsFallbackLevel    int
	channelSwitchCount  int
}

type FanStatus struct {
	FanID       string
	Status      string
	LastUpdate  time.Time
	FaultStart  *time.Time
}

type BufferedAlert struct {
	Alert       *models.Alert
	QueuedAt    time.Time
	Delivered   bool
	DeliveredAt time.Time
	Channel     string
	RetryCount  int
}

type ChannelStatus int

const (
	ChannelPrimary ChannelStatus = iota
	ChannelDegraded
	ChannelFailed
)

func NewAlertManager(ws *websocket.Server, smsCfg SMSConfig) *AlertManager {
	am := &AlertManager{
		thresholds: ThresholdConfig{
			NH3Threshold:    5.0,
			TNThreshold:     15.0,
			Duration:        30 * time.Minute,
			OfflineTimeout:  5 * time.Minute,
			FanFaultTimeout: 60 * time.Second,
		},
		sensorStatus:       make(map[string]*SensorStatus),
		fanStatus:          make(map[string]*FanStatus),
		activeAlerts:       make(map[string]*models.Alert),
		alertHistory:       make([]*models.Alert, 0, 1000),
		alertBuffer:        make([]*BufferedAlert, 0, 100),
		bufferMaxSize:      100,
		smsFallbackEnabled: true,
		smsFallbackLevel:   1,
		wsServer:           ws,
		smsConfig:          smsCfg,
	}

	if ws != nil {
		ws.SetStatusChangeCallback(am.OnWSStatusChange)
		am.wsAvailable = ws.HasClients()
		log.Printf("[ALERT] WebSocket status initialized: available=%v", am.wsAvailable)
	}

	return am
}

type SMSConfig struct {
	Enabled    bool
	APIURL     string
	APIKey     string
	Recipients []string
}

func (am *AlertManager) OnWSStatusChange(connected bool, clientCount int) {
	am.mu.Lock()
	defer am.mu.Unlock()

	oldStatus := am.wsAvailable
	am.wsAvailable = connected
	am.wsClientCount = clientCount
	am.wsLastStateChange = time.Now()

	if oldStatus != connected {
		am.channelSwitchCount++
		log.Printf("[ALERT] WebSocket status changed: %v -> %v, clients=%d, total_switches=%d",
			oldStatus, connected, clientCount, am.channelSwitchCount)

		if connected {
			log.Printf("[ALERT] WebSocket reconnected, flushing buffered alerts (%d pending)", len(am.alertBuffer))
			go am.flushBufferedAlerts()
		} else {
			log.Printf("[ALERT] WebSocket disconnected, enabling SMS fallback for all alerts")
			go am.resendBufferedAlertsViaSMS()
		}
	}
}

func (am *AlertManager) GetChannelStatus() ChannelStatus {
	am.mu.RLock()
	defer am.mu.RUnlock()

	if am.wsAvailable {
		return ChannelPrimary
	} else if am.smsConfig.Enabled {
		return ChannelDegraded
	}
	return ChannelFailed
}

func (am *AlertManager) shouldUseSMSFallback(alertLevel int) bool {
	am.mu.RLock()
	defer am.mu.RUnlock()

	if !am.smsFallbackEnabled {
		return false
	}

	if !am.wsAvailable {
		return true
	}

	if alertLevel <= am.smsFallbackLevel && am.wsAvailable {
		return false
	}

	return false
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

	am.deliverAlert(alert)
}

func (am *AlertManager) deliverAlert(alert *models.Alert) {
	am.mu.RLock()
	wsAvailable := am.wsAvailable
	smsEnabled := am.smsConfig.Enabled
	useSMSFallback := am.shouldUseSMSFallback(alert.Level)
	am.mu.RUnlock()

	delivered := false
	channelsUsed := []string{}

	if wsAvailable && am.wsServer != nil {
		err := am.wsServer.BroadcastAlert(alert)
		if err != nil {
			log.Printf("[ALERT] WebSocket broadcast failed for alert %s: %v", alert.AlertID, err)
		} else {
			delivered = true
			channelsUsed = append(channelsUsed, "websocket")
			log.Printf("[ALERT] Alert %s delivered via WebSocket", alert.AlertID)
		}
	}

	if useSMSFallback || !wsAvailable {
		if smsEnabled {
			go am.sendSMS(alert)
			channelsUsed = append(channelsUsed, "sms")
			log.Printf("[ALERT] Alert %s sent via SMS (fallback: %v)", alert.AlertID, useSMSFallback)
		} else if !delivered {
			log.Printf("[ALERT] WARNING: No delivery channel available for alert %s!", alert.AlertID)
		}
	}

	if !delivered && smsEnabled {
		buffered := &BufferedAlert{
			Alert:      alert,
			QueuedAt:   time.Now(),
			Delivered:  false,
			Channel:    "buffer",
			RetryCount: 0,
		}
		am.mu.Lock()
		if len(am.alertBuffer) < am.bufferMaxSize {
			am.alertBuffer = append(am.alertBuffer, buffered)
			log.Printf("[ALERT] Alert %s buffered for later delivery (buffer size: %d)",
				alert.AlertID, len(am.alertBuffer))
		} else {
			log.Printf("[ALERT] WARNING: Alert buffer full, dropping alert %s", alert.AlertID)
		}
		am.mu.Unlock()
	}

	am.mu.Lock()
	alert.DeliveryChannels = channelsUsed
	alert.Delivered = delivered || smsEnabled
	am.mu.Unlock()
}

func (am *AlertManager) flushBufferedAlerts() {
	am.mu.Lock()
	buffered := make([]*BufferedAlert, len(am.alertBuffer))
	copy(buffered, am.alertBuffer)
	am.alertBuffer = am.alertBuffer[:0]
	am.mu.Unlock()

	deliveredCount := 0
	for _, ba := range buffered {
		if ba.Delivered {
			continue
		}

		if am.wsServer != nil {
			err := am.wsServer.BroadcastAlert(ba.Alert)
			if err != nil {
				log.Printf("[ALERT] Failed to flush buffered alert %s: %v", ba.Alert.AlertID, err)
			} else {
				ba.Delivered = true
				ba.DeliveredAt = time.Now()
				ba.Channel = "websocket"
				deliveredCount++
				log.Printf("[ALERT] Buffered alert %s flushed via WebSocket", ba.Alert.AlertID)
			}
		}
	}

	log.Printf("[ALERT] Flush complete: %d/%d alerts delivered", deliveredCount, len(buffered))
}

func (am *AlertManager) resendBufferedAlertsViaSMS() {
	if !am.smsConfig.Enabled {
		log.Printf("[ALERT] SMS not enabled, cannot resend buffered alerts")
		return
	}

	am.mu.RLock()
	buffered := make([]*BufferedAlert, len(am.alertBuffer))
	copy(buffered, am.alertBuffer)
	am.mu.RUnlock()

	sentCount := 0
	for _, ba := range buffered {
		if ba.Delivered {
			continue
		}

		ba.RetryCount++
		if ba.RetryCount > 3 {
			log.Printf("[ALERT] Alert %s exceeded max retries, discarding", ba.Alert.AlertID)
			continue
		}

		go am.sendSMS(ba.Alert)
		ba.Delivered = true
		ba.DeliveredAt = time.Now()
		ba.Channel = "sms_fallback"
		sentCount++
		log.Printf("[ALERT] Buffered alert %s resent via SMS (attempt %d)",
			ba.Alert.AlertID, ba.RetryCount)
	}

	am.mu.Lock()
	am.alertBuffer = am.alertBuffer[:0]
	am.mu.Unlock()

	log.Printf("[ALERT] Buffered alerts resent via SMS: %d sent", sentCount)
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

	channelStatus := am.GetChannelStatus()
	channelStatusStr := "unknown"
	switch channelStatus {
	case ChannelPrimary:
		channelStatusStr = "primary"
	case ChannelDegraded:
		channelStatusStr = "degraded"
	case ChannelFailed:
		channelStatusStr = "failed"
	}

	return map[string]interface{}{
		"active_alerts":      len(am.activeAlerts),
		"level1_alerts":      level1Count,
		"level2_alerts":      level2Count,
		"total_sensors":      len(am.sensorStatus),
		"online_sensors":     onlineSensors,
		"offline_sensors":    len(am.sensorStatus) - onlineSensors,
		"total_fans":       len(am.fanStatus),
		"channel_status":      channelStatusStr,
		"ws_available":      am.wsAvailable,
		"ws_client_count":    am.wsClientCount,
		"ws_last_change":    am.wsLastStateChange,
		"buffer_size":        len(am.alertBuffer),
		"channel_switches":   am.channelSwitchCount,
		"sms_fallback_enabled": am.smsFallbackEnabled,
	}
}

func (am *AlertManager) SetSMSFallbackEnabled(enabled bool) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.smsFallbackEnabled = enabled
	log.Printf("[ALERT] SMS fallback %s", map[bool]string{true: "enabled", false: "disabled"}[enabled])
}

func (am *AlertManager) SetSMSFallbackLevel(level int) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.smsFallbackLevel = level
	log.Printf("[ALERT] SMS fallback level set to %d", level)
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
