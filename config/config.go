package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	LogLevel      string `json:"log_level"`
	RelayCallsign string `json:"relay_callsign"`
	BindAddress   string `json:"bind_address"`
}

func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	config := &Config{}
	err = decoder.Decode(config)
	if err != nil {
		return nil, err
	}

	return config, nil
}
