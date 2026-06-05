package models

import (
	"fmt"
	"time"
)

type SensorType string

const (
	SensorTypeDO       SensorType = "DO"
	SensorTypeNH3     SensorType = "NH3"
	SensorTypeNO3     SensorType = "NO3"
	SensorTypePO4     SensorType = "PO4"
	SensorTypeCOD     SensorType = "COD"
	SensorTypeTN      SensorType = "TN"
	SensorTypeTP      SensorType = "TP"
	SensorTypeMLSS    SensorType = "MLSS"
)

type ProcessStage string

const (
	StageCoarseScreen   ProcessStage = "coarse_screen"
	StageFineScreen    ProcessStage = "fine_screen"
	StageGritChamber  ProcessStage = "grit_chamber"
	StagePrimary      ProcessStage = "primary_settling"
	StageAnaerobic    ProcessStage = "anaerobic"
	StageAnoxic       ProcessStage = "anoxic"
	StageAerobic       ProcessStage = "aerobic"
	StageSecondary    ProcessStage = "secondary_settling"
	StageAdvanced     ProcessStage = "advanced_treatment"
	StageEffluent     ProcessStage = "effluent"
)

type SensorData struct {
	ID         string       `json:"id"`
	Type       SensorType   `json:"type"`
	Stage      ProcessStage `json:"stage"`
	Section    int          `json:"section"`
	Value      float64      `json:"value"`
	Unit       string       `json:"unit"`
	Timestamp  time.Time    `json:"timestamp"`
	Status     string       `json:"status"`
	AlarmLevel int          `json:"alarm_level"`
}

type SensorConfig struct {
	ID          string       `json:"id"`
	Type        SensorType   `json:"type"`
	Stage       ProcessStage `json:"stage"`
	Section     int          `json:"section"`
	X           float64      `json:"x"`
	Y           float64      `json:"y"`
	TargetMin  float64      `json:"target_min"`
	TargetMax  float64      `json:"target_max"`
	WarningLow  float64      `json:"warning_low"`
	WarningHigh float64      `json:"warning_high"`
	AlarmLow    float64      `json:"alarm_low"`
	AlarmHigh   float64      `json:"alarm_high"`
	Unit        string       `json:"unit"`
	Name        string       `json:"name"`
}

type ControlCommand struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Target     string    `json:"target"`
	Value      float64   `json:"value"`
	Unit       string    `json:"unit"`
	Timestamp  time.Time `json:"timestamp"`
	Source     string    `json:"source"`
}

type AerationControl struct {
	Section       int       `json:"section"`
	AerationRate float64   `json:"aeration_rate"`
	DO            float64   `json:"do"`
	NH3           float64   `json:"nh3"`
	Timestamp     time.Time `json:"timestamp"`
}

type CarbonControl struct {
	DosageRate  float64   `json:"dosage_rate"`
	NO3In      float64   `json:"no3"`
	CODIn       float64   `json:"cod_in"`
	TNRemoval  float64   `json:"tn_removal"`
	Timestamp   time.Time `json:"timestamp"`
}

type Alarm struct {
	ID          string    `json:"id"`
	Level       int       `json:"level"`
	Type        string    `json:"type"`
	Message     string    `json:"message"`
	SensorID    string    `json:"sensor_id"`
	Value       float64   `json:"value"`
	Threshold   float64   `json:"threshold"`
	Timestamp   time.Time `json:"timestamp"`
	AckStatus  bool      `json:"acknowledged"`
}

type KPIData struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Value      float64   `json:"value"`
	Unit       string    `json:"unit"`
	Timestamp  time.Time `json:"timestamp"`
}

type TrendPoint struct {
	Time  time.Time `json:"time"`
	Value float64   `json:"value"`
}

type SensorStatus struct {
	ID         string  `json:"id"`
	Type       string  `json:"type"`
	Stage      string  `json:"stage"`
	Value      float64 `json:"value"`
	Deviation  float64 `json:"deviation"`
	Color      string  `json:"color"`
	LastUpdate time.Time `json:"last_update"`
	Online     bool    `json:"online"`
}

type ProcessStatus struct {
	Stage      string       `json:"stage"`
	Sensors    []SensorStatus `json:"sensors"`
	Status     string       `json:"status"`
}

var SensorConfigs []SensorConfig

func InitSensorConfigs() {
	SensorConfigs = make([]SensorConfig, 0)
	for i := 1; i <= 30; i++ {
		section := ((i - 1) / 10) + 1
		idx := ((i - 1) % 10) + 1
		SensorConfigs = append(SensorConfigs, SensorConfig{
			ID:          fmt.Sprintf("DO-%c-%d", 'A'+section-1, idx),
			Type:        SensorTypeDO,
			Stage:       StageAerobic,
			Section:     section,
			X:           100 + float64(section) * 150 + float64((i-1)%10) * 20,
			Y:           150 + float64((i-1)/10) * 40,
			TargetMin:   1.5,
			TargetMax:   2.5,
			WarningLow:  1.2,
			WarningHigh: 2.8,
			AlarmLow:    0.5,
			AlarmHigh:   3.5,
			Unit:        "mg/L",
			Name:        "溶解氧",
		})
	}
	for i := 1; i <= 20; i++ {
		section := ((i - 1) / 7) + 1
		idx := ((i - 1) % 7) + 1
		SensorConfigs = append(SensorConfigs, SensorConfig{
			ID:          fmt.Sprintf("NH3-%c-%d", 'A'+section-1, idx),
			Type:        SensorTypeNH3,
			Stage:       StageAerobic,
			Section:     section,
			X:           120 + float64(section) * 150 + float64((i-1)%7) * 25,
			Y:           200,
			TargetMin:   1.0,
			TargetMax:   2.0,
			WarningLow:  0.8,
			WarningHigh: 3.0,
			AlarmLow:    0.3,
			AlarmHigh:   5.0,
			Unit:        "mg/L",
			Name:        "氨氮",
		})
	}
	for i := 1; i <= 15; i++ {
		section := ((i - 1) / 5) + 1
		idx := ((i - 1) % 5) + 1
		SensorConfigs = append(SensorConfigs, SensorConfig{
			ID:          fmt.Sprintf("NO3-%c-%d", 'A'+section-1, idx),
			Type:        SensorTypeNO3,
			Stage:       StageAnoxic,
			Section:     section,
			X:           80 + float64(section) * 120,
			Y:           100 + float64((i-1)%5) * 30,
			TargetMin:   0.5,
			TargetMax:   3.0,
			WarningLow:  0.3,
			WarningHigh: 5.0,
			AlarmLow:    0.1,
			AlarmHigh:   8.0,
			Unit:        "mg/L",
			Name:        "硝氮",
		})
	}
	for i := 1; i <= 10; i++ {
		SensorConfigs = append(SensorConfigs, SensorConfig{
			ID:          fmt.Sprintf("PO4-%d", i),
			Type:        SensorTypePO4,
			Stage:       StageAnaerobic,
			Section:     1,
			X:           200 + float64(i) * 40,
			Y:           80,
			TargetMin:   0.3,
			TargetMax:   1.0,
			WarningLow:  0.2,
			WarningHigh: 1.5,
			AlarmLow:    0.1,
			AlarmHigh:   2.0,
			Unit:        "mg/L",
			Name:        "磷酸盐",
		})
	}
	SensorConfigs = append(SensorConfigs, SensorConfig{
		ID:          "COD-IN",
		Type:        SensorTypeCOD,
		Stage:       StagePrimary,
		Section:     1,
		X:           50,
		Y:           50,
		TargetMin:   200,
		TargetMax:   400,
		WarningLow:  150,
		WarningHigh: 500,
		AlarmLow:    100,
		AlarmHigh:   600,
		Unit:        "mg/L",
		Name:        "进水COD",
	})
	SensorConfigs = append(SensorConfigs, SensorConfig{
		ID:          "TN-EFF",
		Type:        SensorTypeTN,
		Stage:       StageEffluent,
		Section:     1,
		X:           750,
		Y:           100,
		TargetMin:   5,
		TargetMax:   15,
		WarningLow:  3,
		WarningHigh: 18,
		AlarmLow:    1,
		AlarmHigh:   20,
		Unit:        "mg/L",
		Name:        "出水总氮",
	})
	SensorConfigs = append(SensorConfigs, SensorConfig{
		ID:          "NH3-EFF",
		Type:        SensorTypeNH3,
		Stage:       StageEffluent,
		Section:     1,
		X:           750,
		Y:           150,
		TargetMin:   0.5,
		TargetMax:   1.5,
		WarningLow:  0.3,
		WarningHigh: 3,
		AlarmLow:    0.1,
		AlarmHigh:   5,
		Unit:        "mg/L",
		Name:        "出水氨氮",
	})
}

func GetSensorConfig(id string) *SensorConfig {
	for i := range SensorConfigs {
		if SensorConfigs[i].ID == id {
			return &SensorConfigs[i]
		}
	}
	return nil
}
