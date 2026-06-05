package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"sewage-plant-system/pkg/models"
)

type Client struct {
	client     mqtt.Client
	broker     string
	clientID   string
	username   string
	password   string
	topics     map[string]byte
	connected  bool
	mu         sync.RWMutex
	handlers   map[string]func(topic string, payload []byte)
}

func New(broker, clientID, username, password string) *Client {
	return &Client{
		broker:   broker,
		clientID: clientID,
		username: username,
		password: password,
		topics:   make(map[string]byte),
		handlers: make(map[string]func(topic string, payload []byte)),
	}
}

func (c *Client) Connect() error {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(c.broker)
	opts.SetClientID(c.clientID)
	opts.SetUsername(c.username)
	opts.SetPassword(c.password)
	opts.SetAutoReconnect(true)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(10 * time.Second)
	opts.SetMaxReconnectInterval(30 * time.Second)

	opts.OnConnect = func(client mqtt.Client) {
		log.Printf("MQTT connected to %s", c.broker)
		c.mu.Lock()
		c.connected = true
		c.mu.Unlock()

		for topic := range c.topics {
			if err := c.Subscribe(topic, c.topics[topic]); err != nil {
				log.Printf("Failed to resubscribe to %s: %v", topic, err)
			}
		}
	}

	opts.OnConnectionLost = func(client mqtt.Client, err error) {
		log.Printf("MQTT connection lost: %v", err)
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
	}

	c.client = mqtt.NewClient(opts)
	token := c.client.Connect()
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("mqtt connect failed: %w", token.Error())
	}

	return nil
}

func (c *Client) Disconnect() {
	if c.client != nil && c.IsConnected() {
		c.client.Disconnect(250)
	}
	c.mu.Lock()
	c.connected = false
	c.mu.Unlock()
}

func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

func (c *Client) Subscribe(topic string, qos byte) error {
	c.mu.Lock()
	c.topics[topic] = qos
	c.mu.Unlock()

	handler := func(client mqtt.Client, msg mqtt.Message) {
		c.mu.RLock()
		handler, exists := c.handlers[topic]
		c.mu.RUnlock()

		if exists {
			handler(msg.Topic(), msg.Payload())
		} else {
			log.Printf("Received message on %s: %s", msg.Topic(), string(msg.Payload()))
		}
	}

	token := c.client.Subscribe(topic, qos, handler)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("subscribe to %s failed: %w", topic, token.Error())
	}

	log.Printf("Subscribed to topic: %s", topic)
	return nil
}

func (c *Client) SetHandler(topic string, handler func(topic string, payload []byte)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[topic] = handler
}

func (c *Client) Publish(topic string, qos byte, retained bool, payload interface{}) error {
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
			return fmt.Errorf("marshal payload failed: %w", err)
		}
	}

	token := c.client.Publish(topic, qos, retained, data)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("publish to %s failed: %w", topic, token.Error())
	}

	return nil
}

func (c *Client) PublishSensorData(data *models.SensorData) error {
	topic := fmt.Sprintf("sensor/%s/data", data.SensorID)
	return c.Publish(topic, 1, false, data)
}

func (c *Client) PublishControlCommand(cmd *models.ControlCommand) error {
	topic := fmt.Sprintf("control/%s/%s", cmd.TargetType, cmd.TargetID)
	return c.Publish(topic, 1, false, cmd)
}

func (c *Client) PublishPLCStatus(status *models.PLCStatus) error {
	topic := fmt.Sprintf("plc/%s/status", status.PLCID)
	return c.Publish(topic, 1, false, status)
}

func (c *Client) ParseSensorData(payload []byte) (*models.SensorData, error) {
	var data models.SensorData
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("parse sensor data failed: %w", err)
	}
	return &data, nil
}

func (c *Client) ParsePLCStatus(payload []byte) (*models.PLCStatus, error) {
	var status models.PLCStatus
	if err := json.Unmarshal(payload, &status); err != nil {
		return nil, fmt.Errorf("parse plc status failed: %w", err)
	}
	return &status, nil
}

func (c *Client) ParseControlCommand(payload []byte) (*models.ControlCommand, error) {
	var cmd models.ControlCommand
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return nil, fmt.Errorf("parse control command failed: %w", err)
	}
	return &cmd, nil
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
