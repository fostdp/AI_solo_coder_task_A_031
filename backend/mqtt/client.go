package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"sewage-treatment/backend/config"
	"sewage-treatment/backend/models"
	"sewage-treatment/backend/influxdb"
)

type Client struct {
	client mqtt.Client
}

var MQTTClient *Client
var sensorDataChan chan *models.SensorData

func NewClient() error {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(config.AppConfig.MQTT.Broker)
	opts.SetClientID(config.AppConfig.MQTT.ClientID)
	if config.AppConfig.MQTT.Username != "" {
		opts.SetUsername(config.AppConfig.MQTT.Username)
		opts.SetPassword(config.AppConfig.MQTT.Password)
	}
	opts.SetAutoReconnect(true)
	opts.SetReconnectingHandler(func(c mqtt.Client, opts *mqtt.ClientOptions) {
		log.Println("Reconnecting to MQTT broker...")
	})
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Println("Connected to MQTT broker")
		subscribeTopics(c)
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to connect to MQTT broker: %v", token.Error())
	}

	MQTTClient = &Client{
		client: client,
	}
	sensorDataChan = make(chan *models.SensorData, 1000)

	go processSensorData()

	log.Println("MQTT client initialized successfully")
	return nil
}

func subscribeTopics(client mqtt.Client) {
	topic := config.AppConfig.MQTT.Topic
	token := client.Subscribe(topic, 1, handleSensorData)
	if token.Wait() && token.Error() != nil {
		log.Printf("Failed to subscribe to topic %s: %v", topic, token.Error())
	} else {
		log.Printf("Subscribed to topic: %s", topic)
	}

	cmdResponseTopic := config.AppConfig.MQTT.CmdTopic + "/response"
	token = client.Subscribe(cmdResponseTopic, 1, handleCommandResponse)
	if token.Wait() && token.Error() != nil {
		log.Printf("Failed to subscribe to topic %s: %v", cmdResponseTopic, token.Error())
	} else {
		log.Printf("Subscribed to topic: %s", cmdResponseTopic)
	}
}

func handleSensorData(client mqtt.Client, msg mqtt.Message) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic in handleSensorData: %v", r)
		}
	}()

	var data models.SensorData
	if err := json.Unmarshal(msg.Payload(), &data); err != nil {
		log.Printf("Failed to parse sensor data: %v, payload: %s", err, string(msg.Payload()))
		return
	}

	data.Timestamp = time.Now()
	data.Status = "online"

	calcDeviationAndAlarm(&data)

	select {
	case sensorDataChan <- &data:
	default:
		log.Printf("Sensor data channel full, dropping data for sensor %s", data.ID)
	}
}

func calcDeviationAndAlarm(data *models.SensorData) {
	cfg := models.GetSensorConfig(data.ID)
	if cfg == nil {
		data.AlarmLevel = 0
		return
	}

	target := (cfg.TargetMin + cfg.TargetMax) / 2
	rangeVal := cfg.TargetMax - cfg.TargetMin
	if rangeVal == 0 {
		rangeVal = 1
	}

	deviation := 0.0
	if data.Value > target {
		deviation = (data.Value - target) / rangeVal
	} else {
		deviation = (target - data.Value) / rangeVal
	}

	deviationPercent := deviation * 100

	if deviationPercent > 20 {
		data.AlarmLevel = 2
	} else if deviationPercent > 10 {
		data.AlarmLevel = 1
	} else {
		data.AlarmLevel = 0
	}
}

func processSensorData() {
	for data := range sensorDataChan {
		if err := influxdb.InfluxClient.WriteSensorData(data); err != nil {
			log.Printf("Failed to write sensor data to InfluxDB: %v", err)
		}
	}
}

func handleCommandResponse(client mqtt.Client, msg mqtt.Message) {
	log.Printf("Received command response: %s", string(msg.Payload()))
}

func PublishCommand(cmd *models.ControlCommand) error {
	if MQTTClient == nil || MQTTClient.client == nil {
		return fmt.Errorf("MQTT client not initialized")
	}

	payload, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %v", err)
	}

	topic := fmt.Sprintf("%s/%s", config.AppConfig.MQTT.CmdTopic, cmd.Target)
	token := MQTTClient.client.Publish(topic, 1, false, payload)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish command: %v", token.Error())
	}

	if err := influxdb.InfluxClient.WriteControlCommand(cmd); err != nil {
		log.Printf("Failed to write control command to InfluxDB: %v", err)
	}

	log.Printf("Published command to topic %s: %+v", topic, cmd)
	return nil
}

func GetSensorDataChannel() <-chan *models.SensorData {
	return sensorDataChan
}

func Disconnect() {
	if MQTTClient != nil && MQTTClient.client != nil {
		MQTTClient.client.Disconnect(250)
	}
	if sensorDataChan != nil {
		close(sensorDataChan)
	}
}
