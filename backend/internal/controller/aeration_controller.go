package controller

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"sewage-treatment-system/internal/config"
	"sewage-treatment-system/internal/influxdb"
	"sewage-treatment-system/internal/messages"
	"sewage-treatment-system/internal/models"
	"sewage-treatment-system/internal/mqtt"
	"sewage-treatment-system/pkg/pid"
)

var (
	DOSetpoint        float64
	NH3Setpoint       float64
	FeedforwardGain   float64
	MinAirFlow        float64
	MaxAirFlow        float64
	MinValveOpen      float64
	MaxValveOpen      float64
)

func initAerationConstants(cfg *config.AerationConfig) {
	DOSetpoint = cfg.DOSetpoint
	NH3Setpoint = cfg.NH3Setpoint
	FeedforwardGain = cfg.FeedforwardGain
	MinAirFlow = cfg.MinAirFlow
	MaxAirFlow = cfg.MaxAirFlow
	MinValveOpen = cfg.MinValveOpen
	MaxValveOpen = cfg.MaxValveOpen
}

type AerationController struct {
	cfg          *config.AerationConfig
	logger       *zap.Logger
	influxClient *influxdb.Client
	mqttClient   *mqtt.Client
	numSections  int

	sensorDataCh <-chan *messages.SensorDataMessage
	controlOutCh chan<- *messages.AerationControlMessage
	alarmCh      chan<- *messages.AlarmMessage

	pidControllers []*pid.Controller
	sectionData    []*SectionData
	mu             sync.RWMutex

	lastComputeTime time.Time
}

type SectionData struct {
	DOValue      float64
	NH3Value     float64
	AirFlowSet   float64
	ValveOpen    float64
	LastUpdate   time.Time
	DOUpdated    bool
	NH3Updated   bool
}

type AerationChannels struct {
	SensorDataIn  chan *messages.SensorDataMessage
	ControlOut    chan *messages.AerationControlMessage
	AlarmOut      chan *messages.AlarmMessage
}

func NewAerationController(
	cfg *config.AerationConfig,
	influxClient *influxdb.Client,
	mqttClient *mqtt.Client,
	logger *zap.Logger,
	channels *AerationChannels,
) *AerationController {
	initAerationConstants(cfg)

	numSections := cfg.NumSections
	pids := make([]*pid.Controller, numSections)
	sectionData := make([]*SectionData, numSections)

	for i := 0; i < numSections; i++ {
		pids[i] = pid.NewController(DOSetpoint, cfg.PID.Kp, cfg.PID.Ki, cfg.PID.Kd)
		pids[i].SetOutputLimits(MinAirFlow, MaxAirFlow)
		pids[i].SetIntegralSeparationThreshold(cfg.PID.IntegralSeparationPercent)

		sectionData[i] = &SectionData{
			DOValue:    DOSetpoint,
			NH3Value:   NH3Setpoint,
			AirFlowSet: 250.0,
			ValveOpen:  50.0,
		}
	}

	return &AerationController{
		cfg:            cfg,
		logger:         logger,
		influxClient:   influxClient,
		mqttClient:     mqttClient,
		numSections:    numSections,
		sensorDataCh:   channels.SensorDataIn,
		controlOutCh:   channels.ControlOut,
		alarmCh:        channels.AlarmOut,
		pidControllers: pids,
		sectionData:    sectionData,
	}
}

func (ac *AerationController) ControlLoop(stopCh <-chan struct{}) {
	ticker := time.NewTicker(time.Duration(ac.cfg.ControlInterval) * time.Second)
	defer ticker.Stop()

	ac.logger.Info("Aeration control loop started",
		zap.Int("sections", ac.numSections),
		zap.Int("interval_sec", ac.cfg.ControlInterval),
		zap.Float64("kp", ac.cfg.PID.Kp),
		zap.Float64("ki", ac.cfg.PID.Ki),
		zap.Float64("kd", ac.cfg.PID.Kd),
		zap.Float64("integral_separation_pct", ac.cfg.PID.IntegralSeparationPercent))

	for {
		select {
		case <-stopCh:
			ac.logger.Info("Aeration control loop stopped")
			return

		case msg := <-ac.sensorDataCh:
			if !msg.Valid {
				continue
			}
			ac.processSensorData(msg.Data)

		case now := <-ticker.C:
			ac.runControlCycle(now, "timer")
		}
	}
}

func (ac *AerationController) processSensorData(data *models.SensorData) {
	section := data.Section - 1
	if section < 0 || section >= ac.numSections {
		return
	}

	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.sectionData[section].LastUpdate = data.Timestamp

	switch data.Type {
	case models.SensorDO:
		ac.sectionData[section].DOValue = data.Value
		ac.sectionData[section].DOUpdated = true

	case models.SensorNH3:
		ac.sectionData[section].NH3Value = data.Value
		ac.sectionData[section].NH3Updated = true

		if ac.cfg.EventDriven && ac.sectionData[section].DOUpdated {
			go ac.runControlCycle(time.Now(), "sensor_update")
		}
	}
}

func (ac *AerationController) runControlCycle(now time.Time, triggerSource string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if triggerSource != "timer" {
		elapsed := now.Sub(ac.lastComputeTime).Seconds()
		if elapsed < float64(ac.cfg.MinComputeInterval) {
			return
		}
	}
	ac.lastComputeTime = now

	for section := 0; section < ac.numSections; section++ {
		data := ac.sectionData[section]

		if !data.DOUpdated && !data.NH3Updated {
			continue
		}

		nh3Deviation := data.NH3Value - NH3Setpoint
		feedforward := FeedforwardGain * nh3Deviation * 10

		airFlowSetpoint := ac.pidControllers[section].ComputeWithFeedforward(
			data.DOValue,
			feedforward,
			now,
		)

		if airFlowSetpoint < MinAirFlow {
			airFlowSetpoint = MinAirFlow
		}
		if airFlowSetpoint > MaxAirFlow {
			airFlowSetpoint = MaxAirFlow
		}

		valveOpen := airFlowSetpoint / MaxAirFlow * 100
		if data.DOValue > 2.5 {
			valveOpen *= 0.8
		} else if data.DOValue < 1.5 {
			valveOpen *= 1.1
		}

		if valveOpen < MinValveOpen {
			valveOpen = MinValveOpen
		}
		if valveOpen > MaxValveOpen {
			valveOpen = MaxValveOpen
		}

		data.AirFlowSet = airFlowSetpoint
		data.ValveOpen = valveOpen

		ac.sendControlCommand(section+1, airFlowSetpoint, valveOpen)

		controlMsg := &messages.AerationControlMessage{
			Section:       section + 1,
			AirFlowSet:    airFlowSetpoint,
			ValveOpen:     valveOpen,
			DOActual:      data.DOValue,
			NH3Actual:     data.NH3Value,
			Timestamp:     now,
			TriggerSource: triggerSource,
		}

		select {
		case ac.controlOutCh <- controlMsg:
		default:
		}

		aerationCtrl := &models.AerationControl{
			Section:       section + 1,
			AirFlowSet:    airFlowSetpoint,
			AirFlowActual: airFlowSetpoint * (0.95 + 0.1*0),
			ValveOpen:     valveOpen,
			DOActual:      data.DOValue,
			NH3Actual:     data.NH3Value,
			Timestamp:     now,
		}

		if err := ac.influxClient.WriteAerationControl(aerationCtrl); err != nil {
			ac.logger.Error("Failed to write aeration control data",
				zap.Int("section", section+1),
				zap.Error(err))
		}

		data.DOUpdated = false
		data.NH3Updated = false
	}

	ac.logger.Debug("Aeration control cycle completed",
		zap.String("trigger", triggerSource),
		zap.Int("sections", ac.numSections))
}

func (ac *AerationController) sendControlCommand(section int, airFlow, valveOpen float64) {
	payload := fmt.Sprintf(`{"section":%d,"air_flow":%.2f,"valve_open":%.2f}`,
		section, airFlow, valveOpen)

	topic := fmt.Sprintf("sewage/control/aeration/%d", section)
	if err := ac.mqttClient.Publish(topic, 1, false, payload); err != nil {
		ac.logger.Error("Failed to publish aeration control command",
			zap.Int("section", section),
			zap.Error(err))

		ac.sendAlarm(2, "mqtt_publish_failed",
			fmt.Sprintf("曝气控制指令发布失败: 段%d", section),
			float64(section), 0)
	}
}

func (ac *AerationController) UpdateTuning(kp, ki, kd float64) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.cfg.PID.Kp = kp
	ac.cfg.PID.Ki = ki
	ac.cfg.PID.Kd = kd

	for i := 0; i < ac.numSections; i++ {
		ac.pidControllers[i].SetTunings(kp, ki, kd)
	}

	ac.logger.Info("PID parameters updated",
		zap.Float64("kp", kp),
		zap.Float64("ki", ki),
		zap.Float64("kd", kd))
}

func (ac *AerationController) ResetSection(section int) {
	s := section - 1
	if s < 0 || s >= ac.numSections {
		return
	}

	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.pidControllers[s].ResetIntegral()
	ac.sectionData[s].AirFlowSet = 250.0
	ac.sectionData[s].ValveOpen = 50.0

	ac.logger.Info("Aeration section reset",
		zap.Int("section", section))
}

func (ac *AerationController) GetSectionStatus(section int) *SectionData {
	s := section - 1
	if s < 0 || s >= ac.numSections {
		return nil
	}

	ac.mu.RLock()
	defer ac.mu.RUnlock()

	data := *ac.sectionData[s]
	return &data
}

func (ac *AerationController) GetAllStatus() []*SectionData {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	result := make([]*SectionData, ac.numSections)
	for i := 0; i < ac.numSections; i++ {
		data := *ac.sectionData[i]
		result[i] = &data
	}
	return result
}

func (ac *AerationController) sendAlarm(level int, alarmType, message string, value, threshold float64) {
	alarmMsg := &messages.AlarmMessage{
		Level:        level,
		Type:         alarmType,
		Message:      message,
		Value:        value,
		Threshold:    threshold,
		SourceModule: "aeration_controller",
		Timestamp:    time.Now(),
	}

	select {
	case ac.alarmCh <- alarmMsg:
	default:
		ac.logger.Warn("Alarm channel full, dropping alarm",
			zap.String("type", alarmType))
	}
}
