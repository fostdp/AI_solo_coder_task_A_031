package aeration_controller

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"sewage-plant-system/pkg/models"
	sc "sewage-plant-system/pkg/sensor_collector"
)

type PIDConfig struct {
	Kp                        float64 `mapstructure:"kp"`
	Ki                        float64 `mapstructure:"ki"`
	Kd                        float64 `mapstructure:"kd"`
	IntegralSeparationThreshold float64 `mapstructure:"integral_separation_threshold"`
	AntiWindupGain            float64 `mapstructure:"anti_windup_gain"`
}

type FeedforwardConfig struct {
	NH3Gain  float64 `mapstructure:"nh3_gain"`
	FlowGain float64 `mapstructure:"flow_gain"`
	NH3Base  float64 `mapstructure:"nh3_base"`
	FlowBase float64 `mapstructure:"flow_base"`
}

type ZoneConfig struct {
	ZoneID       string  `mapstructure:"zone_id"`
	DOSetpoint   float64 `mapstructure:"do_setpoint"`
	NH3Setpoint  float64 `mapstructure:"nh3_setpoint"`
	DOMin        float64 `mapstructure:"do_min"`
	DOMax        float64 `mapstructure:"do_max"`
	MinAirFlow   float64 `mapstructure:"min_air_flow"`
	MaxAirFlow   float64 `mapstructure:"max_air_flow"`
}

type Config struct {
	PID         PIDConfig         `mapstructure:"pid"`
	Feedforward FeedforwardConfig `mapstructure:"feedforward"`
	Zones       []ZoneConfig      `mapstructure:"zones"`
	TargetNH3   float64           `mapstructure:"target_nh3"`
	ControlInterval time.Duration `mapstructure:"control_interval"`
}

type ControlOutput struct {
	Command *models.ControlCommand
	ZoneID  string
}

type ZoneStatus struct {
	*models.AerationControl
	PIDStatus map[string]interface{}
}

type AerationController struct {
	cfg              Config
	zones            map[string]*AerationZone
	optimizer        *AerationOptimizer
	validDataChan    <-chan *sc.ValidatedSensorData
	controlOutputChan chan<- *ControlOutput
	statusChan       chan<- map[string]interface{}
	sensorValues     map[string]map[string]float64
	flowRate         float64
	mu               sync.RWMutex
	lastControlTime  time.Time
}

type PIDController struct {
	Kp                        float64
	Ki                        float64
	Kd                        float64
	Setpoint                  float64
	MinOutput                 float64
	MaxOutput                 float64
	Integral                  float64
	LastError                 float64
	LastTime                  time.Time
	Derivative                float64
	LastOutput                float64
	IntegralSeparationThreshold float64
	AntiWindupGain            float64
	OutputSaturated           bool
	mu                        sync.Mutex
}

type FeedforwardController struct {
	NH3Gain   float64
	FlowGain  float64
	NH3Base   float64
	FlowBase  float64
	MaxOutput float64
	MinOutput float64
}

type AerationZone struct {
	ZoneID         string
	Config         ZoneConfig
	PID            *PIDController
	Feedforward    *FeedforwardController
	DOActual       float64
	DOSetpoint     float64
	NH3Actual      float64
	NH3Setpoint    float64
	AirFlowSetpoint float64
	ValveOpening   float64
	FanSpeed       float64
	PIDOutput      float64
	FFOutput       float64
	TotalOutput    float64
	LastUpdate     time.Time
	FlowRate       float64
}

type AerationOptimizer struct {
	TargetNH3  float64
	MinDO      float64
	MaxDO      float64
	MinAirFlow float64
	MaxAirFlow float64
}

func New(cfg Config, validDataChan <-chan *sc.ValidatedSensorData, controlOutputChan chan<- *ControlOutput, statusChan chan<- map[string]interface{}) *AerationController {
	ac := &AerationController{
		cfg:               cfg,
		zones:             make(map[string]*AerationZone),
		validDataChan:     validDataChan,
		controlOutputChan: controlOutputChan,
		statusChan:        statusChan,
		sensorValues:      make(map[string]map[string]float64),
		flowRate:          cfg.Feedforward.FlowBase,
	}

	if len(cfg.Zones) == 0 {
		cfg.Zones = []ZoneConfig{
			{ZoneID: "aerobic1", DOSetpoint: 2.0, NH3Setpoint: 1.5, DOMin: 1.5, DOMax: 2.5, MinAirFlow: 500, MaxAirFlow: 5000},
			{ZoneID: "aerobic2", DOSetpoint: 2.0, NH3Setpoint: 1.5, DOMin: 1.5, DOMax: 2.5, MinAirFlow: 500, MaxAirFlow: 5000},
			{ZoneID: "aerobic3", DOSetpoint: 2.0, NH3Setpoint: 1.5, DOMin: 1.5, DOMax: 2.5, MinAirFlow: 500, MaxAirFlow: 5000},
		}
	}

	for _, zc := range cfg.Zones {
		ac.zones[zc.ZoneID] = NewAerationZone(zc, cfg)
	}

	ac.optimizer = &AerationOptimizer{
		TargetNH3:  cfg.TargetNH3,
		MinDO:      cfg.Zones[0].DOMin,
		MaxDO:      cfg.Zones[0].DOMax,
		MinAirFlow: cfg.Zones[0].MinAirFlow,
		MaxAirFlow: cfg.Zones[0].MaxAirFlow,
	}

	if ac.optimizer.TargetNH3 == 0 {
		ac.optimizer.TargetNH3 = 1.5
	}

	return ac
}

func NewAerationZone(zc ZoneConfig, cfg Config) *AerationZone {
	return &AerationZone{
		ZoneID:      zc.ZoneID,
		Config:      zc,
		DOSetpoint:  zc.DOSetpoint,
		NH3Setpoint: zc.NH3Setpoint,
		PID: NewPIDController(
			cfg.PID.Kp,
			cfg.PID.Ki,
			cfg.PID.Kd,
			zc.DOSetpoint,
			0,
			100,
			cfg.PID.IntegralSeparationThreshold,
			cfg.PID.AntiWindupGain,
		),
		Feedforward: NewFeedforwardController(
			cfg.Feedforward.NH3Gain,
			cfg.Feedforward.FlowGain,
			30,
			-30,
			cfg.Feedforward.NH3Base,
			cfg.Feedforward.FlowBase,
		),
		FlowRate: cfg.Feedforward.FlowBase,
	}
}

func NewPIDController(kp, ki, kd, setpoint, minOutput, maxOutput, integralSepThreshold, antiWindupGain float64) *PIDController {
	if integralSepThreshold == 0 {
		integralSepThreshold = 0.5
	}
	if antiWindupGain == 0 {
		antiWindupGain = 0.5
	}
	return &PIDController{
		Kp:                         kp,
		Ki:                         ki,
		Kd:                         kd,
		Setpoint:                   setpoint,
		MinOutput:                  minOutput,
		MaxOutput:                  maxOutput,
		IntegralSeparationThreshold: integralSepThreshold,
		AntiWindupGain:             antiWindupGain,
		LastTime:                   time.Now(),
	}
}

func NewFeedforwardController(nh3Gain, flowGain, maxOutput, minOutput, nh3Base, flowBase float64) *FeedforwardController {
	if nh3Base == 0 {
		nh3Base = 1.5
	}
	if flowBase == 0 {
		flowBase = 300000
	}
	return &FeedforwardController{
		NH3Gain:   nh3Gain,
		FlowGain:  flowGain,
		NH3Base:   nh3Base,
		FlowBase:  flowBase,
		MaxOutput: maxOutput,
		MinOutput: minOutput,
	}
}

func (ac *AerationController) Start() {
	go ac.processSensorData()
	go ac.controlLoop()
}

func (ac *AerationController) processSensorData() {
	for data := range ac.validDataChan {
		if !data.IsValid {
			continue
		}

		ac.mu.Lock()
		if ac.sensorValues[data.Location] == nil {
			ac.sensorValues[data.Location] = make(map[string]float64)
		}
		ac.sensorValues[data.Location][string(data.Type)] = data.Value
		ac.mu.Unlock()
	}
}

func (ac *AerationController) controlLoop() {
	interval := ac.cfg.ControlInterval
	if interval == 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		ac.RunControlCycle()
	}
}

func (ac *AerationController) RunControlCycle() {
	ac.mu.Lock()
	now := time.Now()
	ac.lastControlTime = now

	for zoneID, zone := range ac.zones {
		doValue := ac.getAverageSensorValue(models.SensorTypeDO, zoneID)
		nh3Value := ac.getAverageSensorValue(models.SensorTypeNH3, zoneID)

		if doValue > 0 && nh3Value > 0 {
			zone.Update(doValue, nh3Value, ac.flowRate, now)
		}
	}

	effluentNH3 := ac.getAverageSensorValue(models.SensorTypeNH3, "effluent")
	if effluentNH3 > 0 {
		ac.optimizer.Optimize(ac.zones, effluentNH3)
	}

	ac.mu.Unlock()

	ac.generateControlCommands()
	ac.broadcastStatus()
}

func (ac *AerationController) getAverageSensorValue(sensorType models.SensorType, location string) float64 {
	values, ok := ac.sensorValues[location]
	if !ok {
		return 0
	}
	value, ok := values[string(sensorType)]
	if !ok {
		return 0
	}
	return value
}

func (ac *AerationController) generateControlCommands() {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	for zoneID, zone := range ac.zones {
		commands := zone.GetControlCommands()
		for _, cmd := range commands {
			controlCmd := &models.ControlCommand{
				CommandID:  fmt.Sprintf("cmd_%d", time.Now().UnixNano()),
				TargetType: cmd["target_type"].(string),
				TargetID:   cmd["target_id"].(string),
				Action:     cmd["action"].(string),
				Value:      cmd["value"].(float64),
				Unit:       cmd["unit"].(string),
				Timestamp:  time.Now(),
				Source:     "aeration_control",
			}

			select {
			case ac.controlOutputChan <- &ControlOutput{Command: controlCmd, ZoneID: zoneID}:
			default:
				log.Printf("[AERATION] ControlOutputChan full, dropping command for %s", zoneID)
			}
		}
	}
}

func (ac *AerationController) broadcastStatus() {
	status := ac.GetStatus()
	select {
	case ac.statusChan <- status:
	default:
	}
}

func (az *AerationZone) Update(doActual, nh3Actual, flowRate float64, now time.Time) {
	az.DOActual = doActual
	az.NH3Actual = nh3Actual
	az.FlowRate = flowRate
	az.LastUpdate = now

	dynamicSetpoint := calculateDynamicDOSetpoint(nh3Actual, az.NH3Setpoint, az.DOSetpoint)
	az.PID.SetSetpoint(dynamicSetpoint)

	az.PIDOutput = az.PID.Update(doActual, now)
	az.FFOutput = az.Feedforward.Calculate(nh3Actual, flowRate)

	az.TotalOutput = az.PIDOutput + az.FFOutput
	az.TotalOutput = clamp(az.TotalOutput, 0, 100)

	az.AirFlowSetpoint = calculateAirFlow(az.TotalOutput, flowRate)
	az.ValveOpening = az.TotalOutput
	az.FanSpeed = calculateFanSpeed(az.AirFlowSetpoint)
}

func (pid *PIDController) Update(actual float64, now time.Time) float64 {
	pid.mu.Lock()
	defer pid.mu.Unlock()

	if !pid.isDataValid(actual) {
		log.Printf("[PID] Invalid sensor data: %.2f, skipping integration", actual)
		pid.resetIntegral()
		pid.LastError = 0
		pid.LastTime = now
		if pid.LastOutput == 0 {
			pid.LastOutput = (pid.MinOutput + pid.MaxOutput) / 2
		}
		return pid.LastOutput
	}

	error := pid.Setpoint - actual
	dt := now.Sub(pid.LastTime).Seconds()

	if dt <= 0 {
		dt = 0.001
	}

	absError := math.Abs(error)
	useIntegral := true
	if absError > pid.IntegralSeparationThreshold {
		useIntegral = false
	}

	if useIntegral {
		pid.Integral += error * dt
	}

	if !pid.LastTime.IsZero() {
		pid.Derivative = (error - pid.LastError) / dt
	}

	pidOutput := pid.Kp * error
	if useIntegral {
		pidOutput += pid.Ki * pid.Integral
	}
	pidOutput += pid.Kd * pid.Derivative

	preSatOutput := pidOutput
	output := clamp(pidOutput, pid.MinOutput, pid.MaxOutput)

	pid.OutputSaturated = math.Abs(preSatOutput-output) > 0.001
	if pid.OutputSaturated {
		antiWindup := (preSatOutput - output) * pid.AntiWindupGain * dt
		if useIntegral {
			pid.Integral -= antiWindup
		}
	}

	pid.Integral = clamp(pid.Integral, pid.MinOutput/pid.Ki, pid.MaxOutput/pid.Ki)

	pid.LastError = error
	pid.LastTime = now
	pid.LastOutput = output

	return output
}

func (pid *PIDController) isDataValid(actual float64) bool {
	if math.IsNaN(actual) || math.IsInf(actual, 0) {
		return false
	}
	if actual < 0 {
		return false
	}
	if actual > pid.Setpoint*10 {
		return false
	}
	return true
}

func (pid *PIDController) resetIntegral() {
	pid.Integral = 0
	pid.OutputSaturated = false
}

func (pid *PIDController) SetSetpoint(sp float64) {
	pid.mu.Lock()
	defer pid.mu.Unlock()
	pid.Setpoint = sp
}

func (pid *PIDController) GetStatus() map[string]interface{} {
	pid.mu.Lock()
	defer pid.mu.Unlock()
	return map[string]interface{}{
		"kp":                         pid.Kp,
		"ki":                         pid.Ki,
		"kd":                         pid.Kd,
		"setpoint":                   pid.Setpoint,
		"integral":                   pid.Integral,
		"derivative":                 pid.Derivative,
		"last_error":                 pid.LastError,
		"last_output":                pid.LastOutput,
		"output_saturated":           pid.OutputSaturated,
		"integral_separation_active": math.Abs(pid.LastError) > pid.IntegralSeparationThreshold,
	}
}

func (ff *FeedforwardController) Calculate(nh3Actual, flowRate float64) float64 {
	nh3Term := ff.NHGain * (nh3Actual - ff.NH3Base)
	flowTerm := ff.FlowGain * (flowRate - ff.FlowBase) / ff.FlowBase
	output := nh3Term + flowTerm
	return clamp(output, ff.MinOutput, ff.MaxOutput)
}

func calculateDynamicDOSetpoint(nh3Actual, nh3Setpoint, baseDOSetpoint float64) float64 {
	nh3Ratio := nh3Actual / nh3Setpoint
	var adjustment float64

	if nh3Ratio > 1.2 {
		adjustment = 0.5
	} else if nh3Ratio > 1.0 {
		adjustment = 0.3
	} else if nh3Ratio < 0.8 {
		adjustment = -0.3
	} else if nh3Ratio < 0.9 {
		adjustment = -0.1
	}

	return clamp(baseDOSetpoint+adjustment, 1.5, 2.5)
}

func calculateAirFlow(controlOutput, flowRate float64) float64 {
	baseAirFlow := flowRate * 0.008
	return baseAirFlow * (0.5 + controlOutput/100*1.0)
}

func calculateFanSpeed(airFlowSetpoint float64) float64 {
	maxAirFlow := 5000.0
	speed := (airFlowSetpoint / maxAirFlow) * 100
	return clamp(speed, 30, 100)
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

func (az *AerationZone) GetControlCommands() []map[string]interface{} {
	commands := make([]map[string]interface{}, 0)

	commands = append(commands, map[string]interface{}{
		"target_type": "valve",
		"target_id":   az.ZoneID + "_valve",
		"action":      "set_opening",
		"value":       az.ValveOpening,
		"unit":        "%",
		"zone_id":     az.ZoneID,
	})

	commands = append(commands, map[string]interface{}{
		"target_type": "fan",
		"target_id":   az.ZoneID + "_fan",
		"action":      "set_speed",
		"value":       az.FanSpeed,
		"unit":        "%",
		"zone_id":     az.ZoneID,
	})

	commands = append(commands, map[string]interface{}{
		"target_type": "air_flow",
		"target_id":   az.ZoneID + "_flow",
		"action":      "set_flow",
		"value":       az.AirFlowSetpoint,
		"unit":        "m3/h",
		"zone_id":     az.ZoneID,
	})

	return commands
}

func (ao *AerationOptimizer) Optimize(zones map[string]*AerationZone, effluentNH3 float64) {
	nh3Error := effluentNH3 - ao.TargetNH3
	adjustmentFactor := 1.0

	if math.Abs(nh3Error) > 0.5 {
		if nh3Error > 0 {
			adjustmentFactor = 1.1
		} else {
			adjustmentFactor = 0.9
		}
	}

	for _, zone := range zones {
		zone.DOSetpoint = clamp(zone.DOSetpoint*adjustmentFactor, ao.MinDO, ao.MaxDO)
		zone.AirFlowSetpoint = clamp(zone.AirFlowSetpoint*adjustmentFactor, ao.MinAirFlow, ao.MaxAirFlow)
	}
}

func (ac *AerationController) GetStatus() map[string]interface{} {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	status := make(map[string]interface{})

	var totalAirFlow float64
	for _, zone := range ac.zones {
		totalAirFlow += zone.AirFlowSetpoint
	}
	status["total_air_flow"] = totalAirFlow
	status["energy_usage"] = totalAirFlow * 0.008
	status["zone_count"] = len(ac.zones)
	status["last_control_time"] = ac.lastControlTime

	zones := make(map[string]interface{})
	for id, zone := range ac.zones {
		pidStatus := zone.PID.GetStatus()
		zones[id] = map[string]interface{}{
			"do_actual":         zone.DOActual,
			"do_setpoint":       zone.DOSetpoint,
			"nh3_actual":        zone.NH3Actual,
			"air_flow_setpoint": zone.AirFlowSetpoint,
			"valve_opening":     zone.ValveOpening,
			"fan_speed":         zone.FanSpeed,
			"pid_output":        zone.PIDOutput,
			"ff_output":         zone.FFOutput,
			"total_output":      zone.TotalOutput,
			"pid_status":        pidStatus,
		}
	}
	status["zones"] = zones

	return status
}

func (ac *AerationController) SetFlowRate(flowRate float64) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.flowRate = flowRate
}

func (ac *AerationController) GetAllZones() map[string]*AerationZone {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	result := make(map[string]*AerationZone)
	for k, v := range ac.zones {
		result[k] = v
	}
	return result
}

func (ac *AerationController) CalculateEnergyPerTon(waterFlow float64) float64 {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	if waterFlow <= 0 {
		return 0
	}

	var totalAirFlow float64
	for _, zone := range ac.zones {
		totalAirFlow += zone.AirFlowSetpoint
	}
	energyUsage := totalAirFlow * 0.008
	return energyUsage * 24 / (waterFlow / 1000)
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
