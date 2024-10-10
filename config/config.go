package config

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

type ServerConfig struct {
	ID      int           `yaml:"id" json:"id"`
	URL     string        `yaml:"url" json:"url"`
	Monitor MonitorConfig `yaml:"monitor" json:"monitor"`
}

type MonitorConfig struct {
	Threshold                      float64 `yaml:"threshold" json:"threshold"`
	MaxSampleSize                  int     `yaml:"maxSampleSize" json:"maxSampleSize"`
	MinStdDeviationMillis          float64 `yaml:"minStdDeviationMillis" json:"minStdDeviationMillis"`
	AcceptableHeartbeatPauseMillis int64   `yaml:"acceptableHeartbeatPauseMillis" json:"acceptableHeartbeatPauseMillis"`
	FirstHeartbeatEstimateMillis   int64   `yaml:"firstHeartbeatEstimateMillis" json:"firstHeartbeatEstimateMillis"`
}

type Config struct {
	Servers []ServerConfig `yaml:"servers" json:"servers"`
}

func (c *Config) Encode() ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(c); err != nil {
		return nil, fmt.Errorf("failed to encode config: %v", err)
	}
	return buf.Bytes(), nil
}

func DecodeConfig(data []byte) (*Config, error) {
	var cfg Config
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config: %v", err)
	}
	return &cfg, nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for ServerConfig
func (s *ServerConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawServerConfig ServerConfig
	raw := rawServerConfig{
		Monitor: MonitorConfig{
			Threshold:                      16.0,
			MaxSampleSize:                  200,
			MinStdDeviationMillis:          500,
			AcceptableHeartbeatPauseMillis: 0,
			FirstHeartbeatEstimateMillis:   500,
		},
	}

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*s = ServerConfig(raw)
	return nil
}

func Load(filename string) (*Config, error) {
	var config Config
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return &config, err
	}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return &config, err
	}

	// Print the values for all servers' monitor configs
	for i, server := range config.Servers {
		fmt.Printf("Server %d:\n", server.ID)
		fmt.Printf("  Threshold: %f\n", server.Monitor.Threshold)
		fmt.Printf("  MaxSampleSize: %d\n", server.Monitor.MaxSampleSize)
		fmt.Printf("  MinStdDeviationMillis: %f\n", server.Monitor.MinStdDeviationMillis)
		fmt.Printf("  AcceptableHeartbeatPauseMillis: %d\n", server.Monitor.AcceptableHeartbeatPauseMillis)
		fmt.Printf("  FirstHeartbeatEstimateMillis: %d\n", server.Monitor.FirstHeartbeatEstimateMillis)
		if i < len(config.Servers)-1 {
			fmt.Println()
		}
	}

	return &config, nil
}

func init() {
	gob.Register(Config{})
	gob.Register(ServerConfig{})
	gob.Register(MonitorConfig{})
}
