package config

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"
)

type Config struct {
	Redis RedisConfig `yaml:"redis" json:"redis"`
}

type RedisConfig struct {
	Url      string `yaml:"url" json:"url"`
	Port     int    `yaml:"port" json:"port"`
	Password string `json:"password" yaml:"password"`
	DB       int    `json:"db" yaml:"db"`
	Protocol int    `json:"protocol" yaml:"protocol"`
}

var cfg *Config = nil

func LoadConfig(path string) (*Config, error) {
	if cfg != nil {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err.Error())
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err.Error())
	}
	return cfg, nil
}
