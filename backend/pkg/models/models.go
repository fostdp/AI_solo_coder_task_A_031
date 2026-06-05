package models

import "time"

type SensorType string

const (
	SensorTypeDO   SensorType = "DO"
	SensorTypeNH3  SensorType = "NH3"
	SensorTypeNO3  SensorType = "NO3"
	SensorTypePO4  SensorType = "PO4"
	SensorTypeCOD  SensorType = "COD"
	SensorTypeFlow SensorType = "FLOW"
)

type SensorData struct {
	SensorID  string     `json:"sensor_id"`
	Type      SensorType `json:"type"`
	Value     float64    `json:"value"`
	Unit      string     `json:"unit"`
	Location  string     `json:"location"`
	Timestamp time.Time  `json:"timestamp"`
	Status    string     `json:"status"`
}

type SensorConfig struct {
	SensorID    string     `json:"sensor_id"`
	Type        SensorType `json:"type"`
	Location    string     `json:"location"`
	Setpoint    float64    `json:"setpoint"`
	MinValue    float64    `json:"min_value"`
	MaxValue    float64    `json:"max_value"`
	X           float64    `json:"x"`
	Y           float64    `json:"y"`
	Description string     `json:"description"`
}

type ControlCommand struct {
	CommandID   string    `json:"command_id"`
	TargetType  string    `json:"target_type"`
	TargetID    string    `json:"target_id"`
	Action      string    `json:"action"`
	Value       float64   `json:"value"`
	Unit        string    `json:"unit"`
	Timestamp   time.Time `json:"timestamp"`
	Source      string    `json:"source"`
}

type PLCStatus struct {
	PLCID      string    `json:"plc_id"`
	DeviceType string    `json:"device_type"`
	DeviceID   string    `json:"device_id"`
	Status     string    `json:"status"`
	Value      float64   `json:"value"`
	FaultCode  string    `json:"fault_code"`
	Timestamp  time.Time `json:"timestamp"`
}

type Alert struct {
	AlertID          string    `json:"alert_id"`
	Level            int       `json:"level"`
	Type             string    `json:"type"`
	Title            string    `json:"title"`
	Message          string    `json:"message"`
	SensorID         string    `json:"sensor_id"`
	Value            float64   `json:"value"`
	Threshold        float64   `json:"threshold"`
	Timestamp        time.Time `json:"timestamp"`
	Acknowledged     bool      `json:"acknowledged"`
	Delivered        bool      `json:"delivered"`
	DeliveryChannels []string  `json:"delivery_channels"`
}

type AerationControl struct {
	ZoneID         string  `json:"zone_id"`
	DOActual       float64 `json:"do_actual"`
	DOSetpoint     float64 `json:"do_setpoint"`
	NH3Actual      float64 `json:"nh3_actual"`
	NH3Setpoint    float64 `json:"nh3_setpoint"`
	AirFlowSetpoint float64 `json:"air_flow_setpoint"`
	ValveOpening   float64 `json:"valve_opening"`
	FanSpeed       float64 `json:"fan_speed"`
	PIDOutput      float64 `json:"pid_output"`
	FeedforwardOutput float64 `json:"feedforward_output"`
	TotalOutput    float64 `json:"total_output"`
}

type CarbonControl struct {
	NO3Actual       float64 `json:"no3_actual"`
	CODInfluent     float64 `json:"cod_influent"`
	TNEstimate      float64 `json:"tn_estimate"`
	DosageSetpoint  float64 `json:"dosage_setpoint"`
	CarbonSourceType string `json:"carbon_source_type"`
	RemovalRate     float64 `json:"removal_rate"`
}

type KPIData struct {
	Timestamp       time.Time `json:"timestamp"`
	EnergyPerTon    float64   `json:"energy_per_ton"`
	CarbonPerTon    float64   `json:"carbon_per_ton"`
	NH3RemovalRate  float64   `json:"nh3_removal_rate"`
	TNRemovalRate   float64   `json:"tn_removal_rate"`
	TPRemovalRate   float64   `json:"tp_removal_rate"`
	WaterQuality    float64   `json:"water_quality"`
}

type TrendData struct {
	Timestamps []time.Time `json:"timestamps"`
	Values     []float64   `json:"values"`
	Labels     []string    `json:"labels"`
}

type ProcessSection struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Width       float64 `json:"width"`
	Height      float64 `json:"height"`
	Status      string  `json:"status"`
	Description string  `json:"description"`
}
