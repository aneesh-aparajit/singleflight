package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Redis RedisConfig `yaml:"redis" json:"redis"`
}

type RedisConfig struct {
	Url      string `json:"url" yaml:"url"`
	Port     int    `json:"port" yaml:"port"`
	Password string `json:"password" yaml:"password"`
	DB       int    `json:"db" yaml:"db"`
	Protocol int    `json:"protocol" yaml:"protocol"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}

	return &cfg, nil
}
