package controller

import (
	"math"
	"sync"
	"time"

	"go.uber.org/zap"

	"sewage-treatment-system/internal/config"
	"sewage-treatment-system/internal/influxdb"
	"sewage-treatment-system/internal/models"
	"sewage-treatment-system/internal/mqtt"
	"sewage-treatment-system/internal/websocket"
)

const (
	MinComputeIntervalSeconds = 300
	CODChangeThresholdPercent = 20.0
)

type CarbonController struct {
	cfg            *config.CarbonConfig
	logger         *zap.Logger
	influxClient   *influxdb.Client
	mqttClient     *mqtt.Client
	wsServer       *websocket.Server
	mu             sync.RWMutex
	latestData     *carbonDosingData
	eventTriggerCh chan struct{}
}

type carbonDosingData struct {
	CODInfluent     float64
	NO3Anoxic       float64
	DosingRate      float64
	DosingActual    float64
	TNRemoval       float64
	LastUpdate      time.Time
	PrevCODInfluent float64
	LastComputeTime time.Time
}

func NewCarbonController(
	cfg *config.CarbonConfig,
	influxClient *influxdb.Client,
	mqttClient *mqtt.Client,
	wsServer *websocket.Server,
	logger *zap.Logger,
) *CarbonController {
	return &CarbonController{
		cfg:            cfg,
		logger:         logger,
		influxClient:   influxClient,
		mqttClient:     mqttClient,
		wsServer:       wsServer,
		latestData:     &carbonDosingData{},
		eventTriggerCh: make(chan struct{}, 1),
	}
}

func (cc *CarbonController) UpdateData(codInfluent, no3Anoxic float64) {
	cc.mu.Lock()

	prevCOD := cc.latestData.CODInfluent
	codChangePct := 0.0
	if prevCOD > 0 {
		codChangePct = math.Abs(codInfluent-prevCOD) / prevCOD * 100
	}
	timeSinceLastCompute := time.Since(cc.latestData.LastComputeTime).Seconds()

	shouldTriggerEvent := false
	eventBlocked := false
	blockReason := ""

	if codChangePct > CODChangeThresholdPercent {
		if timeSinceLastCompute >= MinComputeIntervalSeconds {
			shouldTriggerEvent = true
		} else {
			eventBlocked = true
			blockReason = "最小计算间隔保护"
		}
	}

	cc.latestData.PrevCODInfluent = prevCOD
	cc.latestData.CODInfluent = codInfluent
	cc.latestData.NO3Anoxic = no3Anoxic
	cc.latestData.LastUpdate = time.Now()
	cc.mu.Unlock()

	if shouldTriggerEvent {
		select {
		case cc.eventTriggerCh <- struct{}{}:
		default:
		}
	}
}

func (cc *CarbonController) Compute() (float64, float64) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.latestData.CODInfluent <= 0 || cc.latestData.NO3Anoxic <= 0 {
		cc.logger.Warn("Invalid carbon dosing input data")
		return 0, 0
	}

	no3ToRemove := cc.latestData.NO3Anoxic * 0.85

	requiredCOD := no3ToRemove * cc.cfg.CODNRatio

	availableCOD := cc.latestData.CODInfluent * 0.7

	carbonDeficit := requiredCOD - availableCOD

	if carbonDeficit < 0 {
		carbonDeficit = 0
	}

	sodiumAcetateEquivalent := carbonDeficit / 0.58

	dosingRate := sodiumAcetateEquivalent * cc.cfg.TNRemovalTarget

	optimizationFactor := 1.0
	if cc.latestData.TNRemoval > cc.cfg.TNRemovalTarget {
		optimizationFactor = 0.9
	} else if cc.latestData.TNRemoval < cc.cfg.TNRemovalTarget*0.9 {
		optimizationFactor = 1.1
	}

	dosingRate *= optimizationFactor

	if dosingRate > cc.cfg.DosingMax {
		dosingRate = cc.cfg.DosingMax
	}
	if dosingRate < 0 {
		dosingRate = 0
	}

	tnRemoval := cc.calculateTNRemoval(dosingRate)

	return math.Round(dosingRate*100) / 100, math.Round(tnRemoval*10000) / 10000
}

func (cc *CarbonController) calculateTNRemoval(dosingRate float64) float64 {
	if cc.latestData.NO3Anoxic <= 0 {
		return cc.cfg.TNRemovalTarget
	}

	providedCOD := dosingRate * 0.58
	totalAvailableCOD := cc.latestData.CODInfluent*0.7 + providedCOD

	removableNO3 := totalAvailableCOD / cc.cfg.CODNRatio

	maxRemoval := removableNO3 / cc.latestData.NO3Anoxic

	if maxRemoval > 0.95 {
		maxRemoval = 0.95
	}
	if maxRemoval < 0.5 {
		maxRemoval = 0.5
	}

	return maxRemoval
}

func (cc *CarbonController) ControlLoop(stopCh <-chan struct{}) {
	ticker := time.NewTicker(time.Duration(cc.cfg.ControlInterval) * time.Second)
	defer ticker.Stop()

	cc.logger.Info("Carbon dosing control loop started",
		zap.Int("interval_sec", cc.cfg.ControlInterval),
		zap.Float64("cod_change_threshold_pct", CODChangeThresholdPercent),
		zap.Int("min_compute_interval_sec", MinComputeIntervalSeconds))

	for {
		select {
		case <-stopCh:
			cc.logger.Info("Carbon dosing control loop stopped")
			return
		case now := <-ticker.C:
			cc.runControlCycle(now, "timer")
		case <-cc.eventTriggerCh:
			now := time.Now()
			cc.runControlCycle(now, "event")
		}
	}
}

func (cc *CarbonController) runControlCycle(now time.Time, triggerType string) {
	dosingRate, tnRemoval := cc.Compute()

	cc.mu.Lock()
	cc.latestData.LastComputeTime = now
	cc.mu.Unlock()

	cc.mu.RLock()
	codInfluent := cc.latestData.CODInfluent
	no3Anoxic := cc.latestData.NO3Anoxic
	cc.mu.RUnlock()

	dosingActual := dosingRate * (0.98 + math.Abs(math.Sin(now.Second()*0.05))*0.04)

	if err := cc.mqttClient.PublishCarbonCommand(dosingRate); err != nil {
		cc.logger.Error("Failed to publish carbon command", zap.Error(err))
	}

	dosing := &models.CarbonDosing{
		DosingRate:   dosingRate,
		DosingActual: math.Round(dosingActual*100) / 100,
		CODInfluent:  codInfluent,
		NO3Anoxic:    no3Anoxic,
		TNRemoval:    tnRemoval,
		Timestamp:    now,
	}

	if err := cc.influxClient.WriteCarbonDosing(dosing); err != nil {
		cc.logger.Error("Failed to write carbon dosing data", zap.Error(err))
	}

	if err := cc.wsServer.BroadcastCarbonDosing(dosing); err != nil {
		cc.logger.Error("Failed to broadcast carbon dosing", zap.Error(err))
	}

	cc.mu.Lock()
	cc.latestData.DosingRate = dosingRate
	cc.latestData.DosingActual = dosingActual
	cc.latestData.TNRemoval = tnRemoval
	cc.mu.Unlock()

	cc.logger.Debug("Carbon dosing executed",
		zap.String("trigger_type", triggerType),
		zap.Float64("dosing_rate", dosingRate),
		zap.Float64("dosing_actual", dosingActual),
		zap.Float64("cod_influent", codInfluent),
		zap.Float64("no3_anoxic", no3Anoxic),
		zap.Float64("tn_removal", tnRemoval))
}

func (cc *CarbonController) GetLatest() *models.CarbonDosing {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	return &models.CarbonDosing{
		DosingRate:   cc.latestData.DosingRate,
		DosingActual: cc.latestData.DosingActual,
		CODInfluent:  cc.latestData.CODInfluent,
		NO3Anoxic:    cc.latestData.NO3Anoxic,
		TNRemoval:    cc.latestData.TNRemoval,
		Timestamp:    cc.latestData.LastUpdate,
	}
}

func (cc *CarbonController) UpdateTarget(tnRemovalTarget float64) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if tnRemovalTarget > 0 && tnRemovalTarget <= 1.0 {
		cc.cfg.TNRemovalTarget = tnRemovalTarget
		cc.logger.Info("Carbon dosing target updated",
			zap.Float64("tn_removal_target", tnRemovalTarget))
	}
}

func (cc *CarbonController) CalculateOptimalDosing(codInfluent, no3Anoxic, targetTNRemoval float64) (float64, float64) {
	requiredCOD := no3Anoxic * targetTNRemoval * cc.cfg.CODNRatio
	availableCOD := codInfluent * 0.7
	carbonDeficit := math.Max(0, requiredCOD - availableCOD)
	dosingRate := (carbonDeficit / 0.58) * 1.05

	if dosingRate > cc.cfg.DosingMax {
		dosingRate = cc.cfg.DosingMax
	}

	achievableRemoval := math.Min(targetTNRemoval, (availableCOD+dosingRate*0.58)/(no3Anoxic*cc.cfg.CODNRatio))

	return math.Round(dosingRate*100) / 100, math.Round(achievableRemoval*10000) / 10000
}
