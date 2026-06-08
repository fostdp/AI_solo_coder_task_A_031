package messages

import (
	"time"

	"sewage-treatment-system/internal/models"
)

type SensorDataMessage struct {
	Data      *models.SensorData
	Valid     bool
	Error     string
	Timestamp time.Time
}

type AerationControlMessage struct {
	Section       int
	AirFlowSet    float64
	ValveOpen     float64
	DOActual      float64
	NH3Actual     float64
	Timestamp     time.Time
	TriggerSource string
}

type CarbonDosingMessage struct {
	DosingRate   float64
	CODInfluent  float64
	NO3Anoxic    float64
	TNRemoval    float64
	Timestamp    time.Time
	TriggerType  string
}

type AlarmMessage struct {
	Level        int
	Type         string
	Message      string
	Value        float64
	Threshold    float64
	SourceModule string
	Timestamp    time.Time
}

type ControlChannels struct {
	SensorDataIn  <-chan *SensorDataMessage
	AerationOut   chan<- *AerationControlMessage
	CarbonOut     chan<- *CarbonDosingMessage
	AlarmOut      chan<- *AlarmMessage
}

type CollectorChannels struct {
	ValidatedData chan<- *SensorDataMessage
	AlarmOut      chan<- *AlarmMessage
}
