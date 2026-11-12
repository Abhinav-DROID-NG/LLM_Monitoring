package config

import (
	"os"
	"testing"
	"time"
)

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("MODEL_MONITOR_ADDR", "")
	t.Setenv("MODEL_MONITOR_PROVIDER_URL", "")
	t.Setenv("MODEL_MONITOR_DATABASE", "")
	t.Setenv("MODEL_MONITOR_HTTP_TIMEOUT", "")
	t.Setenv("MODEL_MONITOR_PROVIDER_RETRIES", "")
	t.Setenv("MODEL_MONITOR_LOG_LEVEL", "")
	for _, name := range []string{
		"MODEL_MONITOR_ADDR",
		"MODEL_MONITOR_PROVIDER_URL",
		"MODEL_MONITOR_DATABASE",
		"MODEL_MONITOR_HTTP_TIMEOUT",
		"MODEL_MONITOR_PROVIDER_RETRIES",
		"MODEL_MONITOR_LOG_LEVEL",
	} {
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.ProviderURL != "http://127.0.0.1:8081" ||
		cfg.DatabasePath != "model-monitor.db" || cfg.HTTPTimeout != 30*time.Second ||
		cfg.ProviderRetries != 2 || cfg.LogLevel != "info" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestFromEnvRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"timeout", "MODEL_MONITOR_HTTP_TIMEOUT", "nope"},
		{"retries", "MODEL_MONITOR_PROVIDER_RETRIES", "-1"},
		{"log level", "MODEL_MONITOR_LOG_LEVEL", "trace"},
		{"empty address", "MODEL_MONITOR_ADDR", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(test.key, test.value)
			if _, err := FromEnv(); err == nil {
				t.Fatalf("expected invalid %s to fail", test.name)
			}
		})
	}
}
