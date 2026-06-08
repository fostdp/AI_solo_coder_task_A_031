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

type CommandAck struct {
	CommandID   string    `json:"command_id"`
	DeviceID     string    `json:"device_id"`
	Status       string    `json:"status"`
	Message      string    `json:"message"`
	ExecutedAt   time.Time `json:"executed_at"`
	ActualValue  float64   `json:"actual_value"`
}

type AerationSection struct {
	Section      int
	AirFlowSet   float64
	AirFlowAct   float64
	ValveSet     float64
	ValveAct     float64
	FanRunning   bool
	FanSpeed     float64
	LastUpdate    time.Time
	LastCommand   time.Time
	CommandCount  int64
}

type CarbonDosing struct {
	DosingSet    float64
	DosingAct    float64
	PumpRunning  bool
	PumpSpeed    float64
	LastUpdate   time.Time
	LastCommand  time.Time
	CommandCount int64
}

type SimConfig struct {
	MQTTBroker   string
	TopicPrefix    string
	StatusInterval time.Duration
	SimInterval  time.Duration
}

var commandCounter int64

func main() {
	logger := initLogger()
	defer logger.Sync()

	cfg := loadConfig()

	aerationSections := make(map[int]*AerationSection)
	for i := 1; i <= 30; i++ {
		aerationSections[i] = &AerationSection{
			Section:  i,
			FanRunning: true,
			FanSpeed:   75,
		}
	}

	carbonDosing := &CarbonDosing{
		PumpRunning: true,
		PumpSpeed:   50,
	}

	opts := mqttclient.NewClientOptions()
	opts.AddBroker(cfg.MQTTBroker)
	opts.SetClientID("plc-simulator")
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetConnectionLostHandler(func(c mqttclient.Client, err error) {
		logger.Error("MQTT connection lost", zap.Error(err))
	})
	opts.SetOnConnectHandler(func(c mqttclient.Client) {
		logger.Info("MQTT connected")
		subscribeCommands(c, aerationSections, carbonDosing, cfg, logger)
	})

	mqttClient := mqttclient.NewClient(opts)
	token := mqttClient.Connect()
	if token.Wait() && token.Error() != nil {
		logger.Fatal("Failed to connect MQTT broker", zap.Error(token.Error()))
	}
	defer mqttClient.Disconnect(250)
	logger.Info("PLC Simulator connected to MQTT", zap.String("broker", cfg.MQTTBroker))

	stopCh := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		simulationLoop(aerationSections, carbonDosing, stopCh, cfg.SimInterval, logger)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		statusReportLoop(mqttClient, aerationSections, carbonDosing, cfg, stopCh, logger)
	}()

	logger.Info("PLC Simulator started",
		zap.Int("aeration_sections", len(aerationSections)),
		zap.Duration("status_interval", cfg.StatusInterval),
		zap.Duration("sim_interval", cfg.SimInterval))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("Shutting down PLC simulator...")
	close(stopCh)
	wg.Wait()
	logger.Info("PLC simulator stopped")
}

func loadConfig() *SimConfig {
	cfg := &SimConfig{
		MQTTBroker: "tcp://localhost:1883",
		TopicPrefix:  "sewage/",
		StatusInterval: 15 * time.Second,
		SimInterval:  1 * time.Second,
	}

	if broker := os.Getenv("MQTT_BROKER"); broker != "" {
		cfg.MQTTBroker = broker
	}

	if prefix := os.Getenv("MQTT_TOPIC_PREFIX"); prefix != "" {
		cfg.TopicPrefix = prefix
	}

	if interval := os.Getenv("STATUS_INTERVAL"); interval != "" {
		if secs, err := parseSeconds(interval); err == nil {
			cfg.StatusInterval = secs
		}
	}

	if interval := os.Getenv("SIM_INTERVAL"); interval != "" {
		if secs, err := parseSeconds(interval); err == nil {
			cfg.SimInterval = secs
		}
	}

	return cfg
}

func parseSeconds(s string) (time.Duration, error) {
	secs, err := time.ParseDuration(s + "s")
	if err != nil {
		secsInt, err2 := time.ParseDuration(s)
		if err2 != nil {
			return 0, fmt.Errorf("invalid duration: %s", s)
		}
		return secsInt, nil
	}
	return secs, nil
}

func subscribeCommands(client mqttclient.Client, sections map[int]*AerationSection, dosing *CarbonDosing, cfg *SimConfig, logger *zap.Logger) {
	subscribeAerationCommands(client, sections, cfg, logger)
	subscribeCarbonCommands(client, dosing, cfg, logger)
}

func subscribeAerationCommands(client mqttclient.Client, sections map[int]*AerationSection, cfg *SimConfig, logger *zap.Logger) {
	topic := cfg.TopicPrefix + "control/aeration/#"
	token := client.Subscribe(topic, 1, func(c mqttclient.Client, msg mqttclient.Message) {
		var cmd AerationCommand
		if err := json.Unmarshal(msg.Payload(), &cmd); err != nil {
			logger.Error("Failed to parse aeration command",
				zap.String("topic", msg.Topic()),
				zap.Error(err))
			publishAck(c, cfg.TopicPrefix, cmd.DeviceID, "failed", "Invalid command format", 0, logger)
			return
		}

		params := cmd.Params
		section, _ := params["section"].(float64)
		airFlow, _ := params["air_flow"].(float64)
		valveOpen, _ := params["valve_open"].(float64)

		sec := int(section)
		s, ok := sections[sec]
		if !ok {
			logger.Error("Invalid section number", zap.Int("section", sec))
			publishAck(c, cfg.TopicPrefix, cmd.DeviceID, "failed", fmt.Sprintf("Invalid section %d", sec), 0, logger)
			return
		}

		s.AirFlowSet = airFlow
		s.ValveSet = valveOpen
		s.LastCommand = time.Now()
		s.CommandCount++

		logger.Info("Aeration command received and executed",
			zap.Int("section", sec),
			zap.Float64("air_flow_set", airFlow),
			zap.Float64("valve_open_set", valveOpen),
			zap.Time("command_time", s.LastCommand),
			zap.Int64("command_count", s.CommandCount))

		publishAck(c, cfg.TopicPrefix, cmd.DeviceID, "executed",
			fmt.Sprintf("Aeration command executed for section %d", sec),
			s.AirFlowAct, logger)
	})
	token.Wait()
	if token.Error() != nil {
		logger.Error("Failed to subscribe to aeration commands", zap.Error(token.Error()))
	}
	logger.Info("Subscribed to aeration commands", zap.String("topic", topic))
}

func subscribeCarbonCommands(client mqttclient.Client, dosing *CarbonDosing, cfg *SimConfig, logger *zap.Logger) {
	topic := cfg.TopicPrefix + "control/carbon"
	token := client.Subscribe(topic, 1, func(c mqttclient.Client, msg mqttclient.Message) {
		var cmd AerationCommand
		if err := json.Unmarshal(msg.Payload(), &cmd); err != nil {
			logger.Error("Failed to parse carbon command",
				zap.String("topic", msg.Topic()),
				zap.Error(err))
			publishAck(c, cfg.TopicPrefix, cmd.DeviceID, "failed", "Invalid command format", 0, logger)
			return
		}

		params := cmd.Params
		dosingRate, _ := params["dosing_rate"].(float64)

		dosing.DosingSet = dosingRate
		dosing.LastCommand = time.Now()
		dosing.CommandCount++

		logger.Info("Carbon dosing command received and executed",
			zap.Float64("dosing_rate_set", dosingRate),
			zap.Time("command_time", dosing.LastCommand),
			zap.Int64("command_count", dosing.CommandCount))

		publishAck(c, cfg.TopicPrefix, cmd.DeviceID, "executed",
			"Carbon dosing command executed",
			dosing.DosingAct, logger)
	})
	token.Wait()
	if token.Error() != nil {
		logger.Error("Failed to subscribe to carbon commands", zap.Error(token.Error()))
	}
	logger.Info("Subscribed to carbon commands", zap.String("topic", topic))
}

func publishAck(client mqttclient.Client, prefix, deviceID, status, message string, actualValue float64, logger *zap.Logger) {
	commandCounter++
	ack := CommandAck{
		CommandID:  fmt.Sprintf("cmd_%d_%d", commandCounter, time.Now().UnixNano()),
		DeviceID:   deviceID,
		Status:     status,
		Message:    message,
		ExecutedAt: time.Now(),
		ActualValue: actualValue,
	}

	payload, _ := json.Marshal(ack)
	topic := prefix + "plc/ack/" + deviceID
	token := client.Publish(topic, 1, false, payload)
	go func() {
		token.Wait()
		if token.Error() != nil {
			logger.Error("Failed to publish command ack",
				zap.String("topic", topic),
				zap.Error(token.Error()))
		}
	}()

	logger.Debug("Command ack published",
		zap.String("device_id", deviceID),
		zap.String("status", status))
}

func statusReportLoop(client mqttclient.Client, sections map[int]*AerationSection, dosing *CarbonDosing, cfg *SimConfig, stopCh <-chan struct{}, logger *zap.Logger) {
	ticker := time.NewTicker(cfg.StatusInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			now := time.Now()
			reported := 0

			for sec, s := range sections {
				status := PLCStatus{
					DeviceID:    fmt.Sprintf("aeration_%d", sec),
					DeviceType:  "aeration",
					Status:      "running",
					ActualValue: s.AirFlowAct,
					SetValue:    s.AirFlowSet,
					Timestamp:   now,
				}

				if !s.FanRunning {
					status.Status = "fault"
				}

				payload, _ := json.Marshal(status)
				topic := fmt.Sprintf("%splc/status/aeration/%d", cfg.TopicPrefix, sec)
				client.Publish(topic, 1, false, payload)
				reported++
			}

			carbonStatus := PLCStatus{
				DeviceID:    "carbon_dosing_1",
				DeviceType:  "carbon_dosing",
				Status:      "running",
				ActualValue: dosing.DosingAct,
				SetValue:    dosing.DosingSet,
				Timestamp:   now,
			}
			if !dosing.PumpRunning {
				carbonStatus.Status = "fault"
			}
			payload, _ := json.Marshal(carbonStatus)
			topic := fmt.Sprintf("%splc/status/carbon", cfg.TopicPrefix)
			client.Publish(topic, 1, false, payload)

			logger.Debug("PLC status reported",
				zap.Int("aeration_sections", reported),
				zap.Bool("carbon_dosing", true))
		}
	}
}

func simulationLoop(sections map[int]*AerationSection, dosing *CarbonDosing, stopCh <-chan struct{}, interval time.Duration, logger *zap.Logger) {
	ticker := time.NewTicker(interval)
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
					if s.AirFlowAct > 500 {
						s.AirFlowAct = 500
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
				s.LastUpdate = time.Now()
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
			dosing.LastUpdate = time.Now()
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
