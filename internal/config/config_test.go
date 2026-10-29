package config

import (
	"testing"
	"time"
)

func TestLoadPort(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		want    int
		wantErr bool
	}{
		{name: "default", env: "", want: 8080},
		{name: "custom", env: "9090", want: 9090},
		{name: "not a number", env: "http", wantErr: true},
		{name: "zero", env: "0", wantErr: true},
		{name: "too large", env: "70000", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DEVICE_TOKEN", "secret")
			t.Setenv("DATABASE_URL", "postgres://localhost/test")
			t.Setenv("PORT", tt.env)
			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load() = %+v, want error", cfg)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}
			if cfg.Port != tt.want {
				t.Errorf("Port = %d, want %d", cfg.Port, tt.want)
			}
		})
	}
}

func TestLoadDeviceToken(t *testing.T) {
	t.Setenv("PORT", "")

	t.Setenv("DEVICE_TOKEN", "")
	if cfg, err := Load(); err == nil {
		t.Fatalf("Load() = %+v, want error for missing DEVICE_TOKEN", cfg)
	}

	t.Setenv("DEVICE_TOKEN", "secret")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.DeviceToken != "secret" {
		t.Errorf("DeviceToken = %q, want %q", cfg.DeviceToken, "secret")
	}
}

func TestLoadOfflineThreshold(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		want    time.Duration
		wantErr bool
	}{
		{name: "default", env: "", want: 5 * time.Minute},
		{name: "custom", env: "90s", want: 90 * time.Second},
		{name: "not a duration", env: "5", wantErr: true},
		{name: "zero", env: "0s", wantErr: true},
		{name: "negative", env: "-1m", wantErr: true},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("PORT", "")
			t.Setenv("DEVICE_TOKEN", "secret")
			t.Setenv("DATABASE_URL", "postgres://localhost/test")
			t.Setenv("OFFLINE_THRESHOLD", testCase.env)
			cfg, err := Load()
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("Load() = %+v, want error", cfg)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}
			if cfg.OfflineThreshold != testCase.want {
				t.Errorf("OfflineThreshold = %s, want %s", cfg.OfflineThreshold, testCase.want)
			}
		})
	}
}
