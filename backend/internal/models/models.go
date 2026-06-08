package models

import "time"

type SensorType string

const (
	SensorDO    SensorType = "DO"
	SensorNH3   SensorType = "NH3"
	SensorNO3   SensorType = "NO3"
	SensorPO4   SensorType = "PO4"
	SensorCOD   SensorType = "COD"
	SensorTN    SensorType = "TN"
	SensorTP    SensorType = "TP"
	SensorFlow  SensorType = "FLOW"
	SensorLevel SensorType = "LEVEL"
)

type ProcessStage string

const (
	StageCoarseGrate   ProcessStage = "coarse_grate"
	StageFineGrate     ProcessStage = "fine_grate"
	StageGritChamber   ProcessStage = "grit_chamber"
	StagePrimarySett   ProcessStage = "primary_settling"
	StageAnaerobic     ProcessStage = "anaerobic"
	StageAnoxic        ProcessStage = "anoxic"
	StageAerobic       ProcessStage = "aerobic"
	StageSecondarySett ProcessStage = "secondary_settling"
	StageAdvanced      ProcessStage = "advanced_treatment"
	StageEffluent      ProcessStage = "effluent"
)

type SensorData struct {
	ID          string      `json:"id"`
	Type        SensorType  `json:"type"`
	Stage       ProcessStage `json:"stage"`
	Section     int         `json:"section"`
	Value       float64     `json:"value"`
	Setpoint    float64     `json:"setpoint"`
	Timestamp   time.Time   `json:"timestamp"`
	DTUID       string      `json:"dtu_id"`
	Status      string      `json:"status"`
}

type SensorInfo struct {
	ID         string       `json:"id"`
	Type       SensorType   `json:"type"`
	Stage      ProcessStage `json:"stage"`
	Section    int          `json:"section"`
	X          float64      `json:"x"`
	Y          float64      `json:"y"`
	Setpoint   float64      `json:"setpoint"`
	MinDeviation float64    `json:"min_deviation"`
	MaxDeviation float64    `json:"max_deviation"`
}

type AerationControl struct {
	Section       int       `json:"section"`
	AirFlowSet    float64   `json:"air_flow_set"`
	AirFlowActual float64   `json:"air_flow_actual"`
	ValveOpen     float64   `json:"valve_open"`
	DOActual      float64   `json:"do_actual"`
	NH3Actual     float64   `json:"nh3_actual"`
	Timestamp     time.Time `json:"timestamp"`
}

type CarbonDosing struct {
	DosingRate   float64   `json:"dosing_rate"`
	DosingActual float64   `json:"dosing_actual"`
	CODInfluent  float64   `json:"cod_influent"`
	NO3Anoxic    float64   `json:"no3_anoxic"`
	TNRemoval    float64   `json:"tn_removal"`
	Timestamp    time.Time `json:"timestamp"`
}

type Alarm struct {
	ID        string    `json:"id"`
	Level     int       `json:"level"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	Timestamp time.Time `json:"timestamp"`
	ACK       bool      `json:"ack"`
}

type KeyMetrics struct {
	PowerConsumption float64 `json:"power_consumption"`
	CarbonUsage      float64 `json:"carbon_usage"`
	TNRemovalRate    float64 `json:"tn_removal_rate"`
	TPRemovalRate    float64 `json:"tp_removal_rate"`
	CODRemovalRate   float64 `json:"cod_removal_rate"`
	FlowRate         float64 `json:"flow_rate"`
	Timestamp        time.Time `json:"timestamp"`
}

type TrendDataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

type SensorTrendResponse struct {
	SensorID string            `json:"sensor_id"`
	Type     SensorType        `json:"type"`
	Data     []TrendDataPoint  `json:"data"`
}

type ControlCommand struct {
	DeviceType string      `json:"device_type"`
	DeviceID   string      `json:"device_id"`
	Command    string      `json:"command"`
	Params     interface{} `json:"params"`
	Timestamp  time.Time   `json:"timestamp"`
}
