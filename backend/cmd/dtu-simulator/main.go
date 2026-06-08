package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type SensorData struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Stage     string    `json:"stage"`
	Section   int       `json:"section"`
	Value     float64   `json:"value"`
	Setpoint  float64   `json:"setpoint"`
	Timestamp time.Time `json:"timestamp"`
	DTUID     string    `json:"dtu_id"`
	Status    string    `json:"status"`
}

type SensorSim struct {
	ID        string
	Type      string
	Stage     string
	Section   int
	Setpoint  float64
	Noise     float64
	Drift     float64
	LastValue float64
}

type SimConfig struct {
	APIURL               string
	ReportInterval       time.Duration
	WaterQualityFluctuation float64
	SensorCounts         struct {
		DO      int
		NH3     int
		NO3     int
		PO4     int
		Extra   int
	}
}

func main() {
	logger := initLogger()
	defer logger.Sync()

	cfg := loadConfig()
	sensors := createSensors(cfg)

	logger.Info("DTU Simulator started",
		zap.Int("sensor_count", len(sensors)),
		zap.String("api_url", cfg.APIURL),
		zap.Duration("report_interval", cfg.ReportInterval),
		zap.Float64("water_quality_fluctuation", cfg.WaterQualityFluctuation),
		zap.Int("do_sensors", cfg.SensorCounts.DO),
		zap.Int("nh3_sensors", cfg.SensorCounts.NH3),
		zap.Int("no3_sensors", cfg.SensorCounts.NO3),
		zap.Int("po4_sensors", cfg.SensorCounts.PO4))

	stopCh := make(chan struct{})
	var wg sync.WaitGroup

	ticker := time.NewTicker(cfg.ReportInterval)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				reportAllSensors(sensors, cfg, logger)
			}
		}
	}()

	go func() {
		reportAllSensors(sensors, cfg, logger)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("Shutting down DTU simulator...")
	close(stopCh)
	wg.Wait()
	logger.Info("DTU simulator stopped")
}

func loadConfig() *SimConfig {
	cfg := &SimConfig{
		APIURL:               "http://localhost:8080/api/v1",
		ReportInterval:       120 * time.Second,
		WaterQualityFluctuation: 0.3,
		SensorCounts: struct {
			DO    int
			NH3   int
			NO3   int
			PO4   int
			Extra int
		}{
			DO:    30,
			NH3:   20,
			NO3:   15,
			PO4:   5,
			Extra: 5,
		},
	}

	if envURL := os.Getenv("API_URL"); envURL != "" {
		cfg.APIURL = envURL
	}

	if envInterval := os.Getenv("REPORT_INTERVAL"); envInterval != "" {
		if secs, err := strconv.Atoi(envInterval); err == nil {
			cfg.ReportInterval = time.Duration(secs) * time.Second
		}
	}

	if envFluct := os.Getenv("WATER_QUALITY_FLUCTUATION"); envFluct != "" {
		if fluct, err := strconv.ParseFloat(envFluct, 64); err == nil {
			cfg.WaterQualityFluctuation = math.Max(0, math.Min(1, fluct))
		}
	}

	if envCount := os.Getenv("SENSOR_COUNT"); envCount != "" {
		if count, err := strconv.Atoi(envCount); err == nil {
			allocateSensors(cfg, count)
		}
	}

	return cfg
}

func allocateSensors(cfg *SimConfig, total int) {
	extra := 5
	remaining := total - extra
	if remaining < 0 {
		remaining = 0
	}

	cfg.SensorCounts.DO = int(float64(remaining) * 0.4)
	cfg.SensorCounts.NH3 = int(float64(remaining) * 0.3)
	cfg.SensorCounts.NO3 = int(float64(remaining) * 0.2)
	cfg.SensorCounts.PO4 = remaining - cfg.SensorCounts.DO - cfg.SensorCounts.NH3 - cfg.SensorCounts.NO3

	if cfg.SensorCounts.PO4 < 0 {
		cfg.SensorCounts.PO4 = 0
	}
	cfg.SensorCounts.Extra = extra
}

func createSensors(cfg *SimConfig) []*SensorSim {
	var sensors []*SensorSim
	fluct := cfg.WaterQualityFluctuation

	for i := 1; i <= cfg.SensorCounts.DO; i++ {
		sensors = append(sensors, &SensorSim{
			ID:        fmt.Sprintf("DO-AER-%02d", i),
			Type:      "DO",
			Stage:     "aerobic",
			Section:   i,
			Setpoint:  2.0,
			Noise:     0.15 * (0.5 + fluct),
			Drift:     0.02 * (0.5 + fluct),
			LastValue: 2.0,
		})
	}

	for i := 1; i <= cfg.SensorCounts.NH3; i++ {
		section := i*3 - 1
		if section > cfg.SensorCounts.DO {
			section = cfg.SensorCounts.DO
		}
		sensors = append(sensors, &SensorSim{
			ID:        fmt.Sprintf("NH3-AER-%02d", i),
			Type:      "NH3",
			Stage:     "aerobic",
			Section:   section,
			Setpoint:  1.5,
			Noise:     0.1 * (0.5 + fluct),
			Drift:     0.015 * (0.5 + fluct),
			LastValue: 1.5,
		})
	}

	for i := 1; i <= cfg.SensorCounts.NO3; i++ {
		sensors = append(sensors, &SensorSim{
			ID:        fmt.Sprintf("NO3-ANX-%02d", i),
			Type:      "NO3",
			Stage:     "anoxic",
			Section:   i,
			Setpoint:  5.0,
			Noise:     0.3 * (0.5 + fluct),
			Drift:     0.05 * (0.5 + fluct),
			LastValue: 5.0,
		})
	}

	for i := 1; i <= cfg.SensorCounts.PO4; i++ {
		sensors = append(sensors, &SensorSim{
			ID:        fmt.Sprintf("PO4-ANA-%02d", i),
			Type:      "PO4",
			Stage:     "anaerobic",
			Section:   i,
			Setpoint:  2.0,
			Noise:     0.15 * (0.5 + fluct),
			Drift:     0.02 * (0.5 + fluct),
			LastValue: 2.0,
		})
	}

	sensors = append(sensors, &SensorSim{
		ID:        "COD-INF-01",
		Type:      "COD",
		Stage:     "primary_settling",
		Section:   1,
		Setpoint:  300,
		Noise:     25 * (0.5 + fluct),
		Drift:     3 * (0.5 + fluct),
		LastValue: 300,
	})

	sensors = append(sensors, &SensorSim{
		ID:        "NH3-EFF-01",
		Type:      "NH3",
		Stage:     "effluent",
		Section:   1,
		Setpoint:  1.5,
		Noise:     0.08 * (0.5 + fluct),
		Drift:     0.01 * (0.5 + fluct),
		LastValue: 1.5,
	})

	sensors = append(sensors, &SensorSim{
		ID:        "TN-EFF-01",
		Type:      "TN",
		Stage:     "effluent",
		Section:   1,
		Setpoint:  10,
		Noise:     0.8 * (0.5 + fluct),
		Drift:     0.1 * (0.5 + fluct),
		LastValue: 10,
	})

	sensors = append(sensors, &SensorSim{
		ID:        "TP-EFF-01",
		Type:      "TP",
		Stage:     "effluent",
		Section:   1,
		Setpoint:  0.5,
		Noise:     0.04 * (0.5 + fluct),
		Drift:     0.005 * (0.5 + fluct),
		LastValue: 0.5,
	})

	sensors = append(sensors, &SensorSim{
		ID:        "FLOW-INF-01",
		Type:      "FLOW",
		Stage:     "coarse_grate",
		Section:   1,
		Setpoint:  1250,
		Noise:     80 * (0.5 + fluct),
		Drift:     10 * (0.5 + fluct),
		LastValue: 1250,
	})

	return sensors
}

func reportAllSensors(sensors []*SensorSim, cfg *SimConfig, logger *zap.Logger) {
	now := time.Now()
	dtuID := fmt.Sprintf("DTU-%04d", rand.Intn(10000))

	logger.Info("Reporting sensor data",
		zap.Int("count", len(sensors)),
		zap.Time("timestamp", now))

	for _, sensor := range sensors {
		value := simulateValue(sensor, cfg.WaterQualityFluctuation)
		sensor.LastValue = value

		data := SensorData{
			ID:        sensor.ID,
			Type:      sensor.Type,
			Stage:     sensor.Stage,
			Section:   sensor.Section,
			Value:     math.Round(value*1000) / 1000,
			Setpoint:  sensor.Setpoint,
			Timestamp: now,
			DTUID:     dtuID,
			Status:    "online",
		}

		if err := sendData(cfg.APIURL, data); err != nil {
			logger.Error("Failed to send sensor data",
				zap.String("sensor_id", sensor.ID),
				zap.Error(err))
		} else {
			logger.Debug("Sensor data sent",
				zap.String("id", sensor.ID),
				zap.String("type", sensor.Type),
				zap.Float64("value", value))
		}

		time.Sleep(10 * time.Millisecond)
	}

	logger.Info("Sensor data report completed")
}

func simulateValue(s *SensorSim, fluct float64) float64 {
	period := 3600.0
	trend := math.Sin(float64(time.Now().Unix()%int64(period)) / (period / 6))
	noise := (rand.Float64() - 0.5) * 2 * s.Noise
	drift := (rand.Float64() - 0.5) * s.Drift

	baseValue := s.LastValue + drift
	if trend > 0.8 && rand.Float64() > 0.7 {
		baseValue += s.Setpoint * 0.05 * fluct
	} else if trend < -0.8 && rand.Float64() > 0.7 {
		baseValue -= s.Setpoint * 0.05 * fluct
	}

	value := baseValue + noise + s.Setpoint*trend*0.03*fluct

	minVal := s.Setpoint * (0.5 - fluct*0.3)
	maxVal := s.Setpoint * (2.0 + fluct*0.5)
	if value < minVal {
		value = minVal
	}
	if value > maxVal {
		value = maxVal
	}

	return value
}

func sendData(apiURL string, data SensorData) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	url := fmt.Sprintf("%s/sensors/data", apiURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DTU-ID", data.DTUID)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func initLogger() *zap.Logger {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder

	cfg := zap.Config{
		Level:            zap.NewAtomicLevelAt(zap.InfoLevel),
		Development:      false,
		Sampling:         nil,
		Encoding:         "console",
		EncoderConfig:    encoderCfg,
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	return logger
}
