package alarm

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"sewage-treatment-system/internal/config"
	"sewage-treatment-system/internal/influxdb"
	"sewage-treatment-system/internal/messages"
	"sewage-treatment-system/internal/models"
	"sewage-treatment-system/internal/websocket"
)

type PushResult struct {
	WebSocketSuccess bool
	SMSSent          bool
	ChannelsUsed     []string
	Errors           []string
}

type AlarmRouter struct {
	cfg          *config.AlarmConfig
	logger       *zap.Logger
	influxClient *influxdb.Client
	wsServer     *websocket.Server
	sensorInfo   map[string]models.SensorInfo

	alarmInCh <-chan *messages.AlarmMessage

	mu           sync.RWMutex
	activeAlarms map[string]*models.Alarm
	pushHistory  map[string]*PushResult

	level1State *level1AlarmState
	level2State *level2AlarmState
}

type level1AlarmState struct {
	nh3CurrentlyExceed bool
	nh3ExceedStart     time.Time
	tnCurrentlyExceed  bool
	tnExceedStart      time.Time
}

type level2AlarmState struct {
	offlineSensors map[string]time.Time
	fanFailures    map[string]time.Time
}

type AlarmRouterChannels struct {
	AlarmIn chan *messages.AlarmMessage
}

func NewAlarmRouter(
	cfg *config.AlarmConfig,
	influxClient *influxdb.Client,
	wsServer *websocket.Server,
	logger *zap.Logger,
	sensorInfo map[string]models.SensorInfo,
	channels *AlarmRouterChannels,
) *AlarmRouter {
	return &AlarmRouter{
		cfg:          cfg,
		logger:       logger,
		influxClient: influxClient,
		wsServer:     wsServer,
		sensorInfo:   sensorInfo,
		alarmInCh:    channels.AlarmIn,
		activeAlarms: make(map[string]*models.Alarm),
		pushHistory:  make(map[string]*PushResult),
		level1State: &level1AlarmState{
			nh3CurrentlyExceed: false,
			tnCurrentlyExceed:  false,
		},
		level2State: &level2AlarmState{
			offlineSensors: make(map[string]time.Time),
			fanFailures:    make(map[string]time.Time),
		},
	}
}

func (ar *AlarmRouter) RouterLoop(stopCh <-chan struct{}) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	ar.logger.Info("Alarm router started",
		zap.Bool("sms_enabled", ar.cfg.SMS.Enabled),
		zap.Int("sms_recipients", len(ar.cfg.SMS.Recipients)),
		zap.Float64("nh3_threshold", ar.cfg.Level1.NH3Threshold),
		zap.Float64("tn_threshold", ar.cfg.Level1.TNThreshold))

	for {
		select {
		case <-stopCh:
			ar.logger.Info("Alarm router stopped")
			return

		case msg := <-ar.alarmInCh:
			ar.processAlarmMessage(msg)

		case now := <-ticker.C:
			ar.CheckLevel1Alarms(now)
			ar.CheckLevel2Alarms(now)
		}
	}
}

func (ar *AlarmRouter) processAlarmMessage(msg *messages.AlarmMessage) {
	alarm := &models.Alarm{
		ID:         uuid.New().String(),
		Level:      msg.Level,
		Type:       msg.Type,
		Message:    msg.Message,
		Value:      msg.Value,
		Threshold:  msg.Threshold,
		Timestamp:  msg.Timestamp,
		ACK:        false,
		Source:     msg.SourceModule,
	}

	ar.mu.Lock()
	ar.activeAlarms[alarm.ID] = alarm
	ar.mu.Unlock()

	ar.logger.Warn("Alarm received from module",
		zap.Int("level", msg.Level),
		zap.String("type", msg.Type),
		zap.String("source", msg.SourceModule),
		zap.String("message", msg.Message))

	ar.handleAlarm(alarm)
}

func (ar *AlarmRouter) handleAlarm(alarm *models.Alarm) {
	if err := ar.influxClient.WriteAlarm(alarm); err != nil {
		ar.logger.Error("Failed to write alarm to influxdb", zap.Error(err))
	}

	pushResult := ar.pushWithFailover(alarm)

	ar.mu.Lock()
	ar.pushHistory[alarm.ID] = pushResult
	ar.mu.Unlock()

	if !pushResult.WebSocketSuccess && !pushResult.SMSSent {
		ar.logger.Error("Alarm delivery failed - ALL CHANNELS DOWN!",
			zap.String("alarm_id", alarm.ID),
			zap.Int("level", alarm.Level),
			zap.Strings("errors", pushResult.Errors))
	}

	ar.logger.Warn("Alarm processed",
		zap.Int("level", alarm.Level),
		zap.String("type", alarm.Type),
		zap.String("message", alarm.Message),
		zap.Bool("ws_success", pushResult.WebSocketSuccess),
		zap.Bool("sms_sent", pushResult.SMSSent),
		zap.Strings("channels", pushResult.ChannelsUsed))
}

func (ar *AlarmRouter) pushWithFailover(alarm *models.Alarm) *PushResult {
	result := &PushResult{
		ChannelsUsed: make([]string, 0),
		Errors:       make([]string, 0),
	}

	wsClientCount := ar.wsServer.GetClientCount()
	wsAvailable := wsClientCount > 0

	forceDualChannel := (alarm.Level == 1)

	if wsAvailable {
		if err := ar.wsServer.BroadcastAlarm(alarm); err != nil {
			result.Errors = append(result.Errors, "websocket: "+err.Error())
			ar.logger.Error("WebSocket push failed",
				zap.String("alarm_id", alarm.ID),
				zap.Error(err))
		} else {
			result.WebSocketSuccess = true
			result.ChannelsUsed = append(result.ChannelsUsed, "websocket")
		}
	} else {
		result.Errors = append(result.Errors, "websocket: no clients connected")
		ar.logger.Warn("WebSocket no clients available",
			zap.String("alarm_id", alarm.ID))
	}

	shouldSendSMS := forceDualChannel || !result.WebSocketSuccess

	if shouldSendSMS && ar.cfg.SMS.Enabled {
		go ar.sendSMS(alarm)
		result.SMSSent = true
		result.ChannelsUsed = append(result.ChannelsUsed, "sms")
		ar.logger.Info("SMS sent for alarm",
			zap.String("alarm_id", alarm.ID),
			zap.Bool("forced", forceDualChannel))
	} else if shouldSendSMS && !ar.cfg.SMS.Enabled {
		result.Errors = append(result.Errors, "sms: not configured")
		ar.logger.Warn("SMS not configured, alarm may be lost",
			zap.String("alarm_id", alarm.ID),
			zap.Bool("ws_success", result.WebSocketSuccess))
	}

	return result
}

func (ar *AlarmRouter) sendSMS(alarm *models.Alarm) {
	levelName := map[int]string{1: "一级", 2: "二级"}[alarm.Level]
	content := fmt.Sprintf("[污水处理告警][%s] %s - %s (当前值: %.2f, 阈值: %.2f)",
		levelName, alarm.Type, alarm.Message, alarm.Value, alarm.Threshold)

	for _, recipient := range ar.cfg.SMS.Recipients {
		if err := ar.sendSMSRequest(recipient, content); err != nil {
			ar.logger.Error("Failed to send SMS",
				zap.String("recipient", recipient),
				zap.Error(err))
		}
	}
}

func (ar *AlarmRouter) sendSMSRequest(recipient, content string) error {
	if ar.cfg.SMS.APIURL == "" {
		return fmt.Errorf("SMS API URL not configured")
	}

	payload := fmt.Sprintf(`{"mobile":"%s","content":"%s","apikey":"%s"}`,
		recipient, content, ar.cfg.SMS.APIKey)

	resp, err := http.Post(ar.cfg.SMS.APIURL, "application/json",
		nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SMS API returned status %d", resp.StatusCode)
	}

	return nil
}

func (ar *AlarmRouter) CheckLevel1Alarms(now time.Time) {
	nh3Effluent, err := ar.influxClient.QueryLatestSensor("effluent_nh3_01")
	if err == nil && nh3Effluent != nil {
		if nh3Effluent.Value > ar.cfg.Level1.NH3Threshold {
			if !ar.level1State.nh3CurrentlyExceed {
				ar.level1State.nh3CurrentlyExceed = true
				ar.level1State.nh3ExceedStart = now
			} else if now.Sub(ar.level1State.nh3ExceedStart) >=
				time.Duration(ar.cfg.Level1.DurationMinutes)*time.Minute {
				ar.TriggerAlarm(1, "nh3_exceed",
					"出水氨氮超标",
					nh3Effluent.Value,
					ar.cfg.Level1.NH3Threshold)
				ar.level1State.nh3CurrentlyExceed = false
			}
		} else {
			ar.level1State.nh3CurrentlyExceed = false
		}
	}

	tnEffluent, err := ar.influxClient.QueryLatestSensor("effluent_tn_01")
	if err == nil && tnEffluent != nil {
		if tnEffluent.Value > ar.cfg.Level1.TNThreshold {
			if !ar.level1State.tnCurrentlyExceed {
				ar.level1State.tnCurrentlyExceed = true
				ar.level1State.tnExceedStart = now
			} else if now.Sub(ar.level1State.tnExceedStart) >=
				time.Duration(ar.cfg.Level1.DurationMinutes)*time.Minute {
				ar.TriggerAlarm(1, "tn_exceed",
					"出水总氮超标",
					tnEffluent.Value,
					ar.cfg.Level1.TNThreshold)
				ar.level1State.tnCurrentlyExceed = false
			}
		} else {
			ar.level1State.tnCurrentlyExceed = false
		}
	}
}

func (ar *AlarmRouter) CheckLevel2Alarms(now time.Time) {
	offlineSensors, err := ar.influxClient.CheckSensorOffline(
		time.Duration(ar.cfg.Level2.OfflineMinutes) * time.Minute)
	if err == nil {
		for _, sensor := range offlineSensors {
			if _, exists := ar.level2State.offlineSensors[sensor.ID]; !exists {
				ar.level2State.offlineSensors[sensor.ID] = now
				ar.TriggerAlarm(2, "sensor_offline",
					fmt.Sprintf("传感器离线: %s", sensor.ID),
					0, float64(ar.cfg.Level2.OfflineMinutes))
			}
		}
	}
}

func (ar *AlarmRouter) TriggerAlarm(level int, alarmType, message string, value, threshold float64) *models.Alarm {
	alarm := &models.Alarm{
		ID:        uuid.New().String(),
		Level:     level,
		Type:      alarmType,
		Message:   message,
		Value:     value,
		Threshold: threshold,
		Timestamp: time.Now(),
		ACK:       false,
		Source:    "alarm_router",
	}

	ar.mu.Lock()
	ar.activeAlarms[alarm.ID] = alarm
	ar.mu.Unlock()

	ar.handleAlarm(alarm)

	return alarm
}

func (ar *AlarmRouter) GetActiveAlarms() []*models.Alarm {
	ar.mu.RLock()
	defer ar.mu.RUnlock()

	result := make([]*models.Alarm, 0, len(ar.activeAlarms))
	for _, alarm := range ar.activeAlarms {
		result = append(result, alarm)
	}
	return result
}

func (ar *AlarmRouter) AcknowledgeAlarm(alarmID string) bool {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	if alarm, exists := ar.activeAlarms[alarmID]; exists {
		alarm.ACK = true
		alarm.ACKTime = time.Now()
		return true
	}
	return false
}

func (ar *AlarmRouter) ClearAlarm(alarmID string) bool {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	if _, exists := ar.activeAlarms[alarmID]; exists {
		delete(ar.activeAlarms, alarmID)
		return true
	}
	return false
}

func (ar *AlarmRouter) GetPushHistory(alarmID string) (*PushResult, bool) {
	ar.mu.RLock()
	defer ar.mu.RUnlock()

	result, exists := ar.pushHistory[alarmID]
	return result, exists
}
