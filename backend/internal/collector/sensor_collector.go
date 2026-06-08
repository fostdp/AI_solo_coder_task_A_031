package collector

import (
	"fmt"
	"math"
	"sync"
	"time"

	"go.uber.org/zap"

	"sewage-treatment-system/internal/config"
	"sewage-treatment-system/internal/influxdb"
	"sewage-treatment-system/internal/messages"
	"sewage-treatment-system/internal/models"
	"sewage-treatment-system/internal/websocket"
)

type SensorCollector struct {
	cfg          *config.CollectorConfig
	logger       *zap.Logger
	influxClient *influxdb.Client
	wsServer     *websocket.Server
	sensorInfo   map[string]models.SensorInfo

	validatedDataCh chan<- *messages.SensorDataMessage
	alarmCh         chan<- *messages.AlarmMessage

	lastValues    map[string]float64
	lastTimestamps map[string]time.Time
	mu            sync.RWMutex

	channels *CollectorChannels
}

type CollectorChannels struct {
	ValidatedData chan *messages.SensorDataMessage
	AlarmOut      chan *messages.AlarmMessage
}

func NewSensorCollector(
	cfg *config.CollectorConfig,
	influxClient *influxdb.Client,
	wsServer *websocket.Server,
	logger *zap.Logger,
	sensorInfo map[string]models.SensorInfo,
	channels *CollectorChannels,
) *SensorCollector {
	return &SensorCollector{
		cfg:              cfg,
		logger:           logger,
		influxClient:     influxClient,
		wsServer:         wsServer,
		sensorInfo:       sensorInfo,
		validatedDataCh:  channels.ValidatedData,
		alarmCh:          channels.AlarmOut,
		lastValues:       make(map[string]float64),
		lastTimestamps:   make(map[string]time.Time),
		channels:         channels,
	}
}

func (sc *SensorCollector) ProcessSensorData(data *models.SensorData) error {
	now := time.Now()

	if info, ok := sc.sensorInfo[data.ID]; ok {
		data.Type = info.Type
		data.Stage = info.Stage
		data.Section = info.Section
		data.Setpoint = info.Setpoint
	}

	data.Timestamp = now
	data.Status = "online"

	valid, errMsg := sc.validateSensorData(data)

	msg := &messages.SensorDataMessage{
		Data:      data,
		Valid:     valid,
		Error:     errMsg,
		Timestamp: now,
	}

	if valid {
		sc.updateCache(data)

		if err := sc.influxClient.WriteSensorData(data); err != nil {
			sc.logger.Error("Failed to write sensor data to InfluxDB",
				zap.String("sensor_id", data.ID),
				zap.Error(err))
		}

		if err := sc.wsServer.BroadcastSensorData(data); err != nil {
			sc.logger.Warn("Failed to broadcast sensor data",
				zap.String("sensor_id", data.ID),
				zap.Error(err))
		}

		sc.checkOfflineSensors()

		select {
		case sc.validatedDataCh <- msg:
		default:
			sc.logger.Warn("Validated data channel full, dropping message",
				zap.String("sensor_id", data.ID))
		}
	} else {
		sc.logger.Warn("Sensor data validation failed",
			zap.String("sensor_id", data.ID),
			zap.String("error", errMsg),
			zap.Float64("value", data.Value))

		sc.sendAlarm(2, "data_validation",
			fmt.Sprintf("传感器数据校验失败: %s, %s", data.ID, errMsg),
			data.Value, 0)
	}

	return nil
}

func (sc *SensorCollector) validateSensorData(data *models.SensorData) (bool, string) {
	if data.ID == "" {
		return false, "传感器ID为空"
	}

	if _, ok := sc.sensorInfo[data.ID]; !ok {
		return false, fmt.Sprintf("未知传感器ID: %s", data.ID)
	}

	info := sc.sensorInfo[data.ID]

	if math.IsNaN(data.Value) || math.IsInf(data.Value, 0) {
		return false, "数值非法(NaN或Inf)"
	}

	validRanges := map[models.SensorType][2]float64{
		models.SensorDO:    {0, 20},
		models.SensorNH3:   {0, 100},
		models.SensorNO3:   {0, 100},
		models.SensorPO4:   {0, 50},
		models.SensorCOD:   {0, 2000},
		models.SensorTN:    {0, 200},
		models.SensorTP:    {0, 50},
		models.SensorFlow:  {0, 5000},
		models.SensorLevel: {0, 10},
	}

	if rng, ok := validRanges[data.Type]; ok {
		if data.Value < rng[0] || data.Value > rng[1] {
			return false, fmt.Sprintf("数值超出范围[%.2f, %.2f]", rng[0], rng[1])
		}
	}

	sc.mu.RLock()
	lastVal, hasLast := sc.lastValues[data.ID]
	lastTs, hasLastTs := sc.lastTimestamps[data.ID]
	sc.mu.RUnlock()

	if hasLast && hasLastTs && sc.cfg.JumpDetectionEnabled {
		changePct := math.Abs(data.Value-lastVal) / math.Max(math.Abs(lastVal), 1.0) * 100
		if changePct > sc.cfg.MaxJumpPercent {
			return false, fmt.Sprintf("数值突变超过阈值: %.2f%% > %.2f%%", changePct, sc.cfg.MaxJumpPercent)
		}
	}

	return true, ""
}

func (sc *SensorCollector) updateCache(data *models.SensorData) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.lastValues[data.ID] = data.Value
	sc.lastTimestamps[data.ID] = data.Timestamp
}

func (sc *SensorCollector) checkOfflineSensors() {
	if !sc.cfg.OfflineDetectionEnabled {
		return
	}

	sc.mu.RLock()
	defer sc.mu.RUnlock()

	offlineThreshold := time.Duration(sc.cfg.OfflineThresholdMinutes) * time.Minute
	now := time.Now()

	for id, lastTs := range sc.lastTimestamps {
		if now.Sub(lastTs) > offlineThreshold {
			info := sc.sensorInfo[id]
			sc.sendAlarm(2, "sensor_offline",
				fmt.Sprintf("传感器离线: %s (%s)", id, info.Type),
				now.Sub(lastTs).Minutes(),
				float64(sc.cfg.OfflineThresholdMinutes))
		}
	}
}

func (sc *SensorCollector) sendAlarm(level int, alarmType, message string, value, threshold float64) {
	alarmMsg := &messages.AlarmMessage{
		Level:        level,
		Type:         alarmType,
		Message:      message,
		Value:        value,
		Threshold:    threshold,
		SourceModule: "sensor_collector",
		Timestamp:    time.Now(),
	}

	select {
	case sc.alarmCh <- alarmMsg:
	default:
		sc.logger.Warn("Alarm channel full, dropping alarm",
			zap.String("type", alarmType))
	}
}

func (sc *SensorCollector) GetLastValue(sensorID string) (float64, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	val, ok := sc.lastValues[sensorID]
	return val, ok
}

func (sc *SensorCollector) GetAllLastValues() map[string]float64 {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	result := make(map[string]float64, len(sc.lastValues))
	for k, v := range sc.lastValues {
		result[k] = v
	}
	return result
}
