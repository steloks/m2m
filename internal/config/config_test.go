package config

import "testing"

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
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.DeviceToken != "secret" {
		t.Errorf("DeviceToken = %q, want %q", cfg.DeviceToken, "secret")
	}
}
