package control

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"sewage-treatment/backend/config"
	"sewage-treatment/backend/influxdb"
	"sewage-treatment/backend/models"
	"sewage-treatment/backend/mqtt"
)

type CarbonOptimizer struct {
	CurrentNO3    float64
	CurrentCOD    float64
	DosageRate    float64
	TNRemoval     float64
	LastUpdate    time.Time
	running       bool
	stopChan      chan struct{}
	mu            sync.Mutex

	historicalData []CarbonDataPoint
}

type CarbonDataPoint struct {
	NO3        float64
	COD        float64
	Dosage     float64
	TNRemoval  float64
	Timestamp  time.Time
}

var CarbonOpt *CarbonOptimizer

func NewCarbonOptimizer() *CarbonOptimizer {
	return &CarbonOptimizer{
		DosageRate:     50.0,
		stopChan:       make(chan struct{}),
		historicalData: make([]CarbonDataPoint, 0, 100),
	}
}

func (co *CarbonOptimizer) Start() {
	if co.running {
		return
	}
	co.running = true

	interval := time.Duration(config.AppConfig.Control.Carbon.ControlInterval) * time.Second
	ticker := time.NewTicker(interval)

	go func() {
		for {
			select {
			case <-ticker.C:
				co.computeAndPublish()
			case <-co.stopChan:
				ticker.Stop()
				co.running = false
				return
			}
		}
	}()

	log.Println("Carbon source optimizer started")
}

func (co *CarbonOptimizer) Stop() {
	if !co.running {
		return
	}
	co.stopChan <- struct{}{}
	log.Println("Carbon source optimizer stopped")
}

func (co *CarbonOptimizer) computeAndPublish() {
	co.mu.Lock()
	defer co.mu.Unlock()

	no3Value, codValue := co.getInputValues()
	co.CurrentNO3 = no3Value
	co.CurrentCOD = codValue

	optimalDosage := co.calculateOptimalDosage(no3Value, codValue)
	co.DosageRate = optimalDosage

	tnRemoval := co.estimateTNRemoval(no3Value, codValue, optimalDosage)
	co.TNRemoval = tnRemoval
	co.LastUpdate = time.Now()

	co.recordDataPoint(no3Value, codValue, optimalDosage, tnRemoval)
	co.publishCarbonCommand(optimalDosage)

	log.Printf("Carbon Optimization: NO3=%.2f mg/L, COD=%.2f mg/L, Dosage=%.1f L/h, TN Removal=%.1f%%",
		no3Value, codValue, optimalDosage, tnRemoval)
}

func (co *CarbonOptimizer) getInputValues() (no3, cod float64) {
	no3SensorID := "NO3-A-3"
	no3Data, err := influxdb.InfluxClient.QueryLatestSensorData(no3SensorID)
	if err != nil {
		no3 = 2.0
	} else {
		no3 = no3Data.Value
	}

	codSensorID := "COD-IN"
	codData, err := influxdb.InfluxClient.QueryLatestSensorData(codSensorID)
	if err != nil {
		cod = 300.0
	} else {
		cod = codData.Value
	}

	return no3, cod
}

func (co *CarbonOptimizer) calculateOptimalDosage(no3, cod float64) float64 {
	tnTarget := config.AppConfig.Control.Carbon.TNTarget
	codPerCarbon := config.AppConfig.Control.Carbon.CODPerCarbonUnit

	no3N := no3
	requiredCOD := no3N * 4.5

	availableCOD := cod - 50
	if availableCOD < 0 {
		availableCOD = 0
	}

	codDeficit := requiredCOD - availableCOD
	if codDeficit < 0 {
		codDeficit = 0
	}

	baseDosage := codDeficit / codPerCarbon / 1000.0

	tnData, err := influxdb.InfluxClient.QueryLatestSensorData("TN-EFF")
	var currentTN float64
	if err != nil {
		currentTN = 12.0
	} else {
		currentTN = tnData.Value
	}

	feedbackGain := 5.0
	tnError := currentTN - tnTarget
	feedbackDosage := feedbackGain * tnError / 10.0
	if feedbackDosage < 0 {
		feedbackDosage = 0
	}

	learningFactor := co.calculateLearningFactor()

	totalDosage := (baseDosage*0.7 + feedbackDosage*0.3) * learningFactor

	influentFlow := 300000.0 / 24.0
	totalDosage = totalDosage * influentFlow / 1000.0

	totalDosage = clamp(totalDosage, 10, 200)

	return totalDosage
}

func (co *CarbonOptimizer) calculateLearningFactor() float64 {
	if len(co.historicalData) < 10 {
		return 1.0
	}

	recentData := co.historicalData
	if len(recentData) > 20 {
		recentData = recentData[len(recentData)-20:]
	}

	var highEfficiencyCount int
	var totalCount int

	for i := 1; i < len(recentData); i++ {
		prev := recentData[i-1]
		curr := recentData[i]

		if curr.TNRemoval > prev.TNRemoval && curr.Dosage < prev.Dosage {
			highEfficiencyCount++
		}
		totalCount++
	}

	if totalCount == 0 {
		return 1.0
	}

	efficiency := float64(highEfficiencyCount) / float64(totalCount)
	return 0.9 + efficiency*0.2
}

func (co *CarbonOptimizer) estimateTNRemoval(no3, cod, dosage float64) float64 {
	infTN := 35.0
	effTN := 15.0

	codRatio := cod / 300.0
	no3Ratio := no3 / 3.0
	dosageRatio := dosage / 100.0

	removalFactor := 0.75 + 0.1*codRatio - 0.05*no3Ratio + 0.08*dosageRatio
	removalFactor = clamp(removalFactor, 0.5, 0.95)

	estimatedRemoval := removalFactor * 100

	tnEff := infTN * (1 - removalFactor)
	if tnEff < effTN {
		excessRemoval := (effTN - tnEff) / effTN
		estimatedRemoval -= excessRemoval * 10
	}

	return clamp(estimatedRemoval, 50, 95)
}

func (co *CarbonOptimizer) recordDataPoint(no3, cod, dosage, tnRemoval float64) {
	point := CarbonDataPoint{
		NO3:       no3,
		COD:       cod,
		Dosage:    dosage,
		TNRemoval: tnRemoval,
		Timestamp: time.Now(),
	}

	co.historicalData = append(co.historicalData, point)
	if len(co.historicalData) > 100 {
		co.historicalData = co.historicalData[1:]
	}
}

func (co *CarbonOptimizer) publishCarbonCommand(dosage float64) {
	pumpID := "carbon_pump"
	valveID := "valve_carbon"

	pumpCmd := &models.ControlCommand{
		ID:        fmt.Sprintf("cmd_%d", time.Now().UnixNano()),
		Type:      "carbon_dosage",
		Target:    pumpID,
		Value:     dosage,
		Unit:      "L/h",
		Timestamp: time.Now(),
		Source:    "carbon_optimizer",
	}

	if err := mqtt.PublishCommand(pumpCmd); err != nil {
		log.Printf("Failed to publish carbon pump command: %v", err)
	}

	valveCmd := &models.ControlCommand{
		ID:        fmt.Sprintf("cmd_%d", time.Now().UnixNano()+1),
		Type:      "valve",
		Target:    valveID,
		Value:     dosage / 200.0 * 100,
		Unit:      "%",
		Timestamp: time.Now(),
		Source:    "carbon_optimizer",
	}

	if err := mqtt.PublishCommand(valveCmd); err != nil {
		log.Printf("Failed to publish carbon valve command: %v", err)
	}
}

func (co *CarbonOptimizer) GetStatus() *models.CarbonControl {
	co.mu.Lock()
	defer co.mu.Unlock()

	return &models.CarbonControl{
		DosageRate: co.DosageRate,
		NO3In:      co.CurrentNO3,
		CODIn:      co.CurrentCOD,
		TNRemoval:  co.TNRemoval,
		Timestamp:  co.LastUpdate,
	}
}

func CalculateCarbonConsumption() float64 {
	if CarbonOpt == nil {
		return 0.025
	}

	dailyDosage := CarbonOpt.DosageRate * 24.0
	carbonConcentration := 400000.0

	return (dailyDosage * carbonConcentration / 1000.0) / 300000.0
}

func CalculateRemovalRate(sensorType string) float64 {
	inletID := ""
	outletID := ""

	switch sensorType {
	case "COD":
		inletID = "COD-IN"
		outletID = "COD-EFF"
	case "NH3":
		inletID = "NH3-IN"
		outletID = "NH3-EFF"
	case "TN":
		inletID = "TN-IN"
		outletID = "TN-EFF"
	case "TP":
		inletID = "TP-IN"
		outletID = "TP-EFF"
	default:
		return 85.0
	}

	inletData, err1 := influxdb.InfluxClient.QueryLatestSensorData(inletID)
	outletData, err2 := influxdb.InfluxClient.QueryLatestSensorData(outletID)

	if err1 != nil || err2 != nil {
		switch sensorType {
		case "COD":
			return 88.0
		case "NH3":
			return 92.0
		case "TN":
			return 72.0
		case "TP":
			return 85.0
		default:
			return 85.0
		}
	}

	if inletData.Value <= 0 {
		return 85.0
	}

	removal := (inletData.Value - outletData.Value) / inletData.Value * 100
	return clamp(removal, 0, 100)
}

func CalculateKPIValues() (energy, carbon, removal float64) {
	energy = CalculateAerationEnergyConsumption()
	carbon = CalculateCarbonConsumption()

	nh3Removal := CalculateRemovalRate("NH3")
	tnRemoval := CalculateRemovalRate("TN")
	codRemoval := CalculateRemovalRate("COD")

	removal = (nh3Removal + tnRemoval + codRemoval) / 3.0

	now := time.Now()

	kpiEnergy := &models.KPIData{
		ID:        "kpi_energy",
		Type:      "energy_consumption",
		Value:     energy,
		Unit:      "kWh/m³",
		Timestamp: now,
	}
	if err := influxdb.InfluxClient.WriteKPI(kpiEnergy); err != nil {
		log.Printf("Failed to write energy KPI: %v", err)
	}

	kpiCarbon := &models.KPIData{
		ID:        "kpi_carbon",
		Type:      "carbon_consumption",
		Value:     carbon,
		Unit:      "kg/m³",
		Timestamp: now,
	}
	if err := influxdb.InfluxClient.WriteKPI(kpiCarbon); err != nil {
		log.Printf("Failed to write carbon KPI: %v", err)
	}

	kpiRemoval := &models.KPIData{
		ID:        "kpi_removal",
		Type:      "removal_rate",
		Value:     removal,
		Unit:      "%",
		Timestamp: now,
	}
	if err := influxdb.InfluxClient.WriteKPI(kpiRemoval); err != nil {
		log.Printf("Failed to write removal KPI: %v", err)
	}

	return energy, carbon, removal
}

func StartKPICalculation() {
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		for range ticker.C {
			CalculateKPIValues()
		}
	}()
	log.Println("KPI calculation started")
}
