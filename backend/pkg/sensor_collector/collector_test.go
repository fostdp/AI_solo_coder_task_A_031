package sensor_collector

import (
	"testing"
	"time"

	"sewage-plant-system/pkg/models"
)

func TestProcessSensorData(t *testing.T) {
	validDataChan := make(chan *ValidatedSensorData, 10)
	statusEventChan := make(chan *SensorStatusEvent, 10)

	cfg := Config{
		OfflineTimeout:    5 * time.Minute,
		MaxDeviationRatio: 5.0,
	}
	collector := New(cfg, validDataChan, statusEventChan)

	data := &models.SensorData{
		SensorID:  "DO-001",
		Type:      models.SensorTypeDO,
		Value:     2.5,
		Timestamp: time.Now(),
		Location:  "aerobic1",
	}

	collector.ProcessSensorData(data)

	select {
	case validated := <-validDataChan:
		if !validated.IsValid {
			t.Error("Expected data to be valid")
		}
		if validated.Value != 2.5 {
			t.Errorf("Expected value 2.5, got %f", validated.Value)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected data on validDataChan, got timeout")
	}
}

func TestValidateData(t *testing.T) {
	validDataChan := make(chan *ValidatedSensorData, 10)
	statusEventChan := make(chan *SensorStatusEvent, 10)

	cfg := Config{
		OfflineTimeout:    5 * time.Minute,
		MaxDeviationRatio: 5.0,
		SensorConfigs: []*models.SensorConfig{
			{
				SensorID: "DO-001",
				Type:     models.SensorTypeDO,
				Setpoint: 2.0,
			},
		},
	}
	collector := New(cfg, validDataChan, statusEventChan)

	tests := []struct {
		name       string
		sensorID   string
		sensorType models.SensorType
		value      float64
		expected   bool
	}{
		{"Valid DO value", "DO-001", models.SensorTypeDO, 2.5, true},
		{"DO value exceeds deviation ratio", "DO-001", models.SensorTypeDO, 15.0, false},
		{"DO value negative", "DO-002", models.SensorTypeDO, -1.0, false},
		{"NaN value", "DO-003", models.SensorTypeDO, 0.0 / 0.0, false},
		{"Empty sensor ID", "", models.SensorTypeDO, 2.5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &models.SensorData{
				SensorID:  tt.sensorID,
				Type:      tt.sensorType,
				Value:     tt.value,
				Timestamp: time.Now(),
			}
			valid, _ := collector.validateData(data)
			if valid != tt.expected {
				t.Errorf("Expected %v, got %v for value %f", tt.expected, valid, tt.value)
			}
		})
	}
}

func TestCheckOffline(t *testing.T) {
	validDataChan := make(chan *ValidatedSensorData, 10)
	statusEventChan := make(chan *SensorStatusEvent, 10)

	cfg := Config{
		OfflineTimeout:    100 * time.Millisecond,
		MaxDeviationRatio: 5.0,
		SensorConfigs: []*models.SensorConfig{
			{
				SensorID: "DO-001",
				Type:     models.SensorTypeDO,
				Setpoint: 2.0,
				Location: "aerobic1",
			},
			{
				SensorID: "DO-002",
				Type:     models.SensorTypeDO,
				Setpoint: 2.0,
				Location: "aerobic1",
			},
		},
	}
	collector := New(cfg, validDataChan, statusEventChan)

	collector.ProcessSensorData(&models.SensorData{
		SensorID:  "DO-001",
		Type:      models.SensorTypeDO,
		Value:     2.5,
		Timestamp: time.Now(),
		Location:  "aerobic1",
	})

	collector.ProcessSensorData(&models.SensorData{
		SensorID:  "DO-002",
		Type:      models.SensorTypeDO,
		Value:     2.0,
		Timestamp: time.Now().Add(-200 * time.Millisecond),
		Location:  "aerobic1",
	})

	<-validDataChan
	<-validDataChan

	collector.CheckOffline(time.Now())

	select {
	case event := <-statusEventChan:
		if event.SensorID != "DO-002" {
			t.Errorf("Expected DO-002, got %s", event.SensorID)
		}
		if event.EventType != "offline" {
			t.Errorf("Expected offline event, got %s", event.EventType)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected offline event on statusEventChan")
	}
}

func TestGetSensorStatus(t *testing.T) {
	validDataChan := make(chan *ValidatedSensorData, 10)
	statusEventChan := make(chan *SensorStatusEvent, 10)

	cfg := Config{
		OfflineTimeout:    5 * time.Minute,
		MaxDeviationRatio: 5.0,
		SensorConfigs: []*models.SensorConfig{
			{
				SensorID: "DO-001",
				Type:     models.SensorTypeDO,
				Setpoint: 2.0,
				Location: "aerobic1",
			},
		},
	}
	collector := New(cfg, validDataChan, statusEventChan)

	collector.ProcessSensorData(&models.SensorData{
		SensorID:  "DO-001",
		Type:      models.SensorTypeDO,
		Value:     2.5,
		Timestamp: time.Now(),
		Location:  "aerobic1",
	})

	<-validDataChan

	status, exists := collector.GetSensorStatus("DO-001")
	if !exists {
		t.Fatal("Expected sensor status to exist")
	}
	if status.LastValue != 2.5 {
		t.Errorf("Expected LastValue 2.5, got %f", status.LastValue)
	}
	if !status.IsOnline {
		t.Error("Expected sensor to be online")
	}
}
