package alarm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"sewage-treatment-system/internal/config"
	"sewage-treatment-system/internal/influxdb"
	"sewage-treatment-system/internal/models"
	"sewage-treatment-system/internal/websocket"
)

type PushResult struct {
	WebSocketSuccess bool
	SMSSent          bool
	ChannelsUsed     []string
	Errors           []string
}

type Manager struct {
	cfg           *config.AlarmConfig
	logger        *zap.Logger
	influxClient  *influxdb.Client
	wsServer      *websocket.Server
	mu            sync.RWMutex
	activeAlarms  map[string]*models.Alarm
	level1State   *level1AlarmState
	level2State   *level2AlarmState
	sensorInfo    map[string]models.SensorInfo
	pushHistory   map[string]*PushResult
}

type level1AlarmState struct {
	nh3ExceedStartTime   time.Time
	tnExceedStartTime    time.Time
	nh3CurrentlyExceed   bool
	tnCurrentlyExceed    bool
	mu                   sync.Mutex
}

type level2AlarmState struct {
	offlineSensors map[string]time.Time
	fanFailures    map[string]time.Time
	mu             sync.Mutex
}

func NewManager(
	cfg *config.AlarmConfig,
	influxClient *influxdb.Client,
	wsServer *websocket.Server,
	logger *zap.Logger,
	sensorInfo map[string]models.SensorInfo,
) *Manager {
	return &Manager{
		cfg:          cfg,
		logger:       logger,
		influxClient: influxClient,
		wsServer:     wsServer,
		activeAlarms: make(map[string]*models.Alarm),
		level1State: &level1AlarmState{
			nh3CurrentlyExceed: false,
			tnCurrentlyExceed:  false,
		},
		level2State: &level2AlarmState{
			offlineSensors: make(map[string]time.Time),
			fanFailures:    make(map[string]time.Time),
		},
		sensorInfo:  sensorInfo,
		pushHistory: make(map[string]*PushResult),
	}
}

func (am *Manager) pushWithFailover(alarm *models.Alarm) *PushResult {
	result := &PushResult{
		ChannelsUsed: make([]string, 0),
		Errors:       make([]string, 0),
	}

	wsClientCount := am.wsServer.GetClientCount()
	wsAvailable := wsClientCount > 0

	forceDualChannel := (alarm.Level == 1)

	if wsAvailable {
		if err := am.wsServer.BroadcastAlarm(alarm); err != nil {
			result.Errors = append(result.Errors, "websocket: "+err.Error())
		} else {
			result.WebSocketSuccess = true
			result.ChannelsUsed = append(result.ChannelsUsed, "websocket")
		}
	} else {
		result.Errors = append(result.Errors, "websocket: no clients connected")
	}

	shouldSendSMS := forceDualChannel || !result.WebSocketSuccess

	if shouldSendSMS && am.cfg.SMS.Enabled {
		am.sendSMS(alarm)
		result.SMSSent = true
		result.ChannelsUsed = append(result.ChannelsUsed, "sms")
	} else if shouldSendSMS && !am.cfg.SMS.Enabled {
		result.Errors = append(result.Errors, "sms: not configured")

		if !result.WebSocketSuccess {
			am.logger.Error("CRITICAL: All alarm channels failed!",
				zap.String("alarm_id", alarm.ID),
				zap.Int("level", alarm.Level),
				zap.String("message", alarm.Message))
		}
	}

	return result
}

func (am *Manager) TriggerAlarm(level int, alarmType string, message string, value, threshold float64) *models.Alarm {
	alarm := &models.Alarm{
		ID:        uuid.New().String(),
		Level:     level,
		Type:      alarmType,
		Message:   message,
		Value:     value,
		Threshold: threshold,
		Timestamp: time.Now(),
		ACK:       false,
	}

	am.mu.Lock()
	am.activeAlarms[alarm.ID] = alarm
	am.mu.Unlock()

	if err := am.influxClient.WriteAlarm(alarm); err != nil {
		am.logger.Error("Failed to write alarm to influxdb", zap.Error(err))
	}

	pushResult := am.pushWithFailover(alarm)

	am.mu.Lock()
	am.pushHistory[alarm.ID] = pushResult
	am.mu.Unlock()

	am.logger.Warn("Alarm triggered",
		zap.Int("level", level),
		zap.String("type", alarmType),
		zap.String("message", message),
		zap.Float64("value", value),
		zap.Float64("threshold", threshold),
		zap.Bool("ws_success", pushResult.WebSocketSuccess),
		zap.Bool("sms_sent", pushResult.SMSSent),
		zap.Strings("channels", pushResult.ChannelsUsed))

	if !pushResult.WebSocketSuccess && !pushResult.SMSSent {
		am.logger.Error("Alarm delivery failed - ALL CHANNELS DOWN!",
			zap.String("alarm_id", alarm.ID),
			zap.Int("level", level),
			zap.Strings("errors", pushResult.Errors))
	}

	return alarm
}

func (am *Manager) sendSMS(alarm *models.Alarm) {
	if !am.cfg.SMS.Enabled {
		return
	}

	levelText := "一级"
	if alarm.Level == 2 {
		levelText = "二级"
	}

	message := fmt.Sprintf("[污水处理厂%s告警] %s 当前值: %.2f, 阈值: %.2f, 时间: %s",
		levelText, alarm.Message, alarm.Value, alarm.Threshold,
		alarm.Timestamp.Format("2006-01-02 15:04:05"))

	for _, recipient := range am.cfg.SMS.Recipients {
		go func(phone string) {
			if err := am.sendSMSRequest(phone, message); err != nil {
				am.logger.Error("Failed to send SMS",
					zap.String("phone", phone),
					zap.Error(err))
			} else {
				am.logger.Info("SMS sent successfully",
					zap.String("phone", phone),
					zap.String("level", levelText))
			}
		}(recipient)
	}
}

func (am *Manager) sendSMSRequest(phone, message string) error {
	payload := map[string]interface{}{
		"apikey":  am.cfg.SMS.APIKey,
		"mobile":  phone,
		"content": message,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := http.Post(am.cfg.SMS.APIURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SMS API returned status: %d", resp.StatusCode)
	}

	return nil
}

func (am *Manager) CheckLevel1Alarms(nh3Effluent, tnEffluent float64) {
	am.level1State.mu.Lock()
	defer am.level1State.mu.Unlock()

	now := time.Now()
	durationThreshold := time.Duration(am.cfg.Level1.DurationMinutes) * time.Minute

	if nh3Effluent > am.cfg.Level1.NH3Threshold {
		if !am.level1State.nh3CurrentlyExceed {
			am.level1State.nh3ExceedStartTime = now
			am.level1State.nh3CurrentlyExceed = true
			am.logger.Info("NH3 exceed threshold started",
				zap.Float64("current", nh3Effluent),
				zap.Float64("threshold", am.cfg.Level1.NH3Threshold))
		} else if now.Sub(am.level1State.nh3ExceedStartTime) >= durationThreshold {
			am.TriggerAlarm(1, "nh3_exceed",
				fmt.Sprintf("出水氨氮持续超标超过%d分钟", am.cfg.Level1.DurationMinutes),
				nh3Effluent, am.cfg.Level1.NH3Threshold)
			am.level1State.nh3ExceedStartTime = now
		}
	} else {
		if am.level1State.nh3CurrentlyExceed {
			am.logger.Info("NH3 back to normal", zap.Float64("current", nh3Effluent))
		}
		am.level1State.nh3CurrentlyExceed = false
	}

	if tnEffluent > am.cfg.Level1.TNThreshold {
		if !am.level1State.tnCurrentlyExceed {
			am.level1State.tnExceedStartTime = now
			am.level1State.tnCurrentlyExceed = true
			am.logger.Info("TN exceed threshold started",
				zap.Float64("current", tnEffluent),
				zap.Float64("threshold", am.cfg.Level1.TNThreshold))
		} else if now.Sub(am.level1State.tnExceedStartTime) >= durationThreshold {
			am.TriggerAlarm(1, "tn_exceed",
				fmt.Sprintf("出水总氮持续超标超过%d分钟", am.cfg.Level1.DurationMinutes),
				tnEffluent, am.cfg.Level1.TNThreshold)
			am.level1State.tnExceedStartTime = now
		}
	} else {
		if am.level1State.tnCurrentlyExceed {
			am.logger.Info("TN back to normal", zap.Float64("current", tnEffluent))
		}
		am.level1State.tnCurrentlyExceed = false
	}
}

func (am *Manager) CheckSensorOffline(sensorID string, lastSeen time.Time) {
	am.level2State.mu.Lock()
	defer am.level2State.mu.Unlock()

	offlineDuration := time.Duration(am.cfg.Level2.SensorOfflineMinutes) * time.Minute
	now := time.Now()

	if now.Sub(lastSeen) > offlineDuration {
		if _, exists := am.level2State.offlineSensors[sensorID]; !exists {
			am.level2State.offlineSensors[sensorID] = now
			sensorType := "未知"
			if info, ok := am.sensorInfo[sensorID]; ok {
				sensorType = string(info.Type)
			}
			am.TriggerAlarm(2, "sensor_offline",
				fmt.Sprintf("传感器离线: %s (%s)", sensorID, sensorType),
				math.Round(now.Sub(lastSeen).Minutes()),
				float64(am.cfg.Level2.SensorOfflineMinutes))
		}
	} else {
		delete(am.level2State.offlineSensors, sensorID)
	}
}

func (am *Manager) CheckFanFailure(fanID string, isRunning bool) {
	if !am.cfg.Level2.FanFailureCheck {
		return
	}

	am.level2State.mu.Lock()
	defer am.level2State.mu.Unlock()

	now := time.Now()

	if !isRunning {
		if _, exists := am.level2State.fanFailures[fanID]; !exists {
			am.level2State.fanFailures[fanID] = now
			am.TriggerAlarm(2, "fan_failure",
				fmt.Sprintf("曝气风机故障: %s", fanID),
				0, 1)
		}
	} else {
		delete(am.level2State.fanFailures, fanID)
	}
}

func (am *Manager) AlarmCheckLoop(stopCh <-chan struct{}, getLatestValues func() (float64, float64)) {
	ticker := time.NewTicker(time.Duration(am.cfg.Level1.CheckInterval) * time.Second)
	defer ticker.Stop()

	am.logger.Info("Alarm check loop started",
		zap.Int("interval_sec", am.cfg.Level1.CheckInterval))

	for {
		select {
		case <-stopCh:
			am.logger.Info("Alarm check loop stopped")
			return
		case <-ticker.C:
			nh3, tn := getLatestValues()
			am.CheckLevel1Alarms(nh3, tn)
		}
	}
}

func (am *Manager) AcknowledgeAlarm(alarmID string) bool {
	am.mu.Lock()
	defer am.mu.Unlock()

	if alarm, ok := am.activeAlarms[alarmID]; ok {
		alarm.ACK = true
		return true
	}
	return false
}

func (am *Manager) GetActiveAlarms() []*models.Alarm {
	am.mu.RLock()
	defer am.mu.RUnlock()

	var alarms []*models.Alarm
	for _, alarm := range am.activeAlarms {
		alarms = append(alarms, alarm)
	}
	return alarms
}

func (am *Manager) GetActiveAlarmsByLevel(level int) []*models.Alarm {
	am.mu.RLock()
	defer am.mu.RUnlock()

	var alarms []*models.Alarm
	for _, alarm := range am.activeAlarms {
		if alarm.Level == level {
			alarms = append(alarms, alarm)
		}
	}
	return alarms
}

func (am *Manager) ClearOldAlarms(before time.Time) int {
	am.mu.Lock()
	defer am.mu.Unlock()

	count := 0
	for id, alarm := range am.activeAlarms {
		if alarm.Timestamp.Before(before) && alarm.ACK {
			delete(am.activeAlarms, id)
			count++
		}
	}
	return count
}

func (am *Manager) GetOfflineSensors() []string {
	am.level2State.mu.Lock()
	defer am.level2State.mu.Unlock()

	var sensors []string
	for id := range am.level2State.offlineSensors {
		sensors = append(sensors, id)
	}
	return sensors
}

func (am *Manager) GetFanFailures() []string {
	am.level2State.mu.Lock()
	defer am.level2State.mu.Unlock()

	var fans []string
	for id := range am.level2State.fanFailures {
		fans = append(fans, id)
	}
	return fans
}
