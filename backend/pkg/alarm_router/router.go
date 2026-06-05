package alarm_router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"sewage-plant-system/pkg/models"
	sc "sewage-plant-system/pkg/sensor_collector"
	ws "sewage-plant-system/pkg/websocket"
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
	AlertTypeInvalidData    AlertType = "invalid_data"
)

type ThresholdConfig struct {
	NH3Threshold     float64       `mapstructure:"nh3_threshold"`
	TNThreshold      float64       `mapstructure:"tn_threshold"`
	Duration         time.Duration `mapstructure:"duration"`
	OfflineTimeout   time.Duration `mapstructure:"offline_timeout"`
	FanFaultTimeout  time.Duration `mapstructure:"fan_fault_timeout"`
}

type SMSConfig struct {
	Enabled    bool     `mapstructure:"enabled"`
	APIURL     string   `mapstructure:"api_url"`
	APIKey     string   `mapstructure:"api_key"`
	Recipients []string `mapstructure:"recipients"`
}

type Config struct {
	Thresholds         ThresholdConfig `mapstructure:"thresholds"`
	SMS                SMSConfig       `mapstructure:"sms"`
	SMSFallbackEnabled  bool            `mapstructure:"sms_fallback_enabled"`
	SMSFallbackLevel   int             `mapstructure:"sms_fallback_level"`
	BufferMaxSize      int             `mapstructure:"buffer_max_size"`
	CheckInterval      time.Duration   `mapstructure:"check_interval"`
}

type SensorStatus struct {
	SensorID    string
	LastValue   float64
	LastUpdate  time.Time
	IsOnline    bool
	ExceedStart *time.Time
	SensorType  models.SensorType
	Location    string
}

type FanStatus struct {
	FanID      string
	Status     string
	LastUpdate time.Time
	FaultStart *time.Time
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

type AlarmRouter struct {
	cfg                 Config
	alerts              []*models.Alert
	sensorStatus        map[string]*SensorStatus
	fanStatus           map[string]*FanStatus
	mu                  sync.RWMutex
	wsServer            *ws.Server
	activeAlerts        map[string]*models.Alert
	alertHistory        []*models.Alert
	wsAvailable         bool
	wsClientCount       int
	wsLastStateChange   time.Time
	alertBuffer         []*BufferedAlert
	channelSwitchCount  int

	validDataChan       <-chan *sc.ValidatedSensorData
	statusEventChan     <-chan *sc.SensorStatusEvent
	plcStatusChan       <-chan *models.PLCStatus
	alertOutChan        chan<- *models.Alert
	statusOutChan       chan<- map[string]interface{}
}

func New(cfg Config, wsServer *ws.Server, validDataChan <-chan *sc.ValidatedSensorData, statusEventChan <-chan *sc.SensorStatusEvent, plcStatusChan <-chan *models.PLCStatus, alertOutChan chan<- *models.Alert, statusOutChan chan<- map[string]interface{}) *AlarmRouter {
	if cfg.BufferMaxSize == 0 {
		cfg.BufferMaxSize = 100
	}
	if cfg.SMSFallbackLevel == 0 {
		cfg.SMSFallbackLevel = 1
	}
	if cfg.Thresholds.Duration == 0 {
		cfg.Thresholds.Duration = 30 * time.Minute
	}
	if cfg.Thresholds.OfflineTimeout == 0 {
		cfg.Thresholds.OfflineTimeout = 5 * time.Minute
	}
	if cfg.Thresholds.FanFaultTimeout == 0 {
		cfg.Thresholds.FanFaultTimeout = 60 * time.Second
	}
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = 30 * time.Second
	}

	ar := &AlarmRouter{
		cfg:              cfg,
		sensorStatus:     make(map[string]*SensorStatus),
		fanStatus:        make(map[string]*FanStatus),
		activeAlerts:     make(map[string]*models.Alert),
		alertHistory:     make([]*models.Alert, 0, 1000),
		alertBuffer:      make([]*BufferedAlert, 0, cfg.BufferMaxSize),
		wsServer:         wsServer,
		validDataChan:    validDataChan,
		statusEventChan:  statusEventChan,
		plcStatusChan:    plcStatusChan,
		alertOutChan:     alertOutChan,
		statusOutChan:    statusOutChan,
	}

	if wsServer != nil {
		wsServer.SetStatusChangeCallback(ar.OnWSStatusChange)
		ar.wsAvailable = wsServer.HasClients()
		log.Printf("[ALARM] WebSocket status initialized: available=%v", ar.wsAvailable)
	}

	return ar
}

func (ar *AlarmRouter) Start() {
	go ar.processSensorData()
	go ar.processStatusEvents()
	go ar.processPLCStatus()
	go ar.checkLoop()
	go ar.statusBroadcastLoop()
}

func (ar *AlarmRouter) processSensorData() {
	for data := range ar.validDataChan {
		ar.UpdateSensorData(data.SensorID, data.Value, data.Type, data.Timestamp, data.Location)
	}
}

func (ar *AlarmRouter) processStatusEvents() {
	for event := range ar.statusEventChan {
		switch event.EventType {
		case "offline":
			ar.handleSensorOffline(event.SensorID, event.Timestamp)
		case "online":
			ar.handleSensorOnline(event.SensorID)
		case "invalid_data":
			ar.handleInvalidData(event.SensorID, event.Value, event.SensorType, event.Timestamp)
		}
	}
}

func (ar *AlarmRouter) processPLCStatus() {
	for status := range ar.plcStatusChan {
		ar.UpdateFanStatus(status.DeviceID, status.Status, status.Timestamp)
	}
}

func (ar *AlarmRouter) checkLoop() {
	ticker := time.NewTicker(ar.cfg.CheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		ar.CheckSensorOffline(time.Now())
		ar.broadcastStatus()
	}
}

func (ar *AlarmRouter) statusBroadcastLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		ar.broadcastStatus()
	}
}

func (ar *AlarmRouter) UpdateSensorData(sensorID string, value float64, sensorType models.SensorType, timestamp time.Time, location string) {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	status, exists := ar.sensorStatus[sensorID]
	if !exists {
		status = &SensorStatus{
			SensorID:   sensorID,
			IsOnline:   true,
			SensorType: sensorType,
			Location:   location,
		}
		ar.sensorStatus[sensorID] = status
	}

	status.LastValue = value
	status.LastUpdate = timestamp

	if !status.IsOnline {
		status.IsOnline = true
		ar.resolveAlert(sensorID, AlertTypeSensorOffline)
	}

	ar.checkLevel1Alerts(sensorID, value, sensorType, timestamp)
}

func (ar *AlarmRouter) checkLevel1Alerts(sensorID string, value float64, sensorType models.SensorType, timestamp time.Time) {
	var alertType AlertType
	var threshold float64
	var exceed bool

	switch sensorType {
	case models.SensorTypeNH3:
		alertType = AlertTypeEffluentNH3
		threshold = ar.cfg.Thresholds.NH3Threshold
		exceed = value > threshold
	case models.SensorTypeNO3:
		alertType = AlertTypeEffluentTN
		threshold = ar.cfg.Thresholds.TNThreshold
		exceed = value > threshold
	default:
		return
	}

	status := ar.sensorStatus[sensorID]

	if exceed {
		if status.ExceedStart == nil {
			start := timestamp
			status.ExceedStart = &start
		} else if timestamp.Sub(*status.ExceedStart) >= ar.cfg.Thresholds.Duration {
			alertKey := fmt.Sprintf("%s_%s", sensorID, alertType)
			if _, exists := ar.activeAlerts[alertKey]; !exists {
				ar.createAlert(
					Level1,
					alertType,
					fmt.Sprintf("出水%s超标", getSensorTypeName(sensorType)),
					fmt.Sprintf("传感器%s检测到%s浓度%.2f mg/L，已连续超过阈值%.2f mg/L达%d分钟",
						sensorID, getSensorTypeName(sensorType), value, threshold, int(ar.cfg.Thresholds.Duration.Minutes())),
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
		ar.resolveAlert(sensorID, alertType)
		delete(ar.activeAlerts, alertKey)
	}
}

func (ar *AlarmRouter) CheckSensorOffline(timestamp time.Time) {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	for sensorID, status := range ar.sensorStatus {
		if status.IsOnline && timestamp.Sub(status.LastUpdate) > ar.cfg.Thresholds.OfflineTimeout {
			status.IsOnline = false
			log.Printf("[ALARM] Sensor %s is offline (timeout: %v)", sensorID, ar.cfg.Thresholds.OfflineTimeout)
			ar.handleSensorOffline(sensorID, timestamp)
		}
	}
}

func (ar *AlarmRouter) handleSensorOffline(sensorID string, timestamp time.Time) {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	alertKey := fmt.Sprintf("%s_%s", sensorID, AlertTypeSensorOffline)
	if _, exists := ar.activeAlerts[alertKey]; !exists {
		ar.createAlert(
			Level2,
			AlertTypeSensorOffline,
			"传感器离线",
			fmt.Sprintf("传感器%s已离线超过%d秒", sensorID, int(ar.cfg.Thresholds.OfflineTimeout.Seconds())),
			sensorID,
			0,
			0,
			timestamp,
		)
	}
}

func (ar *AlarmRouter) handleSensorOnline(sensorID string) {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	status, exists := ar.sensorStatus[sensorID]
	if exists {
		status.IsOnline = true
		ar.resolveAlert(sensorID, AlertTypeSensorOffline)
	}
}

func (ar *AlarmRouter) handleInvalidData(sensorID string, value float64, sensorType models.SensorType, timestamp time.Time) {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	alertKey := fmt.Sprintf("%s_%s", sensorID, AlertTypeInvalidData)
	if _, exists := ar.activeAlerts[alertKey]; !exists {
		ar.createAlert(
			Level2,
			AlertTypeInvalidData,
			"传感器数据异常",
			fmt.Sprintf("传感器%s上报异常数据: %.2f", sensorID, value),
			sensorID,
			value,
			0,
			timestamp,
		)
	}
}

func (ar *AlarmRouter) UpdateFanStatus(fanID string, status string, timestamp time.Time) {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	fs, exists := ar.fanStatus[fanID]
	if !exists {
		fs = &FanStatus{FanID: fanID}
		ar.fanStatus[fanID] = fs
	}

	fs.Status = status
	fs.LastUpdate = timestamp

	if status == "fault" || status == "error" || status == "offline" {
		if fs.FaultStart == nil {
			start := timestamp
			fs.FaultStart = &start
		} else if timestamp.Sub(*fs.FaultStart) >= ar.cfg.Thresholds.FanFaultTimeout {
			alertKey := fmt.Sprintf("%s_%s", fanID, AlertTypeFanFault)
			if _, exists := ar.activeAlerts[alertKey]; !exists {
				ar.createAlert(
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
		delete(ar.activeAlerts, alertKey)
	}
}

func (ar *AlarmRouter) createAlert(level AlertLevel, alertType AlertType, title, message, sensorID string, value, threshold float64, timestamp time.Time) {
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
	ar.activeAlerts[alertKey] = alert
	ar.alertHistory = append(ar.alertHistory, alert)
	if len(ar.alertHistory) > 1000 {
		ar.alertHistory = ar.alertHistory[1:]
	}

	log.Printf("[ALARM] Level %d - %s: %s", level, title, message)

	ar.deliverAlert(alert)

	select {
	case ar.alertOutChan <- alert:
	default:
	}
}

func (ar *AlarmRouter) deliverAlert(alert *models.Alert) {
	ar.mu.RLock()
	wsAvailable := ar.wsAvailable
	smsEnabled := ar.cfg.SMS.Enabled
	useSMSFallback := ar.shouldUseSMSFallback(alert.Level)
	ar.mu.RUnlock()

	delivered := false
	channelsUsed := []string{}

	if wsAvailable && ar.wsServer != nil {
		err := ar.wsServer.BroadcastAlert(alert)
		if err != nil {
			log.Printf("[ALARM] WebSocket broadcast failed for alert %s: %v", alert.AlertID, err)
		} else {
			delivered = true
			channelsUsed = append(channelsUsed, "websocket")
			log.Printf("[ALARM] Alert %s delivered via WebSocket", alert.AlertID)
		}
	}

	if useSMSFallback || !wsAvailable {
		if smsEnabled {
			go ar.sendSMS(alert)
			channelsUsed = append(channelsUsed, "sms")
			log.Printf("[ALARM] Alert %s sent via SMS (fallback: %v)", alert.AlertID, useSMSFallback)
		} else if !delivered {
			log.Printf("[ALARM] WARNING: No delivery channel available for alert %s!", alert.AlertID)
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
		ar.mu.Lock()
		if len(ar.alertBuffer) < ar.cfg.BufferMaxSize {
			ar.alertBuffer = append(ar.alertBuffer, buffered)
			log.Printf("[ALARM] Alert %s buffered for later delivery (buffer size: %d)",
				alert.AlertID, len(ar.alertBuffer))
		} else {
			log.Printf("[ALARM] WARNING: Alert buffer full, dropping alert %s", alert.AlertID)
		}
		ar.mu.Unlock()
	}

	ar.mu.Lock()
	alert.DeliveryChannels = channelsUsed
	alert.Delivered = delivered || smsEnabled
	ar.mu.Unlock()
}

func (ar *AlarmRouter) shouldUseSMSFallback(alertLevel int) bool {
	ar.mu.RLock()
	defer ar.mu.RUnlock()

	if !ar.cfg.SMSFallbackEnabled {
		return false
	}

	if !ar.wsAvailable {
		return true
	}

	if alertLevel <= ar.cfg.SMSFallbackLevel && ar.wsAvailable {
		return false
	}

	return false
}

func (ar *AlarmRouter) OnWSStatusChange(connected bool, clientCount int) {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	oldStatus := ar.wsAvailable
	ar.wsAvailable = connected
	ar.wsClientCount = clientCount
	ar.wsLastStateChange = time.Now()

	if oldStatus != connected {
		ar.channelSwitchCount++
		log.Printf("[ALARM] WebSocket status changed: %v -> %v, clients=%d, total_switches=%d",
			oldStatus, connected, clientCount, ar.channelSwitchCount)

		if connected {
			log.Printf("[ALARM] WebSocket reconnected, flushing buffered alerts (%d pending)", len(ar.alertBuffer))
			go ar.flushBufferedAlerts()
		} else {
			log.Printf("[ALARM] WebSocket disconnected, enabling SMS fallback for all alerts")
			go ar.resendBufferedAlertsViaSMS()
		}
	}
}

func (ar *AlarmRouter) flushBufferedAlerts() {
	ar.mu.Lock()
	buffered := make([]*BufferedAlert, len(ar.alertBuffer))
	copy(buffered, ar.alertBuffer)
	ar.alertBuffer = ar.alertBuffer[:0]
	ar.mu.Unlock()

	deliveredCount := 0
	for _, ba := range buffered {
		if ba.Delivered {
			continue
		}

		if ar.wsServer != nil {
			err := ar.wsServer.BroadcastAlert(ba.Alert)
			if err != nil {
				log.Printf("[ALARM] Failed to flush buffered alert %s: %v", ba.Alert.AlertID, err)
			} else {
				ba.Delivered = true
				ba.DeliveredAt = time.Now()
				ba.Channel = "websocket"
				deliveredCount++
				log.Printf("[ALARM] Buffered alert %s flushed via WebSocket", ba.Alert.AlertID)
			}
		}
	}

	log.Printf("[ALARM] Flush complete: %d/%d alerts delivered", deliveredCount, len(buffered))
}

func (ar *AlarmRouter) resendBufferedAlertsViaSMS() {
	if !ar.cfg.SMS.Enabled {
		log.Printf("[ALARM] SMS not enabled, cannot resend buffered alerts")
		return
	}

	ar.mu.RLock()
	buffered := make([]*BufferedAlert, len(ar.alertBuffer))
	copy(buffered, ar.alertBuffer)
	ar.mu.RUnlock()

	sentCount := 0
	for _, ba := range buffered {
		if ba.Delivered {
			continue
		}

		ba.RetryCount++
		if ba.RetryCount > 3 {
			log.Printf("[ALARM] Alert %s exceeded max retries, discarding", ba.Alert.AlertID)
			continue
		}

		go ar.sendSMS(ba.Alert)
		ba.Delivered = true
		ba.DeliveredAt = time.Now()
		ba.Channel = "sms_fallback"
		sentCount++
		log.Printf("[ALARM] Buffered alert %s resent via SMS (attempt %d)",
			ba.Alert.AlertID, ba.RetryCount)
	}

	ar.mu.Lock()
	ar.alertBuffer = ar.alertBuffer[:0]
	ar.mu.Unlock()

	log.Printf("[ALARM] Buffered alerts resent via SMS: %d sent", sentCount)
}

func (ar *AlarmRouter) sendSMS(alert *models.Alert) {
	if !ar.cfg.SMS.Enabled {
		return
	}

	for _, recipient := range ar.cfg.SMS.Recipients {
		message := fmt.Sprintf("[污水处理厂告警] 级别%d: %s - %s", alert.Level, alert.Title, alert.Message)
		payload := map[string]interface{}{
			"api_key":  ar.cfg.SMS.APIKey,
			"to":       recipient,
			"message":  message,
			"priority": alert.Level,
		}

		go func(recipient string) {
			data, _ := json.Marshal(payload)
			resp, err := http.Post(ar.cfg.SMS.APIURL, "application/json", bytes.NewBuffer(data))
			if err != nil {
				log.Printf("Failed to send SMS to %s: %v", recipient, err)
				return
			}
			defer resp.Body.Close()
			log.Printf("SMS sent to %s, status: %d", recipient, resp.StatusCode)
		}(recipient)
	}
}

func (ar *AlarmRouter) resolveAlert(sensorID string, alertType AlertType) {
	alertKey := fmt.Sprintf("%s_%s", sensorID, alertType)
	delete(ar.activeAlerts, alertKey)
}

func (ar *AlarmRouter) GetChannelStatus() ChannelStatus {
	ar.mu.RLock()
	defer ar.mu.RUnlock()

	if ar.wsAvailable {
		return ChannelPrimary
	} else if ar.cfg.SMS.Enabled {
		return ChannelDegraded
	}
	return ChannelFailed
}

func (ar *AlarmRouter) GetActiveAlerts() []*models.Alert {
	ar.mu.RLock()
	defer ar.mu.RUnlock()

	alerts := make([]*models.Alert, 0, len(ar.activeAlerts))
	for _, alert := range ar.activeAlerts {
		alerts = append(alerts, alert)
	}
	return alerts
}

func (ar *AlarmRouter) GetAlertHistory(limit int) []*models.Alert {
	ar.mu.RLock()
	defer ar.mu.RUnlock()

	start := 0
	if len(ar.alertHistory) > limit {
		start = len(ar.alertHistory) - limit
	}
	return ar.alertHistory[start:]
}

func (ar *AlarmRouter) AcknowledgeAlert(alertID string) bool {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	for key, alert := range ar.activeAlerts {
		if alert.AlertID == alertID {
			alert.Acknowledged = true
			delete(ar.activeAlerts, key)
			return true
		}
	}

	for _, alert := range ar.alertHistory {
		if alert.AlertID == alertID {
			alert.Acknowledged = true
			return true
		}
	}

	return false
}

func (ar *AlarmRouter) broadcastStatus() {
	status := ar.GetStatus()
	select {
	case ar.statusOutChan <- status:
	default:
	}
}

func (ar *AlarmRouter) GetStatus() map[string]interface{} {
	ar.mu.RLock()
	defer ar.mu.RUnlock()

	level1Count := 0
	level2Count := 0
	for _, alert := range ar.activeAlerts {
		if alert.Level == int(Level1) {
			level1Count++
		} else if alert.Level == int(Level2) {
			level2Count++
		}
	}

	onlineSensors := 0
	for _, status := range ar.sensorStatus {
		if status.IsOnline {
			onlineSensors++
		}
	}

	channelStatus := ar.GetChannelStatus()
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
		"active_alerts":       len(ar.activeAlerts),
		"level1_alerts":       level1Count,
		"level2_alerts":       level2Count,
		"total_sensors":       len(ar.sensorStatus),
		"online_sensors":      onlineSensors,
		"offline_sensors":     len(ar.sensorStatus) - onlineSensors,
		"total_fans":          len(ar.fanStatus),
		"channel_status":      channelStatusStr,
		"ws_available":        ar.wsAvailable,
		"ws_client_count":     ar.wsClientCount,
		"ws_last_change":      ar.wsLastStateChange,
		"buffer_size":         len(ar.alertBuffer),
		"channel_switches":    ar.channelSwitchCount,
		"sms_fallback_enabled": ar.cfg.SMSFallbackEnabled,
	}
}

func (ar *AlarmRouter) SetSMSFallbackEnabled(enabled bool) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	ar.cfg.SMSFallbackEnabled = enabled
	log.Printf("[ALARM] SMS fallback %s", map[bool]string{true: "enabled", false: "disabled"}[enabled])
}

func (ar *AlarmRouter) SetSMSFallbackLevel(level int) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	ar.cfg.SMSFallbackLevel = level
	log.Printf("[ALARM] SMS fallback level set to %d", level)
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

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
