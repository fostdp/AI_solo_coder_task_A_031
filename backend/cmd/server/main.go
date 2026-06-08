package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"sewage-treatment-system/internal/alarm"
	"sewage-treatment-system/internal/api"
	"sewage-treatment-system/internal/collector"
	"sewage-treatment-system/internal/config"
	"sewage-treatment-system/internal/controller"
	"sewage-treatment-system/internal/influxdb"
	"sewage-treatment-system/internal/messages"
	"sewage-treatment-system/internal/models"
	"sewage-treatment-system/internal/mqtt"
	"sewage-treatment-system/internal/websocket"
)

func main() {
	logger := initLogger()
	defer logger.Sync()

	if err := config.Load("./config.yaml"); err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	cfg := config.AppConfig
	sensorInfo := models.InitializeSensors()

	influxClient, err := influxdb.New(&cfg.InfluxDB, logger)
	if err != nil {
		logger.Fatal("Failed to create InfluxDB client", zap.Error(err))
	}
	defer influxClient.Close()
	logger.Info("InfluxDB client connected")

	mqttClient, err := mqtt.New(&cfg.MQTT, logger)
	if err != nil {
		logger.Warn("Failed to create MQTT client, running without MQTT", zap.Error(err))
	}
	if mqttClient != nil {
		defer mqttClient.Close()
	}

	wsServer := websocket.NewServer(&cfg.WebSocket, logger)
	go wsServer.Run()
	logger.Info("WebSocket server started")

	bufferSize := cfg.Collector.ChannelBufferSize
	if bufferSize == 0 {
		bufferSize = 1000
	}

	validatedSensorCh := make(chan *messages.SensorDataMessage, bufferSize)
	aerationControlCh := make(chan *messages.AerationControlMessage, bufferSize)
	carbonDosingCh := make(chan *messages.CarbonDosingMessage, bufferSize)
	alarmCh := make(chan *messages.AlarmMessage, bufferSize)

	collectorChannels := &collector.CollectorChannels{
		ValidatedData: validatedSensorCh,
		AlarmOut:      alarmCh,
	}

	sensorCollector := collector.NewSensorCollector(
		&cfg.Collector,
		influxClient,
		wsServer,
		logger,
		sensorInfo,
		collectorChannels,
	)
	logger.Info("Sensor collector initialized")

	if mqttClient != nil {
		go setupMQTTHandlers(mqttClient, sensorCollector, logger)
	}

	aerationChannels := &controller.AerationChannels{
		SensorDataIn: validatedSensorCh,
		ControlOut:   aerationControlCh,
		AlarmOut:     alarmCh,
	}

	aerationController := controller.NewAerationController(
		&cfg.Controller.Aeration,
		influxClient,
		mqttClient,
		logger,
		aerationChannels,
	)
	logger.Info("Aeration controller initialized",
		zap.Int("sections", cfg.Controller.Aeration.NumSections))

	aerationSensorCh := make(chan *messages.SensorDataMessage, bufferSize)
	carbonChannels := &controller.CarbonChannels{
		SensorDataIn: aerationSensorCh,
		CarbonOut:    carbonDosingCh,
		AlarmOut:     alarmCh,
	}

	carbonOptimizer := controller.NewCarbonOptimizer(
		&cfg.Controller.Carbon,
		influxClient,
		mqttClient,
		logger,
		carbonChannels,
	)
	logger.Info("Carbon optimizer initialized")

	alarmRouterChannels := &alarm.AlarmRouterChannels{
		AlarmIn: alarmCh,
	}

	alarmRouter := alarm.NewAlarmRouter(
		&cfg.Alarm,
		influxClient,
		wsServer,
		logger,
		sensorInfo,
		alarmRouterChannels,
	)
	logger.Info("Alarm router initialized")

	apiServer := api.NewServer(
		influxClient,
		wsServer,
		aerationController,
		carbonOptimizer,
		alarmRouter,
		sensorCollector,
		sensorInfo,
		logger,
		cfg.Server.Mode,
	)
	logger.Info("API server initialized")

	stopCh := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		aerationController.ControlLoop(stopCh)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		carbonOptimizer.ControlLoop(stopCh)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		alarmRouter.RouterLoop(stopCh)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		sensorDataFanout(validatedSensorCh, []chan *messages.SensorDataMessage{
			aerationSensorCh,
		}, stopCh)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		controlOutputFanout(aerationControlCh, carbonDosingCh, stopCh, logger)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		metricsLoop(influxClient, wsServer, stopCh, logger)
	}()

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	logger.Info("API server starting", zap.String("addr", addr))
	go func() {
		if err := apiServer.Run(addr); err != nil {
			logger.Fatal("API server failed", zap.Error(err))
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("Shutting down...")
	close(stopCh)
	wg.Wait()
	logger.Info("Server stopped")
}

func sensorDataFanout(
	in <-chan *messages.SensorDataMessage,
	outs []chan *messages.SensorDataMessage,
	stopCh <-chan struct{},
) {
	for {
		select {
		case <-stopCh:
			return
		case msg := <-in:
			for _, out := range outs {
				select {
				case out <- msg:
				default:
				}
			}
		}
	}
}

func controlOutputFanout(
	aerationCh <-chan *messages.AerationControlMessage,
	carbonCh <-chan *messages.CarbonDosingMessage,
	stopCh <-chan struct{},
	logger *zap.Logger,
) {
	for {
		select {
		case <-stopCh:
			return
		case msg := <-aerationCh:
			logger.Debug("Aeration control output",
				zap.Int("section", msg.Section),
				zap.Float64("air_flow", msg.AirFlowSet))
		case msg := <-carbonCh:
			logger.Debug("Carbon dosing output",
				zap.Float64("dosing_rate", msg.DosingRate),
				zap.String("trigger", msg.TriggerType))
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

func setupMQTTHandlers(mqttClient *mqtt.Client, sensorCollector *collector.SensorCollector, logger *zap.Logger) {
	err := mqttClient.Subscribe("sensor/data", func(topic string, payload []byte) {
		data, err := mqttClient.ParseSensorData(payload)
		if err != nil {
			logger.Error("Failed to parse sensor data from MQTT", zap.Error(err))
			return
		}
		if err := sensorCollector.ProcessSensorData(data); err != nil {
			logger.Error("Failed to process sensor data from MQTT", zap.Error(err))
		}
	})
	if err != nil {
		logger.Error("Failed to subscribe to sensor data", zap.Error(err))
	}

	err = mqttClient.Subscribe("plc/status", func(topic string, payload []byte) {
		logger.Debug("Received PLC status", zap.String("payload", string(payload)))
	})
	if err != nil {
		logger.Error("Failed to subscribe to PLC status", zap.Error(err))
	}
}

func metricsLoop(influxClient *influxdb.Client, wsServer *websocket.Server, stopCh <-chan struct{}, logger *zap.Logger) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case now := <-ticker.C:
			flowRate := 1200 + rand.Float64()*200
			powerPerTon := 0.35 + rand.Float64()*0.1
			carbonPerTon := 20 + rand.Float64()*10
			tnRemoval := 0.82 + rand.Float64()*0.06
			tpRemoval := 0.90 + rand.Float64()*0.05
			codRemoval := 0.88 + rand.Float64()*0.06

			powerConsumption := flowRate * powerPerTon
			carbonUsage := flowRate * carbonPerTon / 1000

			metrics := &models.KeyMetrics{
				PowerConsumption: math.Round(powerConsumption*100) / 100,
				CarbonUsage:      math.Round(carbonUsage*100) / 100,
				TNRemovalRate:    math.Round(tnRemoval*10000) / 10000,
				TPRemovalRate:    math.Round(tpRemoval*10000) / 10000,
				CODRemovalRate:   math.Round(codRemoval*10000) / 10000,
				FlowRate:         math.Round(flowRate*100) / 100,
				Timestamp:        now,
			}

			if err := influxClient.WriteKeyMetrics(metrics); err != nil {
				logger.Error("Failed to write key metrics", zap.Error(err))
			}

			if err := wsServer.BroadcastMetrics(metrics); err != nil {
				logger.Error("Failed to broadcast metrics", zap.Error(err))
			}

			logger.Debug("Key metrics updated",
				zap.Float64("power", metrics.PowerConsumption),
				zap.Float64("carbon", metrics.CarbonUsage),
				zap.Float64("tn_removal", metrics.TNRemovalRate))
		}
	}
}
