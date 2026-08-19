package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"ruralhealth/internal/logging"
)

type Config struct {
	Server     ServerConfig     `yaml:"server"`
	DataDir    string           `yaml:"data_dir" env:"CERT_DATA_DIR"`
	Gateway    GatewayConfig    `yaml:"gateway"`
	Scheduler  SchedulerConfig  `yaml:"scheduler"`
	Callback   CallbackConfig   `yaml:"callback"`
	Logging    logging.Config   `yaml:"logging"`
	Upstreams  []UpstreamConfig `yaml:"upstreams"`
	SigningKey string           `yaml:"signing_key" env:"CERT_SIGNING_KEY"`
}

type ServerConfig struct {
	HTTPAddr        string        `yaml:"http_addr" env:"CERT_HTTP_ADDR"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

type GatewayConfig struct {
	DefaultTimeout    time.Duration `yaml:"default_timeout"`
	MaxRetries        int           `yaml:"max_retries"`
	BaseBackoff       time.Duration `yaml:"base_backoff"`
	MaxBackoff        time.Duration `yaml:"max_backoff"`
	CircuitMaxFail    int           `yaml:"circuit_max_fail"`
	CircuitResetAfter time.Duration `yaml:"circuit_reset_after"`
}

type SchedulerConfig struct {
	TickInterval     time.Duration `yaml:"tick_interval"`
	DispatchInterval time.Duration `yaml:"dispatch_interval"`
	CallbackInterval time.Duration `yaml:"callback_interval"`
	ConsistencyCheck time.Duration `yaml:"consistency_check"`
	MaxRetries       int           `yaml:"max_retries"`
	DeadLetterAfter  int           `yaml:"dead_letter_after"`
}

type CallbackConfig struct {
	MaxAttempts int           `yaml:"max_attempts"`
	BaseBackoff time.Duration `yaml:"base_backoff"`
	MaxBackoff  time.Duration `yaml:"max_backoff"`
	Timeout     time.Duration `yaml:"timeout"`
}

type UpstreamConfig struct {
	ID        string        `yaml:"id"`
	Name      string        `yaml:"name"`
	BaseURL   string        `yaml:"base_url"`
	Timeout   time.Duration `yaml:"timeout"`
	Priority  int           `yaml:"priority"`
	Standards []string      `yaml:"standards"`
}

func Default() Config {
	return Config{
		Server: ServerConfig{
			HTTPAddr:        ":52661",
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			ShutdownTimeout: 15 * time.Second,
		},
		DataDir: "./data",
		Gateway: GatewayConfig{
			DefaultTimeout:    10 * time.Second,
			MaxRetries:        3,
			BaseBackoff:       500 * time.Millisecond,
			MaxBackoff:        10 * time.Second,
			CircuitMaxFail:    5,
			CircuitResetAfter: 30 * time.Second,
		},
		Scheduler: SchedulerConfig{
			TickInterval:     5 * time.Second,
			DispatchInterval: 3 * time.Second,
			CallbackInterval: 4 * time.Second,
			ConsistencyCheck: 10 * time.Second,
			MaxRetries:       5,
			DeadLetterAfter:  5,
		},
		Callback: CallbackConfig{
			MaxAttempts: 5,
			BaseBackoff: 1 * time.Second,
			MaxBackoff:  60 * time.Second,
			Timeout:     10 * time.Second,
		},
		Logging: logging.Config{
			Level:  "info",
			Format: "json",
		},
		SigningKey: "default-dev-key-change-in-prod",
		Upstreams: []UpstreamConfig{
			{
				ID:       "upstream-primary",
				Name:     "Primary Testing Center",
				BaseURL:  "http://127.0.0.1:52662",
				Timeout:  10 * time.Second,
				Priority: 1,
			},
			{
				ID:       "upstream-backup",
				Name:     "Backup Testing Center",
				BaseURL:  "http://127.0.0.1:52663",
				Timeout:  10 * time.Second,
				Priority: 2,
			},
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			applyEnv(&cfg)
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	applyEnv(&cfg)
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func applyEnv(cfg *Config) {
	overlayStr(&cfg.DataDir, "CERT_DATA_DIR")
	overlayStr(&cfg.SigningKey, "CERT_SIGNING_KEY")
	overlayStr(&cfg.Server.HTTPAddr, "CERT_HTTP_ADDR")
	overlayStr(&cfg.Logging.Level, "LOG_LEVEL")
	overlayStr(&cfg.Logging.Format, "LOG_FORMAT")
	overlayDuration(&cfg.Server.ReadTimeout, "CERT_READ_TIMEOUT")
	overlayDuration(&cfg.Server.WriteTimeout, "CERT_WRITE_TIMEOUT")
}

func overlayStr(target *string, env string) {
	if v := os.Getenv(env); v != "" {
		*target = v
	}
}

func overlayDuration(target *time.Duration, env string) {
	if v := os.Getenv(env); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			*target = d
		}
	}
}

func overlayInt(target *int, env string) {
	if v := os.Getenv(env); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*target = n
		}
	}
}

func (c Config) Validate() error {
	var problems []string
	if c.DataDir == "" {
		problems = append(problems, "data_dir must not be empty")
	}
	if c.Server.HTTPAddr == "" {
		problems = append(problems, "http_addr must not be empty")
	}
	if c.SigningKey == "" {
		problems = append(problems, "signing_key must not be empty")
	}
	if len(c.Upstreams) < 2 {
		problems = append(problems, "at least 2 upstreams required for failover")
	}
	if c.Gateway.MaxRetries < 0 {
		problems = append(problems, "max_retries must be non-negative")
	}
	if c.Callback.MaxAttempts < 1 {
		problems = append(problems, "callback max_attempts must be >= 1")
	}
	if len(problems) > 0 {
		return fmt.Errorf("config validation failed: %s", strings.Join(problems, "; "))
	}
	return nil
}
