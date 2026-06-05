package carbon_optimizer

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"sewage-plant-system/pkg/models"
	sc "sewage-plant-system/pkg/sensor_collector"
)

type CarbonConfig struct {
	NO3Setpoint              float64       `mapstructure:"no3_setpoint"`
	TNSetpoint               float64       `mapstructure:"tn_setpoint"`
	MaxDosage                 float64       `mapstructure:"max_dosage"`
	MinDosage                 float64       `mapstructure:"min_dosage"`
	CODToNRatio               float64       `mapstructure:"cod_n_ratio"`
	CarbonSourceType          string        `mapstructure:"carbon_source_type"`
	CarbonSourceConcentration float64       `mapstructure:"carbon_source_concentration"`
	CODChangeThreshold        float64       `mapstructure:"cod_change_threshold"`
	EventDrivenEnabled         bool          `mapstructure:"event_driven_enabled"`
	TargetTNRemovalRate        float64       `mapstructure:"target_tn_removal_rate"`
	MaxCarbonPerTon          float64       `mapstructure:"max_carbon_per_ton"`
	EfficiencyWeight         float64       `mapstructure:"efficiency_weight"`
	CostWeight             float64       `mapstructure:"cost_weight"`
	ControlInterval          time.Duration `mapstructure:"control_interval"`
}

type ControlOutput struct {
	Command      *models.ControlCommand
	IsEventDriven bool
}

type CarbonHistoryRecord struct {
	Timestamp      time.Time
	NO3Actual      float64
	CODInfluent    float64
	DosageSetpoint float64
	TNRemovalRate  float64
}

type Status struct {
	NO3Actual              float64
	NO3Setpoint            float64
	CODInfluent            float64
	LastCODInfluent         float64
	CODRequired             float64
	CODAvailable           float64
	TNInfluent             float64
	TNEffluent             float64
	TNRemovalRate           float64
	DosageSetpoint         float64
	CarbonSourceType        string
	CarbonSourceConcentration float64
	LastUpdate             time.Time
	LastCalculationTime     time.Time
	LastCODSignificantChange time.Time
}

type CarbonOptimizer struct {
	cfg                CarbonConfig
	controlSystem      *CarbonControlSystem
	optimizer          *CarbonOptimizerCore
	validDataChan      <-chan *sc.ValidatedSensorData
	statusEventChan    <-chan *sc.SensorStatusEvent
	controlOutputChan  chan<- *ControlOutput
	statusChan         chan<- *Status
	sensorValues       map[string]map[string]float64
	flowRate           float64
	mu                 sync.RWMutex
}

type CarbonControlSystem struct {
	NO3Actual              float64
	NO3Setpoint            float64
	CODInfluent            float64
	LastCODInfluent         float64
	CODRequired             float64
	CODAvailable           float64
	TNInfluent             float64
	TNEffluent             float64
	TNRemovalRate           float64
	DosageSetpoint         float64
	CarbonSourceType        string
	CarbonSourceConcentration float64
	MaxDosage             float64
	MinDosage             float64
	CODToNRatio           float64
	LastUpdate             time.Time
	LastCalculationTime     time.Time
	CODChangeThreshold        float64
	EventDrivenEnabled         bool
	LastCODSignificantChange time.Time
	History                 []CarbonHistoryRecord
	mu                     sync.RWMutex
}

type CarbonOptimizerCore struct {
	TargetTNRemovalRate float64
	MaxCarbonPerTon     float64
	EfficiencyWeight    float64
	CostWeight        float64
}

func New(cfg CarbonConfig, validDataChan <-chan *sc.ValidatedSensorData, statusEventChan <-chan *sc.SensorStatusEvent, controlOutputChan chan<- *ControlOutput, statusChan chan<- *Status) *CarbonOptimizer {
	co := &CarbonOptimizer{
		cfg:                cfg,
		validDataChan:      validDataChan,
		statusEventChan:    statusEventChan,
		controlOutputChan: controlOutputChan,
		statusChan:        statusChan,
		sensorValues:     make(map[string]map[string]float64),
		flowRate:         300000,
	}

	co.controlSystem = NewCarbonControlSystem(cfg)
	co.optimizer = NewCarbonOptimizerCore(cfg)

	return co
}

func NewCarbonControlSystem(cfg CarbonConfig) *CarbonControlSystem {
	if cfg.NO3Setpoint == 0 {
		cfg.NO3Setpoint = 10.0
	}
	if cfg.TNSetpoint == 0 {
		cfg.TNSetpoint = 15.0
	}
	if cfg.MaxDosage == 0 {
		cfg.MaxDosage = 50.0
	}
	if cfg.CODToNRatio == 0 {
		cfg.CODToNRatio = 5.0
	}
	if cfg.CarbonSourceConcentration == 0 {
		cfg.CarbonSourceConcentration = 500000.0
	}
	if cfg.CODChangeThreshold == 0 {
		cfg.CODChangeThreshold = 0.2
	}
	if cfg.CarbonSourceType == "" {
		cfg.CarbonSourceType = "acetate"
	}

	return &CarbonControlSystem{
		NO3Setpoint:              cfg.NO3Setpoint,
		MaxDosage:                 cfg.MaxDosage,
		MinDosage:                 cfg.MinDosage,
		CODToNRatio:               cfg.CODToNRatio,
		CarbonSourceType:          cfg.CarbonSourceType,
		CarbonSourceConcentration: cfg.CarbonSourceConcentration,
		CODChangeThreshold:        cfg.CODChangeThreshold,
		EventDrivenEnabled:         cfg.EventDrivenEnabled,
		History:                   make([]CarbonHistoryRecord, 0, 100),
	}
}

func NewCarbonOptimizerCore(cfg CarbonConfig) *CarbonOptimizerCore {
	if cfg.TargetTNRemovalRate == 0 {
		cfg.TargetTNRemovalRate = 80.0
	}
	if cfg.MaxCarbonPerTon == 0 {
		cfg.MaxCarbonPerTon = 0.5
	}
	if cfg.EfficiencyWeight == 0 {
		cfg.EfficiencyWeight = 0.7
	}
	if cfg.CostWeight == 0 {
		cfg.CostWeight = 0.3
	}

	return &CarbonOptimizerCore{
		TargetTNRemovalRate: cfg.TargetTNRemovalRate,
		MaxCarbonPerTon:     cfg.MaxCarbonPerTon,
		EfficiencyWeight:    cfg.EfficiencyWeight,
		CostWeight:        cfg.CostWeight,
	}
}

func (co *CarbonOptimizer) Start() {
	go co.processSensorData()
	go co.processStatusEvents()
	go co.controlLoop()
}

func (co *CarbonOptimizer) processSensorData() {
	for data := range co.validDataChan {
		if !data.IsValid {
			continue
		}

		co.mu.Lock()
		if co.sensorValues[data.Location] == nil {
			co.sensorValues[data.Location] = make(map[string]float64)
		}
		co.sensorValues[data.Location][string(data.Type)] = data.Value
		co.mu.Unlock()

		if data.Type == models.SensorTypeCOD && data.Location == "influent" {
			go co.checkAndTriggerEventDriven(data.Value)
		}
	}
}

func (co *CarbonOptimizer) processStatusEvents() {
	for event := range co.statusEventChan {
		log.Printf("[CARBON] Received status event: %s for sensor %s", event.EventType, event.SensorID)
	}
}

func (co *CarbonOptimizer) controlLoop() {
	interval := co.cfg.ControlInterval
	if interval == 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		co.RunControlCycle(false)
	}
}

func (co *CarbonOptimizer) RunControlCycle(isEventDriven bool) {
	co.mu.Lock()
	no3Value := co.getSensorValue(models.SensorTypeNO3, "anoxic")
	codValue := co.getSensorValue(models.SensorTypeCOD, "influent")
	flowRate := co.flowRate
	co.mu.Unlock()

	if no3Value <= 0 {
		no3Value = 8.0
	}
	if codValue <= 0 {
		codValue = 300.0
	}

	tnInfluent := 40.0
	tnEffluent := no3Value + 2.0

	co.controlSystem.mu.Lock()
	if isEventDriven {
		co.controlSystem.updateInternal(no3Value, codValue, tnInfluent, tnEffluent, flowRate, true)
	} else {
		co.controlSystem.Update(no3Value, codValue, tnInfluent, tnEffluent, flowRate)
	}

	optimizedDosage := co.optimizer.Optimize(co.controlSystem, flowRate)
	if optimizedDosage != co.controlSystem.DosageSetpoint {
		co.controlSystem.DosageSetpoint = optimizedDosage
	}

	co.controlSystem.mu.Unlock()

	co.generateControlCommand(isEventDriven)
	co.broadcastStatus()
}

func (co *CarbonOptimizer) checkAndTriggerEventDriven(codValue float64) {
	co.controlSystem.mu.RLock()
	eventDrivenEnabled := co.controlSystem.EventDrivenEnabled
	lastCOD := co.controlSystem.LastCODInfluent
	threshold := co.controlSystem.CODChangeThreshold
	co.controlSystem.mu.RUnlock()

	if !eventDrivenEnabled || lastCOD <= 0 {
		return
	}

	codChange := math.Abs(codValue - lastCOD) / lastCOD
	if codChange >= threshold {
		log.Printf("[CARBON] COD significant change detected: old=%.2f, new=%.2f, change=%.2f%%, triggering immediate recalculation",
			lastCOD, codValue, codChange*100)
		co.RunControlCycle(true)
	}
}

func (co *CarbonOptimizer) getSensorValue(sensorType models.SensorType, location string) float64 {
	values, ok := co.sensorValues[location]
	if !ok {
		return 0
	}
	value, ok := values[string(sensorType)]
	if !ok {
		return 0
	}
	return value
}

func (co *CarbonOptimizer) generateControlCommand(isEventDriven bool) {
	co.controlSystem.mu.RLock()
	cmdData := co.controlSystem.GetControlCommand()
	co.controlSystem.mu.RUnlock()

	source := "carbon_control"
	if isEventDriven {
		source = "carbon_control_event_driven"
	}

	controlCmd := &models.ControlCommand{
		CommandID:  fmt.Sprintf("cmd_%d", time.Now().UnixNano()),
		TargetType: cmdData["target_type"].(string),
		TargetID:   cmdData["target_id"].(string),
		Action:     cmdData["action"].(string),
		Value:      cmdData["value"].(float64),
		Unit:       cmdData["unit"].(string),
		Timestamp:  time.Now(),
		Source:     source,
	}

	select {
	case co.controlOutputChan <- &ControlOutput{Command: controlCmd, IsEventDriven: isEventDriven}:
	default:
		log.Printf("[CARBON] ControlOutputChan full, dropping command")
	}
}

func (co *CarbonOptimizer) broadcastStatus() {
	status := co.GetStatus()
	select {
	case co.statusChan <- status:
	default:
	}
}

func (ccs *CarbonControlSystem) Update(no3Actual, codInfluent, tnInfluent, tnEffluent, flowRate float64) {
	ccs.mu.Lock()
	defer ccs.mu.Unlock()
	ccs.updateInternal(no3Actual, codInfluent, tnInfluent, tnEffluent, flowRate, false)
}

func (ccs *CarbonControlSystem) updateInternal(no3Actual, codInfluent, tnInfluent, tnEffluent, flowRate float64, isEventDriven bool) {
	ccs.LastCODInfluent = ccs.CODInfluent
	ccs.NO3Actual = no3Actual
	ccs.CODInfluent = codInfluent
	ccs.TNInfluent = tnInfluent
	ccs.TNEffluent = tnEffluent
	ccs.LastUpdate = time.Now()
	ccs.LastCalculationTime = time.Now()

	if tnInfluent > 0 {
		ccs.TNRemovalRate = (tnInfluent - tnEffluent) / tnInfluent * 100
	}

	ccs.calculateOptimalDosage(flowRate)

	record := CarbonHistoryRecord{
		Timestamp:      time.Now(),
		NO3Actual:      no3Actual,
		CODInfluent:    codInfluent,
		DosageSetpoint: ccs.DosageSetpoint,
		TNRemovalRate:  ccs.TNRemovalRate,
	}
	ccs.History = append(ccs.History, record)
	if len(ccs.History) > 100 {
		ccs.History = ccs.History[1:]
	}

	if isEventDriven {
		ccs.LastCODSignificantChange = time.Now()
		log.Printf("[CARBON] Event-driven recalculation complete: dosage=%.3f kg/h, removal_rate=%.2f%%",
			ccs.DosageSetpoint, ccs.TNRemovalRate)
	}
}

func (ccs *CarbonControlSystem) calculateOptimalDosage(flowRate float64) {
	no3N := ccs.NO3Actual
	tnEstimate := ccs.TNEffluent

	requiredNO3Removal := math.Max(0, no3N - ccs.NO3Setpoint)
	requiredTNRemoval := math.Max(0, tnEstimate - ccs.TNSetpoint)

	totalNRemoval := math.Max(requiredNO3Removal, requiredTNRemoval)

	ccs.CODRequired = totalNRemoval * ccs.CODToNRatio * 1.2

	ccs.CODAvailable = math.Max(0, ccs.CODInfluent-50) * 0.7

	additionalCODNeeded := math.Max(0, ccs.CODRequired - ccs.CODAvailable)

	flowRateM3h := flowRate / 24.0

	if additionalCODNeeded > 0 && ccs.CarbonSourceConcentration > 0 {
		dosageMgL := (additionalCODNeeded * 1000) / ccs.CarbonSourceConcentration
		dosageKgh := dosageMgL * flowRateM3h
		ccs.DosageSetpoint = clamp(dosageKgh, ccs.MinDosage, ccs.MaxDosage)
	} else {
		ccs.DosageSetpoint = ccs.MinDosage
	}

	ccs.DosageSetpoint = ccs.optimizeDosageWithHistory(ccs.DosageSetpoint)
}

func (ccs *CarbonControlSystem) optimizeDosageWithHistory(currentDosage float64) float64 {
	if len(ccs.History) < 10 {
		return currentDosage
	}

	var avgRemovalRate float64
	var avgDosage float64
	for i := len(ccs.History) - 10; i < len(ccs.History); i++ {
		avgRemovalRate += ccs.History[i].TNRemovalRate
		avgDosage += ccs.History[i].DosageSetpoint
	}
	avgRemovalRate /= 10
	avgDosage /= 10

	if avgRemovalRate > 85 && avgDosage < currentDosage {
		return currentDosage * 0.95
	}

	if avgRemovalRate < 70 && avgDosage > currentDosage {
		return currentDosage * 1.05
	}

	return currentDosage
}

func (ccs *CarbonControlSystem) GetControlCommand() map[string]interface{} {
	ccs.mu.RLock()
	defer ccs.mu.RUnlock()

	return map[string]interface{}{
		"target_type": "carbon_pump",
		"target_id":   "carbon_pump_01",
		"action":      "set_dosage",
		"value":       ccs.DosageSetpoint,
		"unit":        "kg/h",
	}
}

func (co *CarbonOptimizerCore) Optimize(ccs *CarbonControlSystem, flowRate float64) float64 {
	ccs.mu.RLock()
	currentDosage := ccs.DosageSetpoint
	currentRemoval := ccs.TNRemovalRate
	history := ccs.History
	ccs.mu.RUnlock()

	if currentRemoval >= co.TargetTNRemovalRate && currentDosage > 0 {
		canReduce := true
		testDosage := currentDosage * 0.9

		for _, record := range history {
			if record.DosageSetpoint <= testDosage && record.TNRemovalRate < co.TargetTNRemovalRate {
				canReduce = false
				break
			}
		}

		if canReduce {
			return testDosage
		}
	}

	if currentRemoval < co.TargetTNRemovalRate*0.95 {
		increaseFactor := (co.TargetTNRemovalRate - currentRemoval) / 100
		newDosage := currentDosage * (1 + increaseFactor)
		carbonPerTon := newDosage * 24 / (flowRate / 1000)

		if carbonPerTon <= co.MaxCarbonPerTon {
			return newDosage
		}
	}

	return currentDosage
}

func (co *CarbonOptimizerCore) CalculateScore(removalRate, carbonPerTon float64) float64 {
	removalScore := math.Min(removalRate/co.TargetTNRemovalRate, 1.0) * 100
	costScore := math.Max(0, 100-(carbonPerTon/co.MaxCarbonPerTon)*100)

	return co.EfficiencyWeight*removalScore + co.CostWeight*costScore
}

func (co *CarbonOptimizer) GetStatus() *Status {
	co.controlSystem.mu.RLock()
	defer co.controlSystem.mu.RUnlock()

	return &Status{
		NO3Actual:                    co.controlSystem.NO3Actual,
		NO3Setpoint:                  co.controlSystem.NO3Setpoint,
		CODInfluent:                  co.controlSystem.CODInfluent,
		LastCODInfluent:               co.controlSystem.LastCODInfluent,
		CODRequired:                    co.controlSystem.CODRequired,
		CODAvailable:               co.controlSystem.CODAvailable,
		TNInfluent:                 co.controlSystem.TNInfluent,
		TNEffluent:                 co.controlSystem.TNEffluent,
		TNRemovalRate:               co.controlSystem.TNRemovalRate,
		DosageSetpoint:             co.controlSystem.DosageSetpoint,
		CarbonSourceType:            co.controlSystem.CarbonSourceType,
		CarbonSourceConcentration: co.controlSystem.CarbonSourceConcentration,
		LastUpdate:                 co.controlSystem.LastUpdate,
		LastCalculationTime:     co.controlSystem.LastCalculationTime,
		LastCODSignificantChange: co.controlSystem.LastCODSignificantChange,
	}
}

func (co *CarbonOptimizer) SetFlowRate(flowRate float64) {
	co.mu.Lock()
	defer co.mu.Unlock()
	co.flowRate = flowRate
}

func (co *CarbonOptimizer) CalculateCarbonPerTon(flowRate float64) float64 {
	co.controlSystem.mu.RLock()
	defer co.controlSystem.mu.RUnlock()

	if flowRate <= 0 {
		return 0
	}

	dailyDosageKg := co.controlSystem.DosageSetpoint * 24
	return dailyDosageKg / (flowRate / 1000)
}

func (co *CarbonOptimizer) GetControlSystem() *CarbonControlSystem {
	return co.controlSystem
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
