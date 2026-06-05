package config

import (
	"log"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	InfluxDB InfluxDBConfig `mapstructure:"influxdb"`
	MQTT     MQTTConfig     `mapstructure:"mqtt"`
	Control  ControlConfig  `mapstructure:"control"`
	Alarm    AlarmConfig    `mapstructure:"alarm"`
}

type ServerConfig struct {
	Port string `mapstructure:"port"`
}

type InfluxDBConfig struct {
	Addr     string `mapstructure:"addr"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Database string `mapstructure:"database"`
}

type MQTTConfig struct {
	Broker   string `mapstructure:"broker"`
	ClientID string `mapstructure:"client_id"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Topic    string `mapstructure:"topic"`
	CmdTopic string `mapstructure:"cmd_topic"`
}

type ControlConfig struct {
	Aeration AerationConfig `mapstructure:"aeration"`
	Carbon   CarbonConfig   `mapstructure:"carbon"`
}

type AerationConfig struct {
	NH3TargetMin     float64 `mapstructure:"nh3_target_min"`
	NH3TargetMax     float64 `mapstructure:"nh3_target_max"`
	DOTargetMin      float64 `mapstructure:"do_target_min"`
	DOTargetMax      float64 `mapstructure:"do_target_max"`
	PIDKp            float64 `mapstructure:"pid_kp"`
	PIDKi            float64 `mapstructure:"pid_ki"`
	PIDKd            float64 `mapstructure:"pid_kd"`
	ControlInterval  int     `mapstructure:"control_interval"`
}

type CarbonConfig struct {
	TNTarget         float64 `mapstructure:"tn_target"`
	CODPerCarbonUnit float64 `mapstructure:"cod_per_carbon_unit"`
	ControlInterval  int     `mapstructure:"control_interval"`
}

type AlarmConfig struct {
	Level1 Level1Config `mapstructure:"level1"`
	Level2 Level2Config `mapstructure:"level2"`
	SMS    SMSConfig    `mapstructure:"sms"`
}

type Level1Config struct {
	NH3Threshold    float64 `mapstructure:"nh3_threshold"`
	TNThreshold     float64 `mapstructure:"tn_threshold"`
	DurationMinutes int     `mapstructure:"duration_minutes"`
}

type Level2Config struct {
	OfflineMinutes int `mapstructure:"offline_minutes"`
}

type SMSConfig struct {
	APIKey    string   `mapstructure:"api_key"`
	APISecret string   `mapstructure:"api_secret"`
	Phones    []string `mapstructure:"phones"`
}

var AppConfig *Config

func LoadConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./backend")

	if err := viper.ReadInConfig(); err != nil {
		log.Printf("Warning: config file not found, using defaults: %v", err)
		setDefaults()
	}

	AppConfig = &Config{}
	if err := viper.Unmarshal(AppConfig); err != nil {
		log.Fatalf("Unable to decode config: %v", err)
	}
}

func setDefaults() {
	viper.SetDefault("server.port", "8080")
	viper.SetDefault("influxdb.addr", "http://localhost:8086")
	viper.SetDefault("influxdb.username", "admin")
	viper.SetDefault("influxdb.password", "admin")
	viper.SetDefault("influxdb.database", "sewage")
	viper.SetDefault("mqtt.broker", "tcp://localhost:1883")
	viper.SetDefault("mqtt.client_id", "sewage_backend")
	viper.SetDefault("mqtt.username", "")
	viper.SetDefault("mqtt.password", "")
	viper.SetDefault("mqtt.topic", "sewage/sensor/+")
	viper.SetDefault("mqtt.cmd_topic", "sewage/command")
	viper.SetDefault("control.aeration.nh3_target_min", 1.0)
	viper.SetDefault("control.aeration.nh3_target_max", 2.0)
	viper.SetDefault("control.aeration.do_target_min", 1.5)
	viper.SetDefault("control.aeration.do_target_max", 2.5)
	viper.SetDefault("control.aeration.pid_kp", 0.8)
	viper.SetDefault("control.aeration.pid_ki", 0.1)
	viper.SetDefault("control.aeration.pid_kd", 0.05)
	viper.SetDefault("control.aeration.control_interval", 30)
	viper.SetDefault("control.carbon.tn_target", 15.0)
	viper.SetDefault("control.carbon.cod_per_carbon_unit", 1.06)
	viper.SetDefault("control.carbon.control_interval", 60)
	viper.SetDefault("alarm.level1.nh3_threshold", 5.0)
	viper.SetDefault("alarm.level1.tn_threshold", 15.0)
	viper.SetDefault("alarm.level1.duration_minutes", 30)
	viper.SetDefault("alarm.level2.offline_minutes", 5)
}
