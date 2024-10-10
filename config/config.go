package config

import (
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

type ServerConfig struct {
	// server related
	ID  int    `yaml:"id"`
	URL string `yaml:"url"`
	// for phi detector
	Threshold                      float64 `yaml:"threshold" default:"16.0"`
	MaxSampleSize                  int     `yaml:"maxSampleSize" default:"200"`
	MinStdDeviationMillis          float64 `yaml:"minStdDeviationMillis" default:"500"`
	AcceptableHeartbeatPauseMillis int64   `yaml:"acceptableHeartbeatPauseMillis" default:"0"`
	FirstHeartbeatEstimateMillis   int64   `yaml:"firstHeartBeatEstimateMillis" default:"500"`
}

type Config struct {
	Servers []ServerConfig `yaml:"servers"`
}

func Load(filename string) (Config, error) {
	var config Config
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return config, err
	}
	err = yaml.Unmarshal(data, &config)
	return config, err
}
