package config

import (
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

type ServerConfig struct {
	ID  int    `yaml:"id"`
	URL string `yaml:"url"`
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
