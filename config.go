package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	PrivateKey string `yaml:"private_key"`
	Listen     string `yaml:"listen"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Listen: ":8080",
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
