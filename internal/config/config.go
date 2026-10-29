package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port             int
	DeviceToken      string
	DatabaseURL      string
	OfflineThreshold time.Duration
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

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	offlineThreshold, err := durationEnv("OFFLINE_THRESHOLD", 5*time.Minute)
	if err != nil {
		return Config{}, err
	}
	if offlineThreshold <= 0 {
		return Config{}, fmt.Errorf("OFFLINE_THRESHOLD must be positive, got %s", offlineThreshold)
	}

	return Config{Port: port, DeviceToken: token, DatabaseURL: dbURL, OfflineThreshold: offlineThreshold}, nil
}

func durationEnv(key string, def time.Duration) (time.Duration, error) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return def, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration such as 5m or 90s, got %q", key, value)
	}
	return duration, nil
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
