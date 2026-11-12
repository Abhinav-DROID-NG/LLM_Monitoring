package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr        string
	ProviderURL     string
	DatabasePath    string
	HTTPTimeout     time.Duration
	ProviderRetries int
	LogLevel        string
}

func FromEnv() (Config, error) {
	cfg := Config{
		HTTPAddr:        envString("MODEL_MONITOR_ADDR", ":8080"),
		ProviderURL:     envString("MODEL_MONITOR_PROVIDER_URL", "http://127.0.0.1:8081"),
		DatabasePath:    envString("MODEL_MONITOR_DATABASE", "model-monitor.db"),
		HTTPTimeout:     30 * time.Second,
		ProviderRetries: 2,
		LogLevel:        envString("MODEL_MONITOR_LOG_LEVEL", "info"),
	}

	if value, ok := os.LookupEnv("MODEL_MONITOR_HTTP_TIMEOUT"); ok {
		timeout, err := time.ParseDuration(value)
		if err != nil || timeout <= 0 {
			return Config{}, fmt.Errorf("MODEL_MONITOR_HTTP_TIMEOUT must be a positive duration: %q", value)
		}
		cfg.HTTPTimeout = timeout
	}
	if value, ok := os.LookupEnv("MODEL_MONITOR_PROVIDER_RETRIES"); ok {
		retries, err := strconv.Atoi(value)
		if err != nil || retries < 0 {
			return Config{}, fmt.Errorf("MODEL_MONITOR_PROVIDER_RETRIES must be a non-negative integer: %q", value)
		}
		cfg.ProviderRetries = retries
	}
	if cfg.HTTPAddr == "" || cfg.ProviderURL == "" || cfg.DatabasePath == "" {
		return Config{}, errors.New("MODEL_MONITOR_ADDR, MODEL_MONITOR_PROVIDER_URL, and MODEL_MONITOR_DATABASE cannot be empty")
	}
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, fmt.Errorf("MODEL_MONITOR_LOG_LEVEL must be debug, info, warn, or error: %q", cfg.LogLevel)
	}
	return cfg, nil
}

func envString(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return fallback
}
