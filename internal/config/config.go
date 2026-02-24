package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Port        int    `yaml:"port"`
	JWTSecret   string `yaml:"jwt_secret"`
	AccessToken string `yaml:"access_token"`
	DBPath      string `yaml:"db_path"`
}

var C Config

func Load() error {
	C = Config{
		Port:        8080,
		JWTSecret:   "hapi-lite-secret",
		AccessToken: "hapi-lite-token",
		DBPath:      "hapi-lite.db",
	}

	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return nil // use defaults
	}
	return yaml.Unmarshal(data, &C)
}
