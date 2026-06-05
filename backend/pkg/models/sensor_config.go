package models

import (
	"math"
	"math/rand"
	"time"
)

func GenerateSensorConfigs() []*SensorConfig {
	configs := make([]*SensorConfig, 0)
	locations := []string{"anaerobic", "anoxic", "aerobic1", "aerobic2", "aerobic3", "effluent"}

	for i := 0; i < 30; i++ {
		location := locations[int(math.Min(float64(i/6), 5))]
		x, y := getSensorPosition(SensorTypeDO, i, location)
		configs = append(configs, &SensorConfig{
			SensorID:    "DO-" + padZero(i+1, 3),
			Type:        SensorTypeDO,
			Location:    location,
			Setpoint:    2.0,
			MinValue:    0,
			MaxValue:    10,
			X:           x,
			Y:           y,
			Description: "溶解氧传感器",
		})
	}

	for i := 0; i < 20; i++ {
		location := locations[int(math.Min(float64(i/4), 5))]
		x, y := getSensorPosition(SensorTypeNH3, i, location)
		configs = append(configs, &SensorConfig{
			SensorID:    "NH3-" + padZero(i+1, 3),
			Type:        SensorTypeNH3,
			Location:    location,
			Setpoint:    1.5,
			MinValue:    0,
			MaxValue:    50,
			X:           x,
			Y:           y,
			Description: "氨氮传感器",
		})
	}

	for i := 0; i < 15; i++ {
		location := locations[int(math.Min(float64(i/3), 5))]
		x, y := getSensorPosition(SensorTypeNO3, i, location)
		configs = append(configs, &SensorConfig{
			SensorID:    "NO3-" + padZero(i+1, 3),
			Type:        SensorTypeNO3,
			Location:    location,
			Setpoint:    10.0,
			MinValue:    0,
			MaxValue:    30,
			X:           x,
			Y:           y,
			Description: "硝氮传感器",
		})
	}

	for i := 0; i < 10; i++ {
		location := locations[int(math.Min(float64(i/2), 5))]
		x, y := getSensorPosition(SensorTypePO4, i, location)
		configs = append(configs, &SensorConfig{
			SensorID:    "PO4-" + padZero(i+1, 3),
			Type:        SensorTypePO4,
			Location:    location,
			Setpoint:    0.5,
			MinValue:    0,
			MaxValue:    10,
			X:           x,
			Y:           y,
			Description: "磷酸盐传感器",
		})
	}

	return configs
}

func getSensorPosition(sensorType SensorType, index int, location string) (float64, float64) {
	locationCoords := map[string][2]float64{
		"anaerobic": {150, 300},
		"anoxic":    {350, 300},
		"aerobic1":  {550, 300},
		"aerobic2":  {700, 300},
		"aerobic3":  {850, 300},
		"effluent":  {1000, 300},
	}

	base, ok := locationCoords[location]
	if !ok {
		base = [2]float64{500, 300}
	}

	typeOffset := map[SensorType]float64{
		SensorTypeDO:  -30,
		SensorTypeNH3: 0,
		SensorTypeNO3: 30,
		SensorTypePO4: 60,
	}

	offsetY := typeOffset[sensorType]
	offsetX := float64((index % 6) - 3) * 15

	rand.Seed(time.Now().UnixNano())
	offsetX += rand.Float64()*10 - 5
	offsetY += rand.Float64()*10 - 5

	return base[0] + offsetX, base[1] + offsetY
}

func padZero(num int, length int) string {
	result := ""
	n := num
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n = n / 10
	}
	for len(result) < length {
		result = "0" + result
	}
	return result
}

func GetProcessSections() []*ProcessSection {
	return []*ProcessSection{
		{
			ID:          "coarse_bar",
			Name:        "粗格栅",
			Type:        "pre_treatment",
			X:           50,
			Y:           250,
			Width:       80,
			Height:      100,
			Status:      "normal",
			Description: "去除大颗粒悬浮物",
		},
		{
			ID:          "fine_bar",
			Name:        "细格栅",
			Type:        "pre_treatment",
			X:           150,
			Y:           250,
			Width:       80,
			Height:      100,
			Status:      "normal",
			Description: "去除细小悬浮物",
		},
		{
			ID:          "grit_chamber",
			Name:        "沉砂池",
			Type:        "pre_treatment",
			X:           250,
			Y:           250,
			Width:       80,
			Height:      100,
			Status:      "normal",
			Description: "去除砂粒",
		},
		{
			ID:          "primary_settler",
			Name:        "初沉池",
			Type:        "primary_treatment",
			X:           350,
			Y:           250,
			Width:       100,
			Height:      100,
			Status:      "normal",
			Description: "初级沉淀",
		},
		{
			ID:          "anaerobic",
			Name:        "厌氧池",
			Type:        "biological",
			X:           100,
			Y:           400,
			Width:       120,
			Height:      150,
			Status:      "normal",
			Description: "厌氧释磷",
		},
		{
			ID:          "anoxic",
			Name:        "缺氧池",
			Type:        "biological",
			X:           240,
			Y:           400,
			Width:       120,
			Height:      150,
			Status:      "normal",
			Description: "反硝化脱氮",
		},
		{
			ID:          "aerobic1",
			Name:        "好氧池1段",
			Type:        "biological",
			X:           380,
			Y:           400,
			Width:       140,
			Height:      150,
			Status:      "normal",
			Description: "硝化反应",
		},
		{
			ID:          "aerobic2",
			Name:        "好氧池2段",
			Type:        "biological",
			X:           540,
			Y:           400,
			Width:       140,
			Height:      150,
			Status:      "normal",
			Description: "硝化反应",
		},
		{
			ID:          "aerobic3",
			Name:        "好氧池3段",
			Type:        "biological",
			X:           700,
			Y:           400,
			Width:       140,
			Height:      150,
			Status:      "normal",
			Description: "硝化反应",
		},
		{
			ID:          "secondary_settler",
			Name:        "二沉池",
			Type:        "secondary_treatment",
			X:           860,
			Y:           400,
			Width:       120,
			Height:      150,
			Status:      "normal",
			Description: "二次沉淀",
		},
		{
			ID:          "advanced_treatment",
			Name:        "深度处理",
			Type:        "advanced_treatment",
			X:           1000,
			Y:           400,
			Width:       100,
			Height:      150,
			Status:      "normal",
			Description: "深度过滤消毒",
		},
	}
}

func GenerateBiologicalTankProfile() []map[string]interface{} {
	profile := make([]map[string]interface{}, 0)

	depths := []float64{0.5, 1.0, 1.5, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5, 5.0}
	zones := []string{"anaerobic", "anoxic", "aerobic1", "aerobic2", "aerobic3"}

	for _, zone := range zones {
		for _, depth := range depths {
			doValue := getDOValue(zone, depth)
			profile = append(profile, map[string]interface{}{
				"zone":  zone,
				"depth": depth,
				"do":    doValue,
				"color": getDOColor(doValue),
			})
		}
	}

	return profile
}

func getDOValue(zone string, depth float64) float64 {
	baseDO := map[string]float64{
		"anaerobic": 0.1,
		"anoxic":    0.3,
		"aerobic1":  2.5,
		"aerobic2":  2.0,
		"aerobic3":  1.5,
	}

	base := baseDO[zone]
	decay := 0.1 * (depth / 5.0)
	return math.Max(0, base - decay + (rand.Float64()-0.5)*0.2)
}

func getDOColor(do float64) string {
	if do < 0.5 {
		return "#2c3e50"
	} else if do < 1.5 {
		return "#e74c3c"
	} else if do < 2.0 {
		return "#f39c12"
	} else if do < 3.0 {
		return "#27ae60"
	} else {
		return "#3498db"
	}
}
