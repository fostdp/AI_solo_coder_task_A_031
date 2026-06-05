package aeration_controller

import (
	"testing"
	"time"

	sc "sewage-plant-system/pkg/sensor_collector"
)

func TestPIDControllerUpdate(t *testing.T) {
	pid := NewPIDController(2.0, 0.1, 0.05, 2.0, 0, 100, 1.0, 0.5)

	output := pid.Update(1.5, 2.0, time.Second)
	if output == 0 {
		t.Error("Expected non-zero output from PID controller")
	}
	if output < 0 || output > 100 {
		t.Errorf("Output should be within bounds [0, 100], got %f", output)
	}
}

func TestPIDIntegralSaturation(t *testing.T) {
	pid := NewPIDController(1.0, 1.0, 0, 1.0, 0, 100, 0.5, 0.5)

	for i := 0; i < 10; i++ {
		pid.Update(1.0, 5.0, time.Second)
	}

	if pid.Integral > pid.MaxOutput {
		t.Errorf("Integral should be capped at max output limit, got %f", pid.Integral)
	}
}

func TestPIDIntegralSeparation(t *testing.T) {
	pid := NewPIDController(1.0, 1.0, 0, 1.0, 0, 100, 1.0, 0.5)

	prevIntegral := pid.Integral
	pid.Update(1.0, 3.0, time.Second)

	if pid.Integral != prevIntegral {
		t.Errorf("Integral should not accumulate when error exceeds separation threshold, got %f (prev: %f)", pid.Integral, prevIntegral)
	}
}

func TestPIDAntiWindup(t *testing.T) {
	pid := NewPIDController(1.0, 1.0, 0, 1.0, 0, 100, 10.0, 1.0)

	for i := 0; i < 5; i++ {
		pid.Update(1.0, 20.0, time.Second)
	}

	if !pid.OutputSaturated {
		t.Error("Output should be saturated")
	}

	prevIntegral := pid.Integral
	pid.Update(1.0, 20.0, time.Second)

	if pid.Integral > prevIntegral {
		t.Errorf("Integral should decrease during anti-windup, prev: %f, current: %f", prevIntegral, pid.Integral)
	}
}

func TestFeedforwardController(t *testing.T) {
	ff := NewFeedforwardController(10.0, 0.0001, 50, -50, 1.5, 300000)

	output := ff.Calculate(2.0, 300000)
	if output <= 0 {
		t.Error("Expected positive feedforward output for higher NH3")
	}

	output = ff.Calculate(1.0, 300000)
	if output >= 0 {
		t.Error("Expected negative feedforward output for lower NH3")
	}
}

func TestNewAerationController(t *testing.T) {
	validDataChan := make(chan *sc.ValidatedSensorData, 10)
	controlOutputChan := make(chan *ControlOutput, 10)
	statusChan := make(chan map[string]interface{}, 10)

	cfg := Config{
		PID: PIDConfig{
			Kp:                        2.0,
			Ki:                        0.1,
			Kd:                        0.05,
			IntegralSeparationThreshold: 1.0,
			AntiWindupGain:            0.5,
		},
		Feedforward: FeedforwardConfig{
			NH3Gain:  10.0,
			FlowGain: 0.0001,
			NH3Base:  1.5,
			FlowBase: 300000,
		},
		Zones: []ZoneConfig{
			{ZoneID: "aerobic1", DOSetpoint: 2.0, NH3Setpoint: 1.5, DOMin: 1.5, DOMax: 2.5, MinAirFlow: 500, MaxAirFlow: 5000},
		},
		TargetNH3:       1.5,
		ControlInterval: 2 * time.Second,
	}

	ctrl := New(cfg, validDataChan, controlOutputChan, statusChan)
	if ctrl == nil {
		t.Fatal("Expected AerationController to be created")
	}

	zones := ctrl.GetAllZones()
	if len(zones) != 1 {
		t.Errorf("Expected 1 zone, got %d", len(zones))
	}

	zone := zones["aerobic1"]
	if zone == nil {
		t.Fatal("Expected aerobic1 zone to exist")
	}
	if zone.DOSetpoint != 2.0 {
		t.Errorf("Expected DOSetpoint 2.0, got %f", zone.DOSetpoint)
	}
}

func TestAerationZoneUpdate(t *testing.T) {
	validDataChan := make(chan *sc.ValidatedSensorData, 10)
	controlOutputChan := make(chan *ControlOutput, 10)
	statusChan := make(chan map[string]interface{}, 10)

	cfg := Config{
		PID: PIDConfig{
			Kp:                        2.0,
			Ki:                        0.1,
			Kd:                        0.05,
			IntegralSeparationThreshold: 1.0,
			AntiWindupGain:            0.5,
		},
		Feedforward: FeedforwardConfig{
			NH3Gain:  10.0,
			FlowGain: 0.0001,
			NH3Base:  1.5,
			FlowBase: 300000,
		},
		Zones: []ZoneConfig{
			{ZoneID: "aerobic1", DOSetpoint: 2.0, NH3Setpoint: 1.5, DOMin: 1.5, DOMax: 2.5, MinAirFlow: 500, MaxAirFlow: 5000},
		},
		TargetNH3:       1.5,
		ControlInterval: 2 * time.Second,
	}

	ctrl := New(cfg, validDataChan, controlOutputChan, statusChan)
	ctrl.SetFlowRate(300000)

	zone := ctrl.GetAllZones()["aerobic1"]
	zone.DOActual = 1.8
	zone.NH3Actual = 1.5

	ctrl.RunControlCycle()

	select {
	case output := <-controlOutputChan:
		if output.ZoneID != "aerobic1" {
			t.Errorf("Expected zone aerobic1, got %s", output.ZoneID)
		}
		if output.Command == nil {
			t.Error("Expected control command to be non-nil")
		}
		if output.Command.Value <= 0 {
			t.Errorf("Expected positive control value, got %f", output.Command.Value)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected control output on channel, got timeout")
	}
}

func TestSetFlowRate(t *testing.T) {
	validDataChan := make(chan *sc.ValidatedSensorData, 10)
	controlOutputChan := make(chan *ControlOutput, 10)
	statusChan := make(chan map[string]interface{}, 10)

	cfg := Config{
		ControlInterval: 2 * time.Second,
	}
	ctrl := New(cfg, validDataChan, controlOutputChan, statusChan)

	testFlowRate := 350000.0
	ctrl.SetFlowRate(testFlowRate)

	status := ctrl.GetStatus()
	if status["flow_rate"] != testFlowRate {
		t.Errorf("Expected flow_rate %f, got %f", testFlowRate, status["flow_rate"])
	}
}

func TestCalculateEnergyPerTon(t *testing.T) {
	validDataChan := make(chan *sc.ValidatedSensorData, 10)
	controlOutputChan := make(chan *ControlOutput, 10)
	statusChan := make(chan map[string]interface{}, 10)

	cfg := Config{
		Zones: []ZoneConfig{
			{ZoneID: "aerobic1", DOSetpoint: 2.0, NH3Setpoint: 1.5, DOMin: 1.5, DOMax: 2.5, MinAirFlow: 500, MaxAirFlow: 5000},
			{ZoneID: "aerobic2", DOSetpoint: 2.0, NH3Setpoint: 1.5, DOMin: 1.5, DOMax: 2.5, MinAirFlow: 500, MaxAirFlow: 5000},
		},
		ControlInterval: 2 * time.Second,
	}
	ctrl := New(cfg, validDataChan, controlOutputChan, statusChan)

	energy := ctrl.CalculateEnergyPerTon(300000)
	if energy <= 0 {
		t.Error("Expected positive energy per ton")
	}
	if energy > 1.0 {
		t.Errorf("Expected reasonable energy value (< 1 kWh/ton), got %f", energy)
	}
}
