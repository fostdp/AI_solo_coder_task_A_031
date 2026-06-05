package sensor_collector

import (
	"log"
	"math"
	"sync"
	"time"

	"sewage-plant-system/pkg/models"
)

type ValidatedSensorData struct {
	*models.SensorData
	IsValid   bool
	Reason    string
	Processed time.Time
}

type SensorStatusEvent struct {
	SensorID   string
	EventType  string
	Timestamp  time.Time
	Value      float64
	Location   string
	SensorType models.SensorType
}

type Config struct {
	OfflineTimeout    time.Duration
	MaxDeviationRatio float64
	SensorConfigs     []*models.SensorConfig
}

type SensorCollector struct {
	cfg            Config
	sensorStatus   map[string]*SensorStatus
	mu             sync.RWMutex
	ValidDataChan  chan<- *ValidatedSensorData
	StatusEventChan chan<- *SensorStatusEvent
}

type SensorStatus struct {
	SensorID    string
	LastValue   float64
	LastUpdate  time.Time
	IsOnline    bool
	SensorType  models.SensorType
	Location    string
	Setpoint    float64
}

func New(cfg Config, validDataChan chan<- *ValidatedSensorData, statusEventChan chan<- *SensorStatusEvent) *SensorCollector {
	sc := &SensorCollector{
		cfg:             cfg,
		sensorStatus:    make(map[string]*SensorStatus),
		ValidDataChan:   validDataChan,
		StatusEventChan: statusEventChan,
	}

	for _, scfg := range cfg.SensorConfigs {
		sc.sensorStatus[scfg.SensorID] = &SensorStatus{
			SensorID:   scfg.SensorID,
			SensorType: scfg.Type,
			Location:   scfg.Location,
			Setpoint:   scfg.Setpoint,
			IsOnline:   true,
		}
	}

	return sc
}

func (sc *SensorCollector) ProcessSensorData(data *models.SensorData) {
	validated := &ValidatedSensorData{
		SensorData: data,
		Processed:  time.Now(),
	}

	validated.IsValid, validated.Reason = sc.validateData(data)

	sc.mu.Lock()
	status, exists := sc.sensorStatus[data.SensorID]
	if !exists {
		status = &SensorStatus{
			SensorID:   data.SensorID,
			SensorType: data.Type,
			Location:   data.Location,
			Setpoint:   getDefaultSetpoint(data.Type),
			IsOnline:   true,
		}
		sc.sensorStatus[data.SensorID] = status
	}

	wasOffline := !status.IsOnline
	status.LastValue = data.Value
	status.LastUpdate = data.Timestamp
	status.IsOnline = true
	sc.mu.Unlock()

	if wasOffline && validated.IsValid {
		sc.emitStatusEvent(data.SensorID, "online", data.Value, data.Type, data.Location)
	}

	if validated.IsValid {
		select {
		case sc.ValidDataChan <- validated:
		default:
			log.Printf("[SENSOR_COLLECTOR] ValidDataChan full, dropping data for %s", data.SensorID)
		}
	} else {
		log.Printf("[SENSOR_COLLECTOR] Invalid data from %s: %s", data.SensorID, validated.Reason)
		sc.emitStatusEvent(data.SensorID, "invalid_data", data.Value, data.Type, data.Location)
	}
}

func (sc *SensorCollector) validateData(data *models.SensorData) (bool, string) {
	if data == nil {
		return false, "nil data"
	}

	if math.IsNaN(data.Value) || math.IsInf(data.Value, 0) {
		return false, "NaN or Inf value"
	}

	if data.Value < 0 {
		return false, "negative value"
	}

	if data.SensorID == "" {
		return false, "empty sensor ID"
	}

	if data.Timestamp.IsZero() {
		return false, "zero timestamp"
	}

	sc.mu.RLock()
	status, exists := sc.sensorStatus[data.SensorID]
	sc.mu.RUnlock()

	if exists && status.Setpoint > 0 {
		if data.Value > status.Setpoint*sc.cfg.MaxDeviationRatio {
			return false, "value exceeds maximum deviation ratio"
		}
	}

	return true, ""
}

func (sc *SensorCollector) CheckOffline(now time.Time) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	for sensorID, status := range sc.sensorStatus {
		if status.IsOnline && now.Sub(status.LastUpdate) > sc.cfg.OfflineTimeout {
			status.IsOnline = false
			log.Printf("[SENSOR_COLLECTOR] Sensor %s is offline (timeout: %v)", sensorID, sc.cfg.OfflineTimeout)
			sc.emitStatusEvent(sensorID, "offline", status.LastValue, status.SensorType, status.Location)
		}
	}
}

func (sc *SensorCollector) emitStatusEvent(sensorID, eventType string, value float64, sensorType models.SensorType, location string) {
	event := &SensorStatusEvent{
		SensorID:   sensorID,
		EventType:  eventType,
		Timestamp:  time.Now(),
		Value:      value,
		SensorType: sensorType,
		Location:   location,
	}

	select {
	case sc.StatusEventChan <- event:
	default:
		log.Printf("[SENSOR_COLLECTOR] StatusEventChan full, dropping event for %s", sensorID)
	}
}

func (sc *SensorCollector) GetSensorStatus(sensorID string) (*SensorStatus, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	status, exists := sc.sensorStatus[sensorID]
	return status, exists
}

func (sc *SensorCollector) GetAllStatus() map[string]*SensorStatus {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	result := make(map[string]*SensorStatus)
	for k, v := range sc.sensorStatus {
		result[k] = v
	}
	return result
}

func (sc *SensorCollector) SensorBelongsToLocation(sensorID, location string) bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	status, exists := sc.sensorStatus[sensorID]
	if !exists {
		return false
	}
	return status.Location == location
}

func getDefaultSetpoint(sensorType models.SensorType) float64 {
	switch sensorType {
	case models.SensorTypeDO:
		return 2.0
	case models.SensorTypeNH3:
		return 1.5
	case models.SensorTypeNO3:
		return 10.0
	case models.SensorTypePO4:
		return 0.5
	case models.SensorTypeCOD:
		return 300.0
	default:
		return 0
	}
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
