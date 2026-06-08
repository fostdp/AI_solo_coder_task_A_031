package main

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	mqttclient "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type AerationCommand struct {
	DeviceType string                 `json:"device_type"`
	DeviceID   string                 `json:"device_id"`
	Command    string                 `json:"command"`
	Params     map[string]interface{} `json:"params"`
	Timestamp  time.Time              `json:"timestamp"`
}

type PLCStatus struct {
	DeviceID    string    `json:"device_id"`
	DeviceType  string    `json:"device_type"`
	Status      string    `json:"status"`
	ActualValue float64   `json:"actual_value"`
	SetValue    float64   `json:"set_value"`
	Timestamp   time.Time `json:"timestamp"`
}

type AerationSection struct {
	Section      int
	AirFlowSet   float64
	AirFlowAct   float64
	ValveSet     float64
	ValveAct     float64
	FanRunning   bool
	FanSpeed     float64
	LastUpdate   time.Time
}

type CarbonDosing struct {
	DosingSet    float64
	DosingAct    float64
	PumpRunning  bool
	PumpSpeed    float64
	LastUpdate   time.Time
}

func main() {
	logger := initLogger()
	defer logger.Sync()

	mqttBroker := os.Getenv("MQTT_BROKER")
	if mqttBroker == "" {
		mqttBroker = "tcp://localhost:1883"
	}

	topicPrefix := os.Getenv("MQTT_TOPIC_PREFIX")
	if topicPrefix == "" {
		topicPrefix = "sewage/"
	}

	aerationSections := make(map[int]*AerationSection)
	for i := 1; i <= 30; i++ {
		aerationSections[i] = &AerationSection{
			Section:    i,
			FanRunning: true,
			FanSpeed:   75,
		}
	}

	carbonDosing := &CarbonDosing{
		PumpRunning: true,
		PumpSpeed:   50,
	}

	opts := mqttclient.NewClientOptions()
	opts.AddBroker(mqttBroker)
	opts.SetClientID("plc-simulator")
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)

	mqttClient := mqttclient.NewClient(opts)
	token := mqttClient.Connect()
	if token.Wait() && token.Error() != nil {
		logger.Fatal("Failed to connect MQTT broker", zap.Error(token.Error()))
	}
	defer mqttClient.Disconnect(250)
	logger.Info("PLC Simulator connected to MQTT", zap.String("broker", mqttBroker))

	stopCh := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		subscribeAerationCommands(mqttClient, aerationSections, topicPrefix, logger)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		subscribeCarbonCommands(mqttClient, carbonDosing, topicPrefix, logger)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		statusReportLoop(mqttClient, aerationSections, carbonDosing, topicPrefix, stopCh, logger)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		simulationLoop(aerationSections, carbonDosing, stopCh, logger)
	}()

	logger.Info("PLC Simulator started",
		zap.Int("aeration_sections", len(aerationSections)))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("Shutting down PLC simulator...")
	close(stopCh)
	wg.Wait()
	logger.Info("PLC simulator stopped")
}

func subscribeAerationCommands(client mqttclient.Client, sections map[int]*AerationSection, prefix string, logger *zap.Logger) {
	topic := prefix + "control/aeration/#"
	token := client.Subscribe(topic, 1, func(c mqttclient.Client, msg mqttclient.Message) {
		var cmd AerationCommand
		if err := json.Unmarshal(msg.Payload(), &cmd); err != nil {
			logger.Error("Failed to parse aeration command",
				zap.String("topic", msg.Topic()),
				zap.Error(err))
			return
		}

		params := cmd.Params
		section, _ := params["section"].(float64)
		airFlow, _ := params["air_flow"].(float64)
		valveOpen, _ := params["valve_open"].(float64)

		sec := int(section)
		if s, ok := sections[sec]; ok {
			s.AirFlowSet = airFlow
			s.ValveSet = valveOpen
			s.LastUpdate = time.Now()

			logger.Info("Aeration command received",
				zap.Int("section", sec),
				zap.Float64("air_flow_set", airFlow),
				zap.Float64("valve_open", valveOpen))
		}
	})
	token.Wait()
	if token.Error() != nil {
		logger.Error("Failed to subscribe to aeration commands", zap.Error(token.Error()))
	}
	logger.Info("Subscribed to aeration commands", zap.String("topic", topic))
}

func subscribeCarbonCommands(client mqttclient.Client, dosing *CarbonDosing, prefix string, logger *zap.Logger) {
	topic := prefix + "control/carbon"
	token := client.Subscribe(topic, 1, func(c mqttclient.Client, msg mqttclient.Message) {
		var cmd AerationCommand
		if err := json.Unmarshal(msg.Payload(), &cmd); err != nil {
			logger.Error("Failed to parse carbon command",
				zap.String("topic", msg.Topic()),
				zap.Error(err))
			return
		}

		params := cmd.Params
		dosingRate, _ := params["dosing_rate"].(float64)

		dosing.DosingSet = dosingRate
		dosing.LastUpdate = time.Now()

		logger.Info("Carbon dosing command received",
			zap.Float64("dosing_rate", dosingRate))
	})
	token.Wait()
	if token.Error() != nil {
		logger.Error("Failed to subscribe to carbon commands", zap.Error(token.Error()))
	}
	logger.Info("Subscribed to carbon commands", zap.String("topic", topic))
}

func statusReportLoop(client mqttclient.Client, sections map[int]*AerationSection, dosing *CarbonDosing, prefix string, stopCh <-chan struct{}, logger *zap.Logger) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			for sec, s := range sections {
				status := PLCStatus{
					DeviceID:    fmt.Sprintf("aeration_%d", sec),
					DeviceType:  "aeration",
					Status:      "running",
					ActualValue: s.AirFlowAct,
					SetValue:    s.AirFlowSet,
					Timestamp:   time.Now(),
				}

				if !s.FanRunning {
					status.Status = "fault"
				}

				payload, _ := json.Marshal(status)
				topic := fmt.Sprintf("%splc/status/aeration/%d", prefix, sec)
				client.Publish(topic, 1, false, payload)
			}

			status := PLCStatus{
				DeviceID:    "carbon_dosing_1",
				DeviceType:  "carbon_dosing",
				Status:      "running",
				ActualValue: dosing.DosingAct,
				SetValue:    dosing.DosingSet,
				Timestamp:   time.Now(),
			}
			if !dosing.PumpRunning {
				status.Status = "fault"
			}
			payload, _ := json.Marshal(status)
			topic := fmt.Sprintf("%splc/status/carbon", prefix)
			client.Publish(topic, 1, false, payload)

			logger.Debug("PLC status reported",
				zap.Int("aeration_sections", len(sections)))
		}
	}
}

func simulationLoop(sections map[int]*AerationSection, dosing *CarbonDosing, stopCh <-chan struct{}, logger *zap.Logger) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			for _, s := range sections {
				if s.FanRunning {
					noise := (rand.Float64() - 0.5) * 2
					targetFlow := s.AirFlowSet * (s.ValveSet / 100.0)

					s.AirFlowAct += (targetFlow - s.AirFlowAct) * 0.1
					s.AirFlowAct += noise

					if s.AirFlowAct < 0 {
						s.AirFlowAct = 0
					}
					if s.AirFlowAct > 120 {
						s.AirFlowAct = 120
					}

					s.ValveAct = s.ValveSet * (0.98 + rand.Float64()*0.04)

					s.FanSpeed = math.Min(100, s.AirFlowSet*1.2)

					if rand.Float64() < 0.0001 {
						s.FanRunning = false
						logger.Warn("Fan failure simulated",
							zap.Int("section", s.Section))
					}
				} else {
					s.AirFlowAct *= 0.95
					if s.AirFlowAct < 0.5 {
						s.AirFlowAct = 0
					}

					if rand.Float64() < 0.01 {
						s.FanRunning = true
						logger.Info("Fan recovered",
							zap.Int("section", s.Section))
					}
				}
			}

			if dosing.PumpRunning {
				noise := (rand.Float64() - 0.5) * 2
				dosing.DosingAct += (dosing.DosingSet - dosing.DosingAct) * 0.05
				dosing.DosingAct += noise

				if dosing.DosingAct < 0 {
					dosing.DosingAct = 0
				}
				if dosing.DosingAct > 200 {
					dosing.DosingAct = 200
				}

				dosing.PumpSpeed = math.Min(100, dosing.DosingSet/1.5)

				if rand.Float64() < 0.00005 {
					dosing.PumpRunning = false
					logger.Warn("Carbon pump failure simulated")
				}
			} else {
				dosing.DosingAct *= 0.9
				if dosing.DosingAct < 0.1 {
					dosing.DosingAct = 0
				}

				if rand.Float64() < 0.02 {
					dosing.PumpRunning = true
					logger.Info("Carbon pump recovered")
				}
			}
		}
	}
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
