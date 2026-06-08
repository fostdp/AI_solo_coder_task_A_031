package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	InfluxDB   InfluxDBConfig   `mapstructure:"influxdb"`
	MQTT       MQTTConfig       `mapstructure:"mqtt"`
	WebSocket  WebSocketConfig  `mapstructure:"websocket"`
	Collector  CollectorConfig  `mapstructure:"collector"`
	Controller ControllerConfig `mapstructure:"controller"`
	Alarm      AlarmConfig      `mapstructure:"alarm"`
	DTU        DTUConfig        `mapstructure:"dtu"`
}

type CollectorConfig struct {
	ValidationEnabled       bool    `mapstructure:"validation_enabled"`
	JumpDetectionEnabled    bool    `mapstructure:"jump_detection_enabled"`
	MaxJumpPercent          float64 `mapstructure:"max_jump_percent"`
	OfflineDetectionEnabled bool    `mapstructure:"offline_detection_enabled"`
	OfflineThresholdMinutes int     `mapstructure:"offline_threshold_minutes"`
	ChannelBufferSize       int     `mapstructure:"channel_buffer_size"`
}

type PIDConfig struct {
	Kp                       float64 `mapstructure:"kp"`
	Ki                       float64 `mapstructure:"ki"`
	Kd                       float64 `mapstructure:"kd"`
	IntegralSeparationPercent float64 `mapstructure:"integral_separation_percent"`
	OutputMin                float64 `mapstructure:"output_min"`
	OutputMax                float64 `mapstructure:"output_max"`
}

type ServerConfig struct {
	Port        int        `mapstructure:"port"`
	Mode        string     `mapstructure:"mode"`
	Pprof       PprofConfig `mapstructure:"pprof"`
}

type PprofConfig struct {
	Enabled bool `mapstructure:"enabled"`
	Port    int  `mapstructure:"port"`
}

type InfluxDBConfig struct {
	Addr            string `mapstructure:"addr"`
	Username        string `mapstructure:"username"`
	Password        string `mapstructure:"password"`
	Database        string `mapstructure:"database"`
	RetentionPolicy string `mapstructure:"retention_policy"`
	Precision       string `mapstructure:"precision"`
}

type MQTTConfig struct {
	Broker      string `mapstructure:"broker"`
	ClientID    string `mapstructure:"client_id"`
	Username    string `mapstructure:"username"`
	Password    string `mapstructure:"password"`
	TopicPrefix string `mapstructure:"topic_prefix"`
	QoS         byte   `mapstructure:"qos"`
}

type WebSocketConfig struct {
	ReadBufferSize  int `mapstructure:"read_buffer_size"`
	WriteBufferSize int `mapstructure:"write_buffer_size"`
	PingPeriod      int `mapstructure:"ping_period"`
	PongWait        int `mapstructure:"pong_wait"`
	WriteWait       int `mapstructure:"write_wait"`
}

type ControllerConfig struct {
	Aeration AerationConfig `mapstructure:"aeration"`
	Carbon   CarbonConfig   `mapstructure:"carbon"`
}

type AerationConfig struct {
	DOSetpoint        float64   `mapstructure:"do_setpoint"`
	DOMin             float64   `mapstructure:"do_min"`
	DOMax             float64   `mapstructure:"do_max"`
	NH3Setpoint       float64   `mapstructure:"nh3_setpoint"`
	NH3Min            float64   `mapstructure:"nh3_min"`
	NH3Max            float64   `mapstructure:"nh3_max"`
	PID               PIDConfig `mapstructure:"pid"`
	FeedforwardGain   float64   `mapstructure:"feedforward_gain"`
	ControlInterval   int       `mapstructure:"control_interval"`
	MinComputeInterval int      `mapstructure:"min_compute_interval"`
	EventDriven       bool      `mapstructure:"event_driven"`
	NumSections       int       `mapstructure:"num_sections"`
	MinAirFlow        float64   `mapstructure:"min_air_flow"`
	MaxAirFlow        float64   `mapstructure:"max_air_flow"`
	MinValveOpen      float64   `mapstructure:"min_valve_open"`
	MaxValveOpen      float64   `mapstructure:"max_valve_open"`
}

type CarbonConfig struct {
	TNRemovalTarget        float64 `mapstructure:"tn_removal_target"`
	CODNRatio              float64 `mapstructure:"cod_n_ratio"`
	DosingMax              float64 `mapstructure:"dosing_max"`
	ControlInterval        int     `mapstructure:"control_interval"`
	MinComputeInterval     int     `mapstructure:"min_compute_interval"`
	CODChangeThresholdPct  float64 `mapstructure:"cod_change_threshold_pct"`
	BioavailableCODRatio   float64 `mapstructure:"bioavailable_cod_ratio"`
	DenitrificationEff     float64 `mapstructure:"denitrification_eff"`
	NaAcCODEquivalent      float64 `mapstructure:"naac_cod_equivalent"`
}

type AlarmConfig struct {
	Level1 Level1AlarmConfig `mapstructure:"level1"`
	Level2 Level2AlarmConfig `mapstructure:"level2"`
	SMS    SMSConfig         `mapstructure:"sms"`
}

type Level1AlarmConfig struct {
	NH3Threshold     float64 `mapstructure:"nh3_threshold"`
	TNThreshold      float64 `mapstructure:"tn_threshold"`
	DurationMinutes  int     `mapstructure:"duration_minutes"`
	CheckInterval    int     `mapstructure:"check_interval"`
}

type Level2AlarmConfig struct {
	FanFailureCheck      bool `mapstructure:"fan_failure_check"`
	SensorOfflineMinutes int  `mapstructure:"sensor_offline_minutes"`
}

type SMSConfig struct {
	Enabled    bool     `mapstructure:"enabled"`
	APIURL     string   `mapstructure:"api_url"`
	APIKey     string   `mapstructure:"api_key"`
	Recipients []string `mapstructure:"recipients"`
}

type DTUConfig struct {
	ReportIntervalSeconds int            `mapstructure:"report_interval_seconds"`
	Sensors               DTUSensorCount `mapstructure:"sensors"`
}

type DTUSensorCount struct {
	DO  int `mapstructure:"do"`
	NH3 int `mapstructure:"nh3"`
	NO3 int `mapstructure:"no3"`
	PO4 int `mapstructure:"po4"`
}

var AppConfig *Config

func Load(path string) error {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	AppConfig = &Config{}
	if err := v.Unmarshal(AppConfig); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return nil
}
