package controller

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
	"sewage-treatment-system/internal/mqtt"
)

var (
	MinComputeIntervalSeconds int
	CODChangeThresholdPercent float64
	CODNRatio                 float64
	BioavailableCODRatio      float64
	DenitrificationEff        float64
	NaAcCODEquivalent         float64
)

func initCarbonConstants(cfg *config.CarbonConfig) {
	MinComputeIntervalSeconds = cfg.MinComputeInterval
	CODChangeThresholdPercent = cfg.CODChangeThresholdPct
	CODNRatio = cfg.CODNRatio
	BioavailableCODRatio = cfg.BioavailableCODRatio
	DenitrificationEff = cfg.DenitrificationEff
	NaAcCODEquivalent = cfg.NaAcCODEquivalent
}

type CarbonOptimizer struct {
	cfg          *config.CarbonConfig
	logger       *zap.Logger
	influxClient *influxdb.Client
	mqttClient   *mqtt.Client

	sensorDataCh  <-chan *messages.SensorDataMessage
	carbonOutCh   chan<- *messages.CarbonDosingMessage
	alarmCh       chan<- *messages.AlarmMessage

	latestData       *LatestData
	eventTriggerCh   chan struct{}
	lastComputeTime  time.Time
	mu               sync.RWMutex
}

type LatestData struct {
	CODInfluent    float64
	NO3Anoxic      float64
	TNInfluent     float64
	TNEffluent     float64
	LastComputeTime time.Time
}

type CarbonChannels struct {
	SensorDataIn  chan *messages.SensorDataMessage
	CarbonOut     chan *messages.CarbonDosingMessage
	AlarmOut      chan *messages.AlarmMessage
}

func NewCarbonOptimizer(
	cfg *config.CarbonConfig,
	influxClient *influxdb.Client,
	mqttClient *mqtt.Client,
	logger *zap.Logger,
	channels *CarbonChannels,
) *CarbonOptimizer {
	initCarbonConstants(cfg)

	return &CarbonOptimizer{
		cfg:            cfg,
		logger:         logger,
		influxClient:   influxClient,
		mqttClient:     mqttClient,
		sensorDataCh:   channels.SensorDataIn,
		carbonOutCh:    channels.CarbonOut,
		alarmCh:        channels.AlarmOut,
		latestData: &LatestData{
			CODInfluent: 300.0,
			NO3Anoxic:   25.0,
			TNInfluent:  45.0,
			TNEffluent:  12.0,
		},
		eventTriggerCh: make(chan struct{}, 1),
	}
}

func (co *CarbonOptimizer) ControlLoop(stopCh <-chan struct{}) {
	ticker := time.NewTicker(time.Duration(co.cfg.ControlInterval) * time.Second)
	defer ticker.Stop()

	co.logger.Info("Carbon dosing control loop started",
		zap.Int("interval_sec", co.cfg.ControlInterval),
		zap.Float64("cod_change_threshold_pct", CODChangeThresholdPercent),
		zap.Int("min_compute_interval_sec", MinComputeIntervalSeconds),
		zap.Float64("dosing_max", co.cfg.DosingMax),
		zap.Float64("tn_removal_target", co.cfg.TNRemovalTarget))

	for {
		select {
		case <-stopCh:
			co.logger.Info("Carbon dosing control loop stopped")
			return

		case msg := <-co.sensorDataCh:
			if !msg.Valid {
				continue
			}
			co.processSensorData(msg.Data)

		case now := <-ticker.C:
			co.runControlCycle(now, "timer")

		case <-co.eventTriggerCh:
			now := time.Now()
			co.runControlCycle(now, "event")
		}
	}
}

func (co *CarbonOptimizer) processSensorData(data *models.SensorData) {
	co.mu.Lock()

	oldCOD := co.latestData.CODInfluent
	shouldTriggerEvent := false
	now := time.Now()

	switch data.Type {
	case models.SensorCOD:
		if data.Value > 0 {
			if co.latestData.CODInfluent > 0 {
				codChangePct := math.Abs(data.Value-co.latestData.CODInfluent) / co.latestData.CODInfluent * 100
				timeSinceLastCompute := now.Sub(co.latestData.LastComputeTime).Seconds()

				if codChangePct > CODChangeThresholdPercent &&
					timeSinceLastCompute >= MinComputeIntervalSeconds {
					shouldTriggerEvent = true
				}
			}
			co.latestData.CODInfluent = data.Value
		}

	case models.SensorNO3:
		if data.Value > 0 && data.Stage == models.StageAnoxic {
			co.latestData.NO3Anoxic = data.Value
		}

	case models.SensorTN:
		if data.Value > 0 {
			if data.Stage == models.StageInfluent {
				co.latestData.TNInfluent = data.Value
			} else if data.Stage == models.StageEffluent {
				co.latestData.TNEffluent = data.Value
			}
		}
	}

	co.mu.Unlock()

	if shouldTriggerEvent {
		co.logger.Info("COD change detected, triggering carbon optimization",
			zap.Float64("old_cod", oldCOD),
			zap.Float64("new_cod", data.Value))

		select {
		case co.eventTriggerCh <- struct{}{}:
		default:
		}
	}
}

func (co *CarbonOptimizer) runControlCycle(now time.Time, triggerType string) {
	co.mu.Lock()
	defer co.mu.Unlock()

	if triggerType != "timer" {
		elapsed := now.Sub(co.lastComputeTime).Seconds()
		if elapsed < MinComputeIntervalSeconds {
			return
		}
	}
	co.lastComputeTime = now
	co.latestData.LastComputeTime = now

	dosingRate, tnRemoval := co.Compute()

	dosingActual := math.Min(dosingRate, co.cfg.DosingMax)
	if dosingActual < 0 {
		dosingActual = 0
	}

	co.sendDosingCommand(dosingActual)

	carbonMsg := &messages.CarbonDosingMessage{
		DosingRate:   dosingRate,
		CODInfluent:  co.latestData.CODInfluent,
		NO3Anoxic:    co.latestData.NO3Anoxic,
		TNRemoval:    tnRemoval,
		Timestamp:    now,
		TriggerType:  triggerType,
	}

	select {
	case co.carbonOutCh <- carbonMsg:
	default:
	}

	carbonDosing := &models.CarbonDosing{
		DosingRate:   dosingRate,
		DosingActual: dosingActual,
		CODInfluent:  co.latestData.CODInfluent,
		NO3Anoxic:    co.latestData.NO3Anoxic,
		TNRemoval:    tnRemoval,
		Timestamp:    now,
	}

	if err := co.influxClient.WriteCarbonDosing(carbonDosing); err != nil {
		co.logger.Error("Failed to write carbon dosing data", zap.Error(err))
	}

	co.logger.Debug("Carbon dosing executed",
		zap.String("trigger_type", triggerType),
		zap.Float64("dosing_rate", dosingRate),
		zap.Float64("dosing_actual", dosingActual),
		zap.Float64("cod_influent", co.latestData.CODInfluent),
		zap.Float64("no3_anoxic", co.latestData.NO3Anoxic),
		zap.Float64("tn_removal", tnRemoval))
}

func (co *CarbonOptimizer) Compute() (float64, float64) {
	no3ToRemove := co.latestData.NO3Anoxic * DenitrificationEff
	requiredCOD := no3ToRemove * CODNRatio
	availableCOD := co.latestData.CODInfluent * BioavailableCODRatio
	carbonDeficit := requiredCOD - availableCOD

	if carbonDeficit < 0 {
		carbonDeficit = 0
	}

	naAcEquivalent := carbonDeficit / NaAcCODEquivalent
	dosingRate := naAcEquivalent * co.cfg.TNRemovalTarget

	currentTNRemoval := (co.latestData.TNInfluent - co.latestData.TNEffluent) / co.latestData.TNInfluent * 100

	if currentTNRemoval > co.cfg.TNRemovalTarget {
		dosingRate *= 0.9
	} else if currentTNRemoval < co.cfg.TNRemovalTarget*0.9 {
		dosingRate *= 1.1
	}

	if dosingRate < 0 {
		dosingRate = 0
	}

	return dosingRate, currentTNRemoval
}

func (co *CarbonOptimizer) sendDosingCommand(dosingRate float64) {
	payload := fmt.Sprintf(`{"dosing_rate":%.2f,"dosing_pump_speed":%.1f}`,
		dosingRate, dosingRate/co.cfg.DosingMax*100)

	topic := "sewage/control/carbon"
	if err := co.mqttClient.Publish(topic, 1, false, payload); err != nil {
		co.logger.Error("Failed to publish carbon dosing command", zap.Error(err))

		co.sendAlarm(2, "mqtt_publish_failed",
			"碳源投加指令发布失败",
			dosingRate, 0)
	}
}

func (co *CarbonOptimizer) CalculateOptimalDosing(codInfluent, no3Anoxic, tnInfluent, tnEffluent float64) float64 {
	no3ToRemove := no3Anoxic * DenitrificationEff
	requiredCOD := no3ToRemove * CODNRatio
	availableCOD := codInfluent * BioavailableCODRatio
	carbonDeficit := requiredCOD - availableCOD

	if carbonDeficit < 0 {
		carbonDeficit = 0
	}

	naAcEquivalent := carbonDeficit / NaAcCODEquivalent
	dosingRate := naAcEquivalent * co.cfg.TNRemovalTarget

	currentTNRemoval := (tnInfluent - tnEffluent) / tnInfluent * 100

	if currentTNRemoval > co.cfg.TNRemovalTarget {
		dosingRate *= 0.9
	} else if currentTNRemoval < co.cfg.TNRemovalTarget*0.9 {
		dosingRate *= 1.1
	}

	return math.Min(math.Max(dosingRate, 0), co.cfg.DosingMax)
}

func (co *CarbonOptimizer) GetLatestData() *LatestData {
	co.mu.RLock()
	defer co.mu.RUnlock()

	data := *co.latestData
	return &data
}

func (co *CarbonOptimizer) UpdateConfig(dosingMax, tnRemovalTarget float64) {
	co.mu.Lock()
	defer co.mu.Unlock()

	co.cfg.DosingMax = dosingMax
	co.cfg.TNRemovalTarget = tnRemovalTarget

	co.logger.Info("Carbon optimizer config updated",
		zap.Float64("dosing_max", dosingMax),
		zap.Float64("tn_removal_target", tnRemovalTarget))
}

func (co *CarbonOptimizer) sendAlarm(level int, alarmType, message string, value, threshold float64) {
	alarmMsg := &messages.AlarmMessage{
		Level:        level,
		Type:         alarmType,
		Message:      message,
		Value:        value,
		Threshold:    threshold,
		SourceModule: "carbon_optimizer",
		Timestamp:    time.Now(),
	}

	select {
	case co.alarmCh <- alarmMsg:
	default:
		co.logger.Warn("Alarm channel full, dropping alarm",
			zap.String("type", alarmType))
	}
}
