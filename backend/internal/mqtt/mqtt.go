package mqtt

import (
	"encoding/json"
	"fmt"
	"time"

	mqttclient "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"

	"sewage-treatment-system/internal/config"
	"sewage-treatment-system/internal/models"
)

type Client struct {
	client mqttclient.Client
	logger *zap.Logger
	cfg    *config.MQTTConfig
}

type MessageHandler func(topic string, payload []byte)

func New(cfg *config.MQTTConfig, logger *zap.Logger) (*Client, error) {
	opts := mqttclient.NewClientOptions()
	opts.AddBroker(cfg.Broker)
	opts.SetClientID(cfg.ClientID)
	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
		opts.SetPassword(cfg.Password)
	}
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetReconnectingHandler(func(c mqttclient.Client, opts *mqttclient.ClientOptions) {
		logger.Info("MQTT reconnecting...")
	})
	opts.SetOnConnectHandler(func(c mqttclient.Client) {
		logger.Info("MQTT connected")
	})
	opts.SetConnectionLostHandler(func(c mqttclient.Client, err error) {
		logger.Error("MQTT connection lost", zap.Error(err))
	})
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(10 * time.Second)
	opts.SetMaxReconnectInterval(5 * time.Second)

	c := mqttclient.NewClient(opts)
	token := c.Connect()
	if token.Wait() && token.Error() != nil {
		return nil, fmt.Errorf("failed to connect MQTT broker: %w", token.Error())
	}

	return &Client{
		client: c,
		logger: logger,
		cfg:    cfg,
	}, nil
}

func (m *Client) Close() {
	if m.client != nil && m.client.IsConnected() {
		m.client.Disconnect(250)
	}
}

func (m *Client) Subscribe(topic string, handler MessageHandler) error {
	fullTopic := m.cfg.TopicPrefix + topic
	token := m.client.Subscribe(fullTopic, m.cfg.QoS, func(c mqttclient.Client, msg mqttclient.Message) {
		handler(msg.Topic(), msg.Payload())
	})
	token.Wait()
	if token.Error() != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", fullTopic, token.Error())
	}
	m.logger.Info("Subscribed to MQTT topic", zap.String("topic", fullTopic))
	return nil
}

func (m *Client) Publish(topic string, payload interface{}) error {
	fullTopic := m.cfg.TopicPrefix + topic
	var data []byte
	var err error

	switch v := payload.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		data, err = json.Marshal(v)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}
	}

	token := m.client.Publish(fullTopic, m.cfg.QoS, false, data)
	token.Wait()
	if token.Error() != nil {
		return fmt.Errorf("failed to publish to %s: %w", fullTopic, token.Error())
	}

	m.logger.Debug("MQTT message published",
		zap.String("topic", fullTopic),
		zap.Int("bytes", len(data)))
	return nil
}

func (m *Client) PublishAerationCommand(section int, airFlow, valveOpen float64) error {
	cmd := map[string]interface{}{
		"device_type": "aeration",
		"device_id":   fmt.Sprintf("aeration_%d", section),
		"command":     "set_air_flow",
		"params": map[string]interface{}{
			"section":    section,
			"air_flow":   airFlow,
			"valve_open": valveOpen,
		},
		"timestamp": time.Now(),
	}
	return m.Publish(fmt.Sprintf("control/aeration/section_%d", section), cmd)
}

func (m *Client) PublishCarbonCommand(dosingRate float64) error {
	cmd := map[string]interface{}{
		"device_type": "carbon_dosing",
		"device_id":   "carbon_dosing_1",
		"command":     "set_dosing",
		"params": map[string]interface{}{
			"dosing_rate": dosingRate,
		},
		"timestamp": time.Now(),
	}
	return m.Publish("control/carbon", cmd)
}

func (m *Client) PublishAlarm(alarm *models.Alarm) error {
	return m.Publish("alarm", alarm)
}

func (m *Client) ParseSensorData(payload []byte) (*models.SensorData, error) {
	var data models.SensorData
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("failed to parse sensor data: %w", err)
	}
	return &data, nil
}

func (m *Client) ParseControlCommand(payload []byte) (*models.ControlCommand, error) {
	var cmd models.ControlCommand
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return nil, fmt.Errorf("failed to parse control command: %w", err)
	}
	return &cmd, nil
}
