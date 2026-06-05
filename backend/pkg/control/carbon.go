package control

import (
	"log"
	"math"
	"sync"
	"time"
)

type CarbonControlSystem struct {
	NO3Actual           float64
	NO3Setpoint         float64
	CODInfluent         float64
	CODRequired         float64
	CODAvailable        float64
	TNInfluent          float64
	TNEffluent          float64
	TNRemovalRate       float64
	DosageSetpoint      float64
	CarbonSourceType    string
	CarbonSourceConcentration float64
	MaxDosage           float64
	MinDosage           float64
	CODToNRatio         float64
	LastUpdate          time.Time
	History             []CarbonHistoryRecord
	mu                  sync.RWMutex
}

type CarbonHistoryRecord struct {
	Timestamp      time.Time
	NO3Actual      float64
	CODInfluent    float64
	DosageSetpoint float64
	TNRemovalRate  float64
}

func NewCarbonControlSystem() *CarbonControlSystem {
	return &CarbonControlSystem{
		NO3Setpoint:              10.0,
		MaxDosage:                50.0,
		MinDosage:                0.0,
		CODToNRatio:              5.0,
		CarbonSourceType:         "acetate",
		CarbonSourceConcentration: 500000.0,
		History:                  make([]CarbonHistoryRecord, 0, 100),
	}
}

func (ccs *CarbonControlSystem) Update(no3Actual, codInfluent, tnInfluent, tnEffluent, flowRate float64) {
	ccs.mu.Lock()
	defer ccs.mu.Unlock()

	ccs.NO3Actual = no3Actual
	ccs.CODInfluent = codInfluent
	ccs.TNInfluent = tnInfluent
	ccs.TNEffluent = tnEffluent
	ccs.LastUpdate = time.Now()

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
}

func (ccs *CarbonControlSystem) calculateOptimalDosage(flowRate float64) {
	no3N := ccs.NO3Actual
	tnEstimate := ccs.TNEffluent

	requiredNO3Removal := math.Max(0, no3N - ccs.NO3Setpoint)
	requiredTNRemoval := math.Max(0, tnEstimate - 15.0)

	totalNRemoval := math.Max(requiredNO3Removal, requiredTNRemoval)

	ccs.CODRequired = totalNRemoval * ccs.CODToNRatio * 1.2

	ccs.CODAvailable = math.Max(0, ccs.CODInfluent-50) * 0.7

	additionalCODNeeded := math.Max(0, ccs.CODRequired-ccs.CODAvailable)

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

func (ccs *CarbonControlSystem) GetStatus() map[string]interface{} {
	ccs.mu.RLock()
	defer ccs.mu.RUnlock()

	return map[string]interface{}{
		"no3_actual":             ccs.NO3Actual,
		"no3_setpoint":           ccs.NO3Setpoint,
		"cod_influent":           ccs.CODInfluent,
		"cod_required":           ccs.CODRequired,
		"cod_available":          ccs.CODAvailable,
		"tn_influent":            ccs.TNInfluent,
		"tn_effluent":            ccs.TNEffluent,
		"tn_removal_rate":        ccs.TNRemovalRate,
		"dosage_setpoint":        ccs.DosageSetpoint,
		"carbon_source_type":     ccs.CarbonSourceType,
		"carbon_source_concentration": ccs.CarbonSourceConcentration,
		"cod_to_n_ratio":         ccs.CODToNRatio,
		"last_update":            ccs.LastUpdate,
	}
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

func (ccs *CarbonControlSystem) CalculateCarbonPerTon(flowRate float64) float64 {
	ccs.mu.RLock()
	defer ccs.mu.RUnlock()

	if flowRate <= 0 {
		return 0
	}

	dailyDosageKg := ccs.DosageSetpoint * 24
	return dailyDosageKg / (flowRate / 1000)
}

func (ccs *CarbonControlSystem) SetCarbonSourceType(sourceType string, concentration float64) {
	ccs.mu.Lock()
	defer ccs.mu.Unlock()

	ccs.CarbonSourceType = sourceType
	ccs.CarbonSourceConcentration = concentration
}

func (ccs *CarbonControlSystem) SetCODToNRatio(ratio float64) {
	ccs.mu.Lock()
	defer ccs.mu.Unlock()
	ccs.CODToNRatio = ratio
}

func (ccs *CarbonControlSystem) SetDosageLimits(min, max float64) {
	ccs.mu.Lock()
	defer ccs.mu.Unlock()
	ccs.MinDosage = min
	ccs.MaxDosage = max
}

type CarbonOptimizer struct {
	TargetTNRemovalRate float64
	MaxCarbonPerTon     float64
	EfficiencyWeight    float64
	CostWeight          float64
}

func NewCarbonOptimizer() *CarbonOptimizer {
	return &CarbonOptimizer{
		TargetTNRemovalRate: 80.0,
		MaxCarbonPerTon:     0.5,
		EfficiencyWeight:    0.7,
		CostWeight:          0.3,
	}
}

func (co *CarbonOptimizer) Optimize(ccs *CarbonControlSystem, flowRate float64) float64 {
	ccs.mu.RLock()
	defer ccs.mu.RUnlock()

	currentDosage := ccs.DosageSetpoint
	currentRemoval := ccs.TNRemovalRate

	if currentRemoval >= co.TargetTNRemovalRate && currentDosage > 0 {
		canReduce := true
		testDosage := currentDosage * 0.9

		for _, record := range ccs.History {
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

func (co *CarbonOptimizer) CalculateScore(removalRate, carbonPerTon float64) float64 {
	removalScore := math.Min(removalRate/co.TargetTNRemovalRate, 1.0) * 100
	costScore := math.Max(0, 100-(carbonPerTon/co.MaxCarbonPerTon)*100)

	return co.EfficiencyWeight*removalScore + co.CostWeight*costScore
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

var _ = math.Abs
