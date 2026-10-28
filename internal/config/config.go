package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port        int
	DeviceToken string
}

func Load() (Config, error) {
	port, err := intEnv("PORT", 8080)
	if err != nil {
		return Config{}, err
	}
	if port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("PORT must be between 1 and 65535, got %d", port)
	}

	token := os.Getenv("DEVICE_TOKEN")
	if token == "" {
		return Config{}, errors.New("DEVICE_TOKEN is required")
	}

	return Config{Port: port, DeviceToken: token}, nil
}

func intEnv(key string, def int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got %q", key, v)
	}
	return n, nil
}
