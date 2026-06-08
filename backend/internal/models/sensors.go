package models

import "fmt"

func InitializeSensors() map[string]SensorInfo {
	sensors := make(map[string]SensorInfo)

	idx := 1

	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("DO-AER-%02d", i)
		sensors[id] = SensorInfo{
			ID:           id,
			Type:         SensorDO,
			Stage:        StageAerobic,
			Section:      i,
			X:            150 + float64(i-1)*130,
			Y:            120,
			Setpoint:     2.0,
			MinDeviation: 0.1,
			MaxDeviation: 0.4,
		}
		idx++
	}

	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("DO-AER-%02d", i+5)
		sensors[id] = SensorInfo{
			ID:           id,
			Type:         SensorDO,
			Stage:        StageAerobic,
			Section:      i + 5,
			X:            150 + float64(i-1)*130,
			Y:            280,
			Setpoint:     2.0,
			MinDeviation: 0.1,
			MaxDeviation: 0.4,
		}
		idx++
	}

	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("DO-AER-%02d", i+10)
		sensors[id] = SensorInfo{
			ID:           id,
			Type:         SensorDO,
			Stage:        StageAerobic,
			Section:      i + 10,
			X:            150 + float64(i-1)*130,
			Y:            440,
			Setpoint:     2.0,
			MinDeviation: 0.1,
			MaxDeviation: 0.4,
		}
		idx++
	}

	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("DO-AER-%02d", i+15)
		sensors[id] = SensorInfo{
			ID:           id,
			Type:         SensorDO,
			Stage:        StageAerobic,
			Section:      i + 15,
			X:            150 + float64(i-1)*130,
			Y:            600,
			Setpoint:     2.0,
			MinDeviation: 0.1,
			MaxDeviation: 0.4,
		}
		idx++
	}

	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("DO-AER-%02d", i+20)
		sensors[id] = SensorInfo{
			ID:           id,
			Type:         SensorDO,
			Stage:        StageAerobic,
			Section:      i + 20,
			X:            150 + float64(i-1)*130,
			Y:            760,
			Setpoint:     2.0,
			MinDeviation: 0.1,
			MaxDeviation: 0.4,
		}
		idx++
	}

	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("DO-AER-%02d", i+25)
		sensors[id] = SensorInfo{
			ID:           id,
			Type:         SensorDO,
			Stage:        StageAerobic,
			Section:      i + 25,
			X:            150 + float64(i-1)*130,
			Y:            920,
			Setpoint:     2.0,
			MinDeviation: 0.1,
			MaxDeviation: 0.4,
		}
		idx++
	}

	for i := 1; i <= 10; i++ {
		id := fmt.Sprintf("NH3-AER-%02d", i)
		sensors[id] = SensorInfo{
			ID:           id,
			Type:         SensorNH3,
			Stage:        StageAerobic,
			Section:      i*3 - 1,
			X:            215 + float64(i-1)*130,
			Y:            180,
			Setpoint:     1.5,
			MinDeviation: 0.1,
			MaxDeviation: 0.3,
		}
	}

	for i := 1; i <= 10; i++ {
		id := fmt.Sprintf("NH3-AER-%02d", i+10)
		sensors[id] = SensorInfo{
			ID:           id,
			Type:         SensorNH3,
			Stage:        StageAerobic,
			Section:      (i+10)*3 - 1,
			X:            215 + float64(i-1)*130,
			Y:            520,
			Setpoint:     1.5,
			MinDeviation: 0.1,
			MaxDeviation: 0.3,
		}
	}

	for i := 1; i <= 8; i++ {
		id := fmt.Sprintf("NO3-ANX-%02d", i)
		sensors[id] = SensorInfo{
			ID:           id,
			Type:         SensorNO3,
			Stage:        StageAnoxic,
			Section:      i,
			X:            120 + float64(i-1)*100,
			Y:            150,
			Setpoint:     5.0,
			MinDeviation: 0.5,
			MaxDeviation: 1.5,
		}
	}

	for i := 1; i <= 7; i++ {
		id := fmt.Sprintf("NO3-ANX-%02d", i+8)
		sensors[id] = SensorInfo{
			ID:           id,
			Type:         SensorNO3,
			Stage:        StageAnoxic,
			Section:      i + 8,
			X:            120 + float64(i-1)*100,
			Y:            350,
			Setpoint:     5.0,
			MinDeviation: 0.5,
			MaxDeviation: 1.5,
		}
	}

	for i := 1; i <= 10; i++ {
		id := fmt.Sprintf("PO4-AER-%02d", i)
		sensors[id] = SensorInfo{
			ID:           id,
			Type:         SensorPO4,
			Stage:        StageAnaerobic,
			Section:      i,
			X:            150 + float64(i-1)*115,
			Y:            150,
			Setpoint:     2.0,
			MinDeviation: 0.2,
			MaxDeviation: 0.5,
		}
	}

	id := "COD-INF-01"
	sensors[id] = SensorInfo{
		ID:           id,
		Type:         SensorCOD,
		Stage:        StagePrimarySett,
		Section:      1,
		X:            50,
		Y:            80,
		Setpoint:     300,
		MinDeviation: 20,
		MaxDeviation: 50,
	}

	id = "NH3-EFF-01"
	sensors[id] = SensorInfo{
		ID:           id,
		Type:         SensorNH3,
		Stage:        StageEffluent,
		Section:      1,
		X:            900,
		Y:            80,
		Setpoint:     1.5,
		MinDeviation: 0.1,
		MaxDeviation: 0.5,
	}

	id = "TN-EFF-01"
	sensors[id] = SensorInfo{
		ID:           id,
		Type:         SensorTN,
		Stage:        StageEffluent,
		Section:      1,
		X:            900,
		Y:            120,
		Setpoint:     10,
		MinDeviation: 0.5,
		MaxDeviation: 2.0,
	}

	id = "TP-EFF-01"
	sensors[id] = SensorInfo{
		ID:           id,
		Type:         SensorTP,
		Stage:        StageEffluent,
		Section:      1,
		X:            900,
		Y:            160,
		Setpoint:     0.5,
		MinDeviation: 0.05,
		MaxDeviation: 0.15,
	}

	id = "FLOW-INF-01"
	sensors[id] = SensorInfo{
		ID:           id,
		Type:         SensorFlow,
		Stage:        StageCoarseGrate,
		Section:      1,
		X:            30,
		Y:            50,
		Setpoint:     1250,
		MinDeviation: 100,
		MaxDeviation: 300,
	}

	return sensors
}
