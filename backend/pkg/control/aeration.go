package control

import (
	"log"
	"math"
	"sync"
	"time"
)

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
	LastSaturatedTime         time.Time
	mu                        sync.Mutex
}

func NewPIDController(kp, ki, kd, setpoint, minOutput, maxOutput float64) *PIDController {
	return &PIDController{
		Kp:                         kp,
		Ki:                         ki,
		Kd:                         kd,
		Setpoint:                   setpoint,
		MinOutput:                  minOutput,
		MaxOutput:                  maxOutput,
		LastTime:                   time.Now(),
		IntegralSeparationThreshold: 0.5,
		AntiWindupGain:             0.5,
		OutputSaturated:            false,
	}
}

func (pid *PIDController) Update(actual float64, now time.Time) float64 {
	pid.mu.Lock()
	defer pid.mu.Unlock()

	if !pid.isDataValid(actual) {
		log.Printf("[PID] Invalid sensor data: %.2f, skipping integration", actual)
		pid.resetIntegral()
		pid.LastError = 0
		pid.LastTime = now
		pid.LastOutput = pid.LastOutput
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
		log.Printf("[PID] Integral separation active: error=%.3f, threshold=%.3f, stopping integration",
			error, pid.IntegralSeparationThreshold)
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
		pid.LastSaturatedTime = now
		log.Printf("[PID] Output saturated: pre_sat=%.3f, clamped=%.3f, applying anti-windup",
			preSatOutput, output)

		antiWindup := (preSatOutput - output) * pid.AntiWindupGain * dt
		if useIntegral {
			pid.Integral -= antiWindup
			log.Printf("[PID] Anti-windup correction: %.6f, new integral=%.6f", antiWindup, pid.Integral)
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
	log.Printf("[PID] Integral reset")
}

func (pid *PIDController) Reset() {
	pid.mu.Lock()
	defer pid.mu.Unlock()
	pid.Integral = 0
	pid.LastError = 0
	pid.Derivative = 0
	pid.LastOutput = 0
	pid.OutputSaturated = false
	pid.LastSaturatedTime = time.Time{}
	pid.LastTime = time.Now()
}

func (pid *PIDController) SetSetpoint(sp float64) {
	pid.mu.Lock()
	defer pid.mu.Unlock()
	pid.Setpoint = sp
}

func (pid *PIDController) SetIntegralSeparationThreshold(threshold float64) {
	pid.mu.Lock()
	defer pid.mu.Unlock()
	pid.IntegralSeparationThreshold = threshold
	log.Printf("[PID] Integral separation threshold set to %.3f", threshold)
}

func (pid *PIDController) SetAntiWindupGain(gain float64) {
	pid.mu.Lock()
	defer pid.mu.Unlock()
	pid.AntiWindupGain = gain
	log.Printf("[PID] Anti-windup gain set to %.3f", gain)
}

func (pid *PIDController) GetStatus() map[string]interface{} {
	pid.mu.Lock()
	defer pid.mu.Unlock()
	return map[string]interface{}{
		"kp":                          pid.Kp,
		"ki":                          pid.Ki,
		"kd":                          pid.Kd,
		"setpoint":                    pid.Setpoint,
		"integral":                    pid.Integral,
		"derivative":                  pid.Derivative,
		"last_error":                  pid.LastError,
		"last_output":                 pid.LastOutput,
		"output_saturated":            pid.OutputSaturated,
		"integral_separation_active":  math.Abs(pid.LastError) > pid.IntegralSeparationThreshold,
	}
}

type FeedforwardController struct {
	NH3Gain    float64
	FlowGain   float64
	NH3Base    float64
	FlowBase   float64
	MaxOutput  float64
	MinOutput  float64
}

func NewFeedforwardController(nh3Gain, flowGain, maxOutput, minOutput float64) *FeedforwardController {
	return &FeedforwardController{
		NH3Gain:   nh3Gain,
		FlowGain:  flowGain,
		NH3Base:   1.5,
		FlowBase:  300000,
		MaxOutput: maxOutput,
		MinOutput: minOutput,
	}
}

func (ff *FeedforwardController) Calculate(nh3Actual, flowRate float64) float64 {
	nh3Term := ff.NHGain * (nh3Actual - ff.NH3Base)
	flowTerm := ff.FlowGain * (flowRate - ff.FlowBase) / ff.FlowBase
	output := nh3Term + flowTerm
	return clamp(output, ff.MinOutput, ff.MaxOutput)
}

type AerationZone struct {
	ZoneID         string
	PID            *PIDController
	Feedforward    *FeedforwardController
	DOActual       float64
	DOSetpoint     float64
	NH3Actual      float64
	NH3Setpoint    float64
	AirFlowActual  float64
	AirFlowSetpoint float64
	ValveOpening   float64
	FanSpeed       float64
	PIDOutput      float64
	FFOutput       float64
	TotalOutput    float64
	LastUpdate     time.Time
	FlowRate       float64
}

func NewAerationZone(zoneID string, doSetpoint, nh3Setpoint float64) *AerationZone {
	return &AerationZone{
		ZoneID:      zoneID,
		DOSetpoint:  doSetpoint,
		NH3Setpoint: nh3Setpoint,
		PID: NewPIDController(
			0.8,
			0.3,
			0.1,
			doSetpoint,
			0,
			100,
		),
		Feedforward: NewFeedforwardController(
			0.5,
			0.3,
			30,
			-30,
		),
		FlowRate: 300000,
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

type AerationControlSystem struct {
	Zones        map[string]*AerationZone
	TotalAirFlow float64
	EnergyUsage  float64
	mu           sync.RWMutex
}

func NewAerationControlSystem() *AerationControlSystem {
	acs := &AerationControlSystem{
		Zones: make(map[string]*AerationZone),
	}

	zones := []string{"aerobic1", "aerobic2", "aerobic3"}
	for _, z := range zones {
		acs.Zones[z] = NewAerationZone(z, 2.0, 1.5)
	}

	return acs
}

func (acs *AerationControlSystem) UpdateZone(zoneID string, doActual, nh3Actual, flowRate float64) error {
	acs.mu.Lock()
	defer acs.mu.Unlock()

	zone, exists := acs.Zones[zoneID]
	if !exists {
		return &ControlError{Msg: "zone not found: " + zoneID}
	}

	zone.Update(doActual, nh3Actual, flowRate, time.Now())

	acs.TotalAirFlow = 0
	for _, z := range acs.Zones {
		acs.TotalAirFlow += z.AirFlowSetpoint
	}

	acs.EnergyUsage = calculateEnergyUsage(acs.TotalAirFlow)

	return nil
}

func (acs *AerationControlSystem) GetZone(zoneID string) (*AerationZone, error) {
	acs.mu.RLock()
	defer acs.mu.RUnlock()

	zone, exists := acs.Zones[zoneID]
	if !exists {
		return nil, &ControlError{Msg: "zone not found: " + zoneID}
	}
	return zone, nil
}

func (acs *AerationControlSystem) GetAllZones() map[string]*AerationZone {
	acs.mu.RLock()
	defer acs.mu.RUnlock()

	zones := make(map[string]*AerationZone)
	for k, v := range acs.Zones {
		zones[k] = v
	}
	return zones
}

func (acs *AerationControlSystem) GetControlCommands() []map[string]interface{} {
	acs.mu.RLock()
	defer acs.mu.RUnlock()

	commands := make([]map[string]interface{}, 0)

	for zoneID, zone := range acs.Zones {
		commands = append(commands, map[string]interface{}{
			"target_type":  "valve",
			"target_id":    zoneID + "_valve",
			"action":       "set_opening",
			"value":        zone.ValveOpening,
			"unit":         "%",
			"zone_id":      zoneID,
		})

		commands = append(commands, map[string]interface{}{
			"target_type":  "fan",
			"target_id":    zoneID + "_fan",
			"action":       "set_speed",
			"value":        zone.FanSpeed,
			"unit":         "%",
			"zone_id":      zoneID,
		})

		commands = append(commands, map[string]interface{}{
			"target_type":  "air_flow",
			"target_id":    zoneID + "_flow",
			"action":       "set_flow",
			"value":        zone.AirFlowSetpoint,
			"unit":         "m3/h",
			"zone_id":      zoneID,
		})
	}

	return commands
}

func calculateEnergyUsage(totalAirFlow float64) float64 {
	specificEnergy := 0.008
	return totalAirFlow * specificEnergy
}

func (acs *AerationControlSystem) CalculateEnergyPerTon(waterFlow float64) float64 {
	acs.mu.RLock()
	defer acs.mu.RUnlock()

	if waterFlow <= 0 {
		return 0
	}
	return acs.EnergyUsage * 24 / (waterFlow / 1000)
}

type ControlError struct {
	Msg string
}

func (e *ControlError) Error() string {
	return e.Msg
}

type AerationOptimizer struct {
	TargetNH3    float64
	MinDO        float64
	MaxDO        float64
	MinAirFlow   float64
	MaxAirFlow   float64
}

func NewAerationOptimizer() *AerationOptimizer {
	return &AerationOptimizer{
		TargetNH3:  1.5,
		MinDO:      1.5,
		MaxDO:      2.5,
		MinAirFlow: 500,
		MaxAirFlow: 5000,
	}
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

func (acs *AerationControlSystem) GetStatus() map[string]interface{} {
	acs.mu.RLock()
	defer acs.mu.RUnlock()

	status := make(map[string]interface{})
	status["total_air_flow"] = acs.TotalAirFlow
	status["energy_usage"] = acs.EnergyUsage
	status["zone_count"] = len(acs.Zones)

	zones := make(map[string]interface{})
	for id, zone := range acs.Zones {
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

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

var _ = log.Printf
