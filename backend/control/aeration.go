package control

import (
	"fmt"
	"log"
	"sync"
	"time"

	"sewage-treatment/backend/config"
	"sewage-treatment/backend/influxdb"
	"sewage-treatment/backend/models"
	"sewage-treatment/backend/mqtt"
)

type PIDController struct {
	Kp float64
	Ki float64
	Kd float64

	integral   float64
	prevError  float64
	prevTime   time.Time
	maxOutput  float64
	minOutput  float64
	setpoint   float64
}

type AerationSection struct {
	Section     int
	DOController *PIDController
	NH3Controller *PIDController
	CurrentDO    float64
	CurrentNH3   float64
	AerationRate float64
	LastUpdate   time.Time
	mu           sync.Mutex
}

type AerationController struct {
	sections   map[int]*AerationSection
	feedForward float64
	running    bool
	stopChan   chan struct{}
}

var AerationCtl *AerationController

func NewPIDController(kp, ki, kd float64, setpoint, minOut, maxOut float64) *PIDController {
	return &PIDController{
		Kp:        kp,
		Ki:        ki,
		Kd:        kd,
		setpoint:  setpoint,
		minOutput: minOut,
		maxOutput: maxOut,
		prevTime:  time.Now(),
	}
}

func (pid *PIDController) Compute(input float64) float64 {
	now := time.Now()
	dt := now.Sub(pid.prevTime).Seconds()
	if dt <= 0 {
		dt = 1.0
	}

	error := pid.setpoint - input

	pid.integral += error * dt
	pid.integral = clamp(pid.integral, -10, 10)

	derivative := (error - pid.prevError) / dt

	output := pid.Kp*error + pid.Ki*pid.integral + pid.Kd*derivative

	pid.prevError = error
	pid.prevTime = now

	return clamp(output, pid.minOutput, pid.maxOutput)
}

func (pid *PIDController) Reset() {
	pid.integral = 0
	pid.prevError = 0
	pid.prevTime = time.Now()
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

func NewAerationController() *AerationController {
	sections := make(map[int]*AerationSection)

	for i := 1; i <= 3; i++ {
		sections[i] = &AerationSection{
			Section:     i,
			DOController: NewPIDController(
				config.AppConfig.Control.Aeration.PIDKp,
				config.AppConfig.Control.Aeration.PIDKi,
				config.AppConfig.Control.Aeration.PIDKd,
				2.0,
				20,
				100,
			),
			NH3Controller: NewPIDController(
				config.AppConfig.Control.Aeration.PIDKp*0.6,
				config.AppConfig.Control.Aeration.PIDKi*0.6,
				config.AppConfig.Control.Aeration.PIDKd*0.6,
				1.5,
				0,
				50,
			),
			AerationRate: 50,
		}
	}

	return &AerationController{
		sections:   sections,
		feedForward: 0.3,
		stopChan:   make(chan struct{}),
	}
}

func (ac *AerationController) Start() {
	if ac.running {
		return
	}
	ac.running = true

	interval := time.Duration(config.AppConfig.Control.Aeration.ControlInterval) * time.Second
	ticker := time.NewTicker(interval)

	go func() {
		for {
			select {
			case <-ticker.C:
				ac.computeAndPublish()
			case <-ac.stopChan:
				ticker.Stop()
				ac.running = false
				return
			}
		}
	}()

	log.Println("Aeration controller started")
}

func (ac *AerationController) Stop() {
	if !ac.running {
		return
	}
	ac.stopChan <- struct{}{}
	log.Println("Aeration controller stopped")
}

func (ac *AerationController) computeAndPublish() {
	for sectionID, section := range ac.sections {
		section.mu.Lock()

		doValue, nh3Value := ac.getSectionSensorValues(sectionID)
		section.CurrentDO = doValue
		section.CurrentNH3 = nh3Value

		doCorrection := section.DOController.Compute(doValue)
		nh3Correction := section.NH3Controller.Compute(nh3Value)

		feedForwardTerm := ac.calculateFeedForward(sectionID)

		baseAeration := 50.0
		totalAeration := baseAeration + doCorrection*0.7 + nh3Correction*0.3 + feedForwardTerm
		totalAeration = clamp(totalAeration, 20, 100)

		section.AerationRate = totalAeration
		section.LastUpdate = time.Now()

		ac.publishAerationCommand(sectionID, totalAeration)

		section.mu.Unlock()

		log.Printf("Section %d: DO=%.2f, NH3=%.2f, Aeration=%.1f%%",
			sectionID, doValue, nh3Value, totalAeration)
	}
}

func (ac *AerationController) getSectionSensorValues(section int) (do, nh3 float64) {
	start := time.Now().Add(-5 * time.Minute)
	end := time.Now()

	doSensorID := fmt.Sprintf("DO-%c-%d", 'A'+section-1, 5)
	nh3SensorID := fmt.Sprintf("NH3-%c-%d", 'A'+section-1, 4)

	doData, err := influxdb.InfluxClient.QueryLatestSensorData(doSensorID)
	if err != nil {
		do = 2.0
	} else {
		do = doData.Value
	}

	nh3Data, err := influxdb.InfluxClient.QueryLatestSensorData(nh3SensorID)
	if err != nil {
		nh3 = 1.5
	} else {
		nh3 = nh3Data.Value
	}

	return do, nh3
}

func (ac *AerationController) calculateFeedForward(section int) float64 {
	start := time.Now().Add(-30 * time.Minute)
	end := time.Now()

	nh3Trend, err := influxdb.InfluxClient.QuerySensorTrend(
		fmt.Sprintf("NH3-%c-%d", 'A'+section-1, 4),
		start, end,
	)
	if err != nil || len(nh3Trend) < 2 {
		return 0
	}

	recentNH3 := nh3Trend[len(nh3Trend)-1].Value
	previousNH3 := nh3Trend[len(nh3Trend)-2].Value
	nh3Rate := (recentNH3 - previousNH3) / 5.0

	feedForward := nh3Rate * 15 * ac.feedForward
	return clamp(feedForward, -10, 10)
}

func (ac *AerationController) publishAerationCommand(section int, rate float64) {
	blowerID := fmt.Sprintf("blower_%d", section)
	valveID := fmt.Sprintf("valve_aerobic_%d", section)

	blowerCmd := &models.ControlCommand{
		ID:        fmt.Sprintf("cmd_%d", time.Now().UnixNano()),
		Type:      "aeration",
		Target:    blowerID,
		Value:     rate,
		Unit:      "%",
		Timestamp: time.Now(),
		Source:    "aeration_controller",
	}

	if err := mqtt.PublishCommand(blowerCmd); err != nil {
		log.Printf("Failed to publish blower command: %v", err)
	}

	valveCmd := &models.ControlCommand{
		ID:        fmt.Sprintf("cmd_%d", time.Now().UnixNano()+1),
		Type:      "valve",
		Target:    valveID,
		Value:     rate * 0.9,
		Unit:      "%",
		Timestamp: time.Now(),
		Source:    "aeration_controller",
	}

	if err := mqtt.PublishCommand(valveCmd); err != nil {
		log.Printf("Failed to publish valve command: %v", err)
	}
}

func (ac *AerationController) GetSectionStatus(section int) *models.AerationControl {
	s, exists := ac.sections[section]
	if !exists {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	return &models.AerationControl{
		Section:       section,
		AerationRate: s.AerationRate,
		DO:            s.CurrentDO,
		NH3:           s.CurrentNH3,
		Timestamp:     s.LastUpdate,
	}
}

func (ac *AerationController) GetAllStatus() []*models.AerationControl {
	statuses := make([]*models.AerationControl, 0, len(ac.sections))
	for i := 1; i <= len(ac.sections); i++ {
		if s := ac.GetSectionStatus(i); s != nil {
			statuses = append(statuses, s)
		}
	}
	return statuses
}

func (ac *AerationController) GetEffluentQuality() (nh3Eff, doEff float64) {
	nh3Data, err := influxdb.InfluxClient.QueryLatestSensorData("NH3-EFF")
	if err != nil {
		nh3Eff = 1.2
	} else {
		nh3Eff = nh3Data.Value
	}

	doSensorID := fmt.Sprintf("DO-%c-%d", 'C', 10)
	doData, err := influxdb.InfluxClient.QueryLatestSensorData(doSensorID)
	if err != nil {
		doEff = 2.0
	} else {
		doEff = doData.Value
	}

	return nh3Eff, doEff
}

func (ac *AerationController) OptimizeForEffluent() {
	nh3Eff, _ := ac.GetEffluentQuality()

	nh3Target := (config.AppConfig.Control.Aeration.NH3TargetMin + config.AppConfig.Control.Aeration.NH3TargetMax) / 2
	doTarget := (config.AppConfig.Control.Aeration.DOTargetMin + config.AppConfig.Control.Aeration.DOTargetMax) / 2

	adjustFactor := 1.0
	if nh3Eff > config.AppConfig.Control.Aeration.NH3TargetMax {
		adjustFactor = 1.1 + (nh3Eff-config.AppConfig.Control.Aeration.NH3TargetMax)*0.05
	} else if nh3Eff < config.AppConfig.Control.Aeration.NH3TargetMin {
		adjustFactor = 0.9 - (config.AppConfig.Control.Aeration.NH3TargetMin-nh3Eff)*0.03
	}

	for _, section := range ac.sections {
		section.DOController.setpoint = doTarget
		section.NH3Controller.setpoint = nh3Target * adjustFactor
	}
}

func CalculateAerationEnergyConsumption() float64 {
	if AerationCtl == nil {
		return 0.35
	}

	totalRate := 0.0
	for _, s := range AerationCtl.sections {
		totalRate += s.AerationRate
	}
	avgRate := totalRate / float64(len(AerationCtl.sections))

	basePower := 120.0
	return basePower * (avgRate / 50.0) * 24.0 / 30000.0
}
