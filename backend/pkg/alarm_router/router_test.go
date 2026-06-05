package alarm_router

import (
	"testing"
	"time"

	"sewage-plant-system/pkg/models"
	sc "sewage-plant-system/pkg/sensor_collector"
	ws "sewage-plant-system/pkg/websocket"
)

func TestNewAlarmRouter(t *testing.T) {
	wsServer := ws.New()
	validDataChan := make(chan *sc.ValidatedSensorData, 10)
	statusEventChan := make(chan *sc.SensorStatusEvent, 10)
	plcStatusChan := make(chan *models.PLCStatus, 10)
	alertOutChan := make(chan *models.Alert, 10)
	statusOutChan := make(chan map[string]interface{}, 10)

	cfg := Config{
		Thresholds: ThresholdConfig{
			NHEffluentLimit:  5.0,
			TNEffluentLimit:  15.0,
			Duration:         30 * time.Minute,
			OfflineTimeout:   5 * time.Minute,
		},
		SMSFallbackEnabled: true,
		SMSFallbackLevel:   1,
		BufferMaxSize:      10,
		CheckInterval:      30 * time.Second,
	}

	router := New(cfg, wsServer, validDataChan, statusEventChan, plcStatusChan, alertOutChan, statusOutChan)
	if router == nil {
		t.Fatal("Expected AlarmRouter to be created")
	}

	router.SetWSConnected(true)
	router.SetSMSAvailable(true)

	status := router.GetStatus()
	if status == nil {
		t.Fatal("Expected status to be non-nil")
	}
	if !status["ws_connected"].(bool) {
		t.Error("Expected ws_connected to be true")
	}
	if !status["sms_available"].(bool) {
		t.Error("Expected sms_available to be true")
	}
}

func TestRouteAlert(t *testing.T) {
	wsServer := ws.New()
	validDataChan := make(chan *sc.ValidatedSensorData, 10)
	statusEventChan := make(chan *sc.SensorStatusEvent, 10)
	plcStatusChan := make(chan *models.PLCStatus, 10)
	alertOutChan := make(chan *models.Alert, 10)
	statusOutChan := make(chan map[string]interface{}, 10)

	cfg := Config{
		Thresholds: ThresholdConfig{
			NHEffluentLimit: 5.0,
			TNEffluentLimit: 15.0,
		},
		SMSFallbackEnabled: true,
		SMSFallbackLevel:   1,
		BufferMaxSize:      10,
		CheckInterval:      30 * time.Second,
	}

	router := New(cfg, wsServer, validDataChan, statusEventChan, plcStatusChan, alertOutChan, statusOutChan)
	router.SetWSConnected(true)
	router.SetSMSAvailable(true)

	alert := &models.Alert{
		AlertID:   "test-1",
		Level:     1,
		Message:   "Test alert",
		Timestamp: time.Now(),
	}

	router.RouteAlert(alert)

	router.mu.RLock()
	if router.wsDelivered != 1 {
		t.Errorf("Expected 1 WS delivery, got %d", router.wsDelivered)
	}
	if len(router.alertBuffer) != 0 {
		t.Errorf("Expected buffer to be empty, got %d alerts", len(router.alertBuffer))
	}
	router.mu.RUnlock()
}

func TestWSFailoverToSMS(t *testing.T) {
	wsServer := ws.New()
	validDataChan := make(chan *sc.ValidatedSensorData, 10)
	statusEventChan := make(chan *sc.SensorStatusEvent, 10)
	plcStatusChan := make(chan *models.PLCStatus, 10)
	alertOutChan := make(chan *models.Alert, 10)
	statusOutChan := make(chan map[string]interface{}, 10)

	cfg := Config{
		Thresholds: ThresholdConfig{
			NHEffluentLimit: 5.0,
			TNEffluentLimit: 15.0,
		},
		SMSFallbackEnabled: true,
		SMSFallbackLevel:   1,
		BufferMaxSize:      10,
		CheckInterval:      30 * time.Second,
	}

	router := New(cfg, wsServer, validDataChan, statusEventChan, plcStatusChan, alertOutChan, statusOutChan)
	router.SetWSConnected(false)
	router.SetSMSAvailable(true)

	alert := &models.Alert{
		AlertID:   "test-2",
		Level:     1,
		Message:   "Test alert for failover",
		Timestamp: time.Now(),
	}

	router.RouteAlert(alert)

	router.mu.RLock()
	if router.smsDelivered != 1 {
		t.Errorf("Expected 1 SMS delivery, got %d", router.smsDelivered)
	}
	router.mu.RUnlock()
}

func TestBufferAndFlush(t *testing.T) {
	wsServer := ws.New()
	validDataChan := make(chan *sc.ValidatedSensorData, 10)
	statusEventChan := make(chan *sc.SensorStatusEvent, 10)
	plcStatusChan := make(chan *models.PLCStatus, 10)
	alertOutChan := make(chan *models.Alert, 10)
	statusOutChan := make(chan map[string]interface{}, 10)

	cfg := Config{
		Thresholds: ThresholdConfig{
			NHEffluentLimit: 5.0,
			TNEffluentLimit: 15.0,
		},
		SMSFallbackEnabled: false,
		SMSFallbackLevel:   1,
		BufferMaxSize:      10,
		CheckInterval:      30 * time.Second,
	}

	router := New(cfg, wsServer, validDataChan, statusEventChan, plcStatusChan, alertOutChan, statusOutChan)
	router.SetWSConnected(false)
	router.SetSMSAvailable(false)

	for i := 0; i < 5; i++ {
		alert := &models.Alert{
			AlertID:   string(rune('a' + i)),
			Level:     1,
			Message:   "Buffered alert",
			Timestamp: time.Now(),
		}
		router.RouteAlert(alert)
	}

	router.mu.RLock()
	if len(router.alertBuffer) != 5 {
		t.Errorf("Expected 5 buffered alerts, got %d", len(router.alertBuffer))
	}
	router.mu.RUnlock()

	router.SetWSConnected(true)
	router.flushBufferedAlerts()

	router.mu.RLock()
	if len(router.alertBuffer) != 0 {
		t.Errorf("Expected buffer to be empty after flush, got %d", len(router.alertBuffer))
	}
	if router.wsDelivered != 5 {
		t.Errorf("Expected 5 WS deliveries after flush, got %d", router.wsDelivered)
	}
	router.mu.RUnlock()
}

func TestUpdateFanStatus(t *testing.T) {
	wsServer := ws.New()
	validDataChan := make(chan *sc.ValidatedSensorData, 10)
	statusEventChan := make(chan *sc.SensorStatusEvent, 10)
	plcStatusChan := make(chan *models.PLCStatus, 10)
	alertOutChan := make(chan *models.Alert, 10)
	statusOutChan := make(chan map[string]interface{}, 10)

	cfg := Config{
		Thresholds: ThresholdConfig{
			NHEffluentLimit: 5.0,
			TNEffluentLimit: 15.0,
			OfflineTimeout: 100 * time.Millisecond,
		},
		BufferMaxSize: 10,
		CheckInterval: 30 * time.Second,
	}

	router := New(cfg, wsServer, validDataChan, statusEventChan, plcStatusChan, alertOutChan, statusOutChan)

	router.UpdateFanStatus("fan-1", true, time.Now())
	router.UpdateFanStatus("fan-2", false, time.Now().Add(-10*time.Minute))

	router.mu.RLock()
	if router.fanStatuses["fan-1"].Status != "running" {
		t.Error("Expected fan-1 to be running")
	}
	if router.fanStatuses["fan-2"].Status != "failed" {
		t.Errorf("Expected fan-2 to be failed, got %s", router.fanStatuses["fan-2"].Status)
	}
	router.mu.RUnlock()

	select {
	case alert := <-alertOutChan:
		if alert.Level != 2 {
			t.Errorf("Expected level 2 alert for fan failure, got %d", alert.Level)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected fan failure alert on alertOutChan")
	}
}
