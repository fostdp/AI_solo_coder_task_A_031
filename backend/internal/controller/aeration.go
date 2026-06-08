package controller

import (
	"fmt"
	"math"
	"sync"
	"time"

	"go.uber.org/zap"

	"sewage-treatment-system/internal/config"
	"sewage-treatment-system/internal/influxdb"
	"sewage-treatment-system/internal/models"
	"sewage-treatment-system/internal/mqtt"
	"sewage-treatment-system/internal/websocket"
	"sewage-treatment-system/pkg/pid"
)

type AerationController struct {
	cfg          *config.AerationConfig
	logger       *zap.Logger
	influxClient *influxdb.Client
	mqttClient   *mqtt.Client
	wsServer     *websocket.Server
	pidControllers map[int]*pid.Controller
	sectionCount int
	mu           sync.RWMutex
	latestData   map[int]*aerationSectionData
}

type aerationSectionData struct {
	DO          float64
	NH3         float64
	AirFlowSet  float64
	AirFlowAct  float64
	ValveOpen   float64
	LastUpdate  time.Time
}

func NewAerationController(
	cfg *config.AerationConfig,
	influxClient *influxdb.Client,
	mqttClient *mqtt.Client,
	wsServer *websocket.Server,
	logger *zap.Logger,
	sectionCount int,
) *AerationController {
	ac := &AerationController{
		cfg:            cfg,
		logger:         logger,
		influxClient:   influxClient,
		mqttClient:     mqttClient,
		wsServer:       wsServer,
		sectionCount:   sectionCount,
		pidControllers: make(map[int]*pid.Controller),
		latestData:     make(map[int]*aerationSectionData),
	}

	for i := 1; i <= sectionCount; i++ {
		ac.pidControllers[i] = pid.NewController(
			cfg.Kp,
			cfg.Ki,
			cfg.Kd,
			cfg.DOSetpoint,
			0,
			100,
		)
		ac.latestData[i] = &aerationSectionData{}
	}

	return ac
}

func (ac *AerationController) UpdateSectionData(section int, do, nh3 float64) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if data, ok := ac.latestData[section]; ok {
		data.DO = do
		data.NH3 = nh3
		data.LastUpdate = time.Now()
	}
}

func (ac *AerationController) Compute(section int, now time.Time) (float64, float64) {
	ac.mu.RLock()
	data, ok := ac.latestData[section]
	ac.mu.RUnlock()

	if !ok {
		ac.logger.Warn("No data for aeration section", zap.Int("section", section))
		return 0, 0
	}

	pidCtl, ok := ac.pidControllers[section]
	if !ok {
		ac.logger.Warn("No PID controller for section", zap.Int("section", section))
		return 0, 0
	}

	nh3Deviation := data.NH3 - ac.cfg.NH3Setpoint
	feedforward := ac.cfg.FeedforwardGain * nh3Deviation * 10

	airFlow := pidCtl.ComputeWithFeedforward(data.DO, feedforward, now)

	minAir := 20.0
	maxAir := 100.0
	if airFlow < minAir {
		airFlow = minAir
	}
	if airFlow > maxAir {
		airFlow = maxAir
	}

	valveOpen := ac.computeValveOpen(airFlow, data.DO)

	return airFlow, valveOpen
}

func (ac *AerationController) computeValveOpen(airFlow, do float64) float64 {
	baseValve := (airFlow / 100.0) * 100

	if do > ac.cfg.DOMax {
		baseValve *= 0.8
	} else if do < ac.cfg.DOMin {
		baseValve *= 1.1
	}

	if baseValve < 0 {
		baseValve = 0
	}
	if baseValve > 100 {
		baseValve = 100
	}

	return baseValve
}

func (ac *AerationController) ControlLoop(stopCh <-chan struct{}) {
	ticker := time.NewTicker(time.Duration(ac.cfg.ControlInterval) * time.Second)
	defer ticker.Stop()

	ac.logger.Info("Aeration control loop started",
		zap.Int("sections", ac.sectionCount),
		zap.Int("interval_sec", ac.cfg.ControlInterval))

	for {
		select {
		case <-stopCh:
			ac.logger.Info("Aeration control loop stopped")
			return
		case now := <-ticker.C:
			ac.runControlCycle(now)
		}
	}
}

func (ac *AerationController) runControlCycle(now time.Time) {
	for section := 1; section <= ac.sectionCount; section++ {
		airFlowSet, valveOpen := ac.Compute(section, now)

		ac.mu.RLock()
		data := ac.latestData[section]
		ac.mu.RUnlock()

		airFlowActual := airFlowSet * (0.95 + math.Abs(math.Sin(float64(section)*0.5+now.Second()*0.01))*0.1)

		if err := ac.mqttClient.PublishAerationCommand(section, airFlowSet, valveOpen); err != nil {
			ac.logger.Error("Failed to publish aeration command",
				zap.Int("section", section),
				zap.Error(err))
		}

		ctrl := &models.AerationControl{
			Section:       section,
			AirFlowSet:    math.Round(airFlowSet*100) / 100,
			AirFlowActual: math.Round(airFlowActual*100) / 100,
			ValveOpen:     math.Round(valveOpen*100) / 100,
			DOActual:      data.DO,
			NH3Actual:     data.NH3,
			Timestamp:     now,
		}

		if err := ac.influxClient.WriteAerationControl(ctrl); err != nil {
			ac.logger.Error("Failed to write aeration control data", zap.Error(err))
		}

		if err := ac.wsServer.BroadcastAerationControl(ctrl); err != nil {
			ac.logger.Error("Failed to broadcast aeration control", zap.Error(err))
		}

		ac.mu.Lock()
		if d, ok := ac.latestData[section]; ok {
			d.AirFlowSet = airFlowSet
			d.AirFlowAct = airFlowActual
			d.ValveOpen = valveOpen
		}
		ac.mu.Unlock()

		ac.logger.Debug("Aeration control executed",
			zap.Int("section", section),
			zap.Float64("air_flow_set", airFlowSet),
			zap.Float64("valve_open", valveOpen),
			zap.Float64("do", data.DO),
			zap.Float64("nh3", data.NH3))
	}
}

func (ac *AerationController) GetLatestControl(section int) (*models.AerationControl, error) {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	data, ok := ac.latestData[section]
	if !ok {
		return nil, fmt.Errorf("section %d not found", section)
	}

	return &models.AerationControl{
		Section:       section,
		AirFlowSet:    data.AirFlowSet,
		AirFlowActual: data.AirFlowAct,
		ValveOpen:     data.ValveOpen,
		DOActual:      data.DO,
		NH3Actual:     data.NH3,
		Timestamp:     data.LastUpdate,
	}, nil
}

func (ac *AerationController) GetAllLatestControls() []*models.AerationControl {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	var results []*models.AerationControl
	for section := 1; section <= ac.sectionCount; section++ {
		if data, ok := ac.latestData[section]; ok {
			results = append(results, &models.AerationControl{
				Section:       section,
				AirFlowSet:    data.AirFlowSet,
				AirFlowActual: data.AirFlowAct,
				ValveOpen:     data.ValveOpen,
				DOActual:      data.DO,
				NH3Actual:     data.NH3,
				Timestamp:     data.LastUpdate,
			})
		}
	}
	return results
}

func (ac *AerationController) UpdateSetpoint(doSetpoint, nh3Setpoint float64) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.cfg.DOSetpoint = doSetpoint
	ac.cfg.NH3Setpoint = nh3Setpoint

	for _, ctl := range ac.pidControllers {
		ctl.SetSetpoint(doSetpoint)
	}

	ac.logger.Info("Aeration setpoints updated",
		zap.Float64("do", doSetpoint),
		zap.Float64("nh3", nh3Setpoint))
}

func (ac *AerationController) UpdateTunings(kp, ki, kd float64) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.cfg.Kp = kp
	ac.cfg.Ki = ki
	ac.cfg.Kd = kd

	for _, ctl := range ac.pidControllers {
		ctl.SetTunings(kp, ki, kd)
	}

	ac.logger.Info("Aeration PID tunings updated",
		zap.Float64("kp", kp),
		zap.Float64("ki", ki),
		zap.Float64("kd", kd))
}
