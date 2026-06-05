package carbon_optimizer

import (
	"testing"
	"time"

	"sewage-plant-system/pkg/models"
	sc "sewage-plant-system/pkg/sensor_collector"
)

func TestCalculateOptimalDosage(t *testing.T) {
	cfg := CarbonConfig{
		NO3Setpoint:              10.0,
		TNSetpoint:               15.0,
		MaxDosage:                100.0,
		MinDosage:                10.0,
		CODToNRatio:              4.5,
		CODChangeThreshold:       0.2,
		EventDrivenEnabled:       true,
		TargetTNRemovalRate:      75.0,
		ControlInterval:          30 * time.Minute,
	}

	optimizer := NewCarbonOptimizerCore(cfg)

	dosage, removalRate := optimizer.CalculateOptimalDosage(300, 25, 30)
	if dosage < cfg.MinDosage {
		t.Errorf("Expected dosage >= %f, got %f", cfg.MinDosage, dosage)
	}
	if dosage > cfg.MaxDosage {
		t.Errorf("Expected dosage <= %f, got %f", cfg.MaxDosage, dosage)
	}
	if removalRate <= 0 || removalRate > 100 {
		t.Errorf("Expected removal rate between 0 and 100, got %f", removalRate)
	}
}

func TestEventDrivenOptimization(t *testing.T) {
	validDataChan := make(chan *sc.ValidatedSensorData, 10)
	statusEventChan := make(chan *sc.SensorStatusEvent, 10)
	controlOutputChan := make(chan *ControlOutput, 10)
	statusChan := make(chan *Status, 10)

	cfg := CarbonConfig{
		NO3Setpoint:              10.0,
		TNSetpoint:               15.0,
		MaxDosage:                100.0,
		MinDosage:                10.0,
		CODToNRatio:              4.5,
		CODChangeThreshold:       0.2,
		EventDrivenEnabled:       true,
		TargetTNRemovalRate:      75.0,
		ControlInterval:          30 * time.Minute,
	}

	opt := New(cfg, validDataChan, statusEventChan, controlOutputChan, statusChan)
	controlSys := opt.GetControlSystem()

	controlSys.SetCOD(200)
	controlSys.SetLastOptimization(time.Now())

	opt.checkAndTriggerEventDriven(250)

	select {
	case output := <-controlOutputChan:
		if !output.IsEventDriven {
			t.Error("Expected event-driven control output")
		}
		if output.Command == nil {
			t.Error("Expected control command to be non-nil")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected event-driven control output, got timeout")
	}

	controlSys.SetCOD(250)
	opt.checkAndTriggerEventDriven(260)

	select {
	case <-controlOutputChan:
		t.Error("Should not trigger for small COD change")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestNewCarbonOptimizer(t *testing.T) {
	validDataChan := make(chan *sc.ValidatedSensorData, 10)
	statusEventChan := make(chan *sc.SensorStatusEvent, 10)
	controlOutputChan := make(chan *ControlOutput, 10)
	statusChan := make(chan *Status, 10)

	cfg := CarbonConfig{
		NO3Setpoint:              10.0,
		TNSetpoint:               15.0,
		MaxDosage:                100.0,
		MinDosage:                10.0,
		CODToNRatio:              4.5,
		CODChangeThreshold:       0.2,
		EventDrivenEnabled:       true,
		TargetTNRemovalRate:      75.0,
		ControlInterval:          30 * time.Minute,
	}

	opt := New(cfg, validDataChan, statusEventChan, controlOutputChan, statusChan)
	if opt == nil {
		t.Fatal("Expected CarbonOptimizer to be created")
	}

	status := opt.GetStatus()
	if status == nil {
		t.Fatal("Expected status to be non-nil")
	}
	if status.Config.TargetTNRemovalRate != 75.0 {
		t.Errorf("Expected TargetTNRemovalRate 75.0, got %f", status.Config.TargetTNRemovalRate)
	}
}

func TestCalculateCarbonPerTon(t *testing.T) {
	validDataChan := make(chan *sc.ValidatedSensorData, 10)
	statusEventChan := make(chan *sc.SensorStatusEvent, 10)
	controlOutputChan := make(chan *ControlOutput, 10)
	statusChan := make(chan *Status, 10)

	cfg := CarbonConfig{
		NO3Setpoint:              10.0,
		TNSetpoint:               15.0,
		MaxDosage:                100.0,
		MinDosage:                10.0,
		MaxCarbonPerTon:          0.5,
		ControlInterval:          30 * time.Minute,
	}

	opt := New(cfg, validDataChan, statusEventChan, controlOutputChan, statusChan)
	controlSys := opt.GetControlSystem()
	controlSys.SetDosageSetpoint(50)

	carbonPerTon := opt.CalculateCarbonPerTon(300000)
	if carbonPerTon <= 0 {
		t.Error("Expected positive carbon per ton")
	}
	if carbonPerTon > cfg.MaxCarbonPerTon {
		t.Errorf("Expected carbon per ton <= %f, got %f", cfg.MaxCarbonPerTon, carbonPerTon)
	}
}
