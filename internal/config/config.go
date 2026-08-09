package config

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/core"
)

type Config struct {
	Environment string
	HTTP        HTTPConfig
	PostgreSQL  PostgreSQLConfig
	Valkey      ValkeyConfig
	ObjectStore ObjectStoreConfig
	Desktop     DesktopConfig
	Providers   ProviderConfig
}

type HTTPConfig struct {
	Address          string
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	IdleTimeout      time.Duration
	ShutdownTimeout  time.Duration
	MaximumBodyBytes int64
}

type PostgreSQLConfig struct{ URL string }

type ValkeyConfig struct {
	Address  string
	Username string
	Password string
}

type ObjectStoreConfig struct {
	Endpoint string
	Bucket   string
	Region   string
}

type DesktopConfig struct {
	IPCName        string
	StartMinimized bool
	SettingsPath   string
}

type ProviderConfig struct {
	OpenAIAPIKey     string
	AnthropicAPIKey  string
	GoogleAPIKey     string
	DeepSeekAPIKey   string
	OpenRouterAPIKey string
}

// Load reads process configuration from environment variables. It intentionally
// does not parse the example YAML file: production deployments should render
// environment variables or build a separate config-source adapter rather than
// let every binary own a subtly different YAML parser.
func Load(ctx context.Context) (Config, error) {
	if err := ctx.Err(); err != nil {
		return Config{}, err
	}

	cfg := Config{
		Environment: envString("ARG_ENVIRONMENT", "development"),
		HTTP: HTTPConfig{
			Address:          envString("ARG_HTTP_ADDRESS", ":8080"),
			ReadTimeout:      15 * time.Second,
			WriteTimeout:     0,
			IdleTimeout:      90 * time.Second,
			ShutdownTimeout:  15 * time.Second,
			MaximumBodyBytes: 8 << 20,
		},
		PostgreSQL: PostgreSQLConfig{URL: strings.TrimSpace(os.Getenv("ARG_POSTGRES_URL"))},
		Valkey: ValkeyConfig{
			Address:  envString("ARG_VALKEY_ADDRESS", "127.0.0.1:6379"),
			Username: strings.TrimSpace(os.Getenv("ARG_VALKEY_USERNAME")),
			Password: os.Getenv("ARG_VALKEY_PASSWORD"),
		},
		ObjectStore: ObjectStoreConfig{
			Endpoint: strings.TrimSpace(os.Getenv("ARG_OBJECT_STORE_ENDPOINT")),
			Bucket:   strings.TrimSpace(os.Getenv("ARG_OBJECT_STORE_BUCKET")),
			Region:   strings.TrimSpace(os.Getenv("ARG_OBJECT_STORE_REGION")),
		},
		Desktop: DesktopConfig{
			IPCName:      envString("ARG_DESKTOP_IPC_NAME", "relaydock"),
			SettingsPath: strings.TrimSpace(os.Getenv("ARG_DESKTOP_SETTINGS_PATH")),
		},
		Providers: ProviderConfig{
			OpenAIAPIKey:     os.Getenv("OPENAI_API_KEY"),
			AnthropicAPIKey:  os.Getenv("ANTHROPIC_API_KEY"),
			GoogleAPIKey:     os.Getenv("GOOGLE_API_KEY"),
			DeepSeekAPIKey:   os.Getenv("DEEPSEEK_API_KEY"),
			OpenRouterAPIKey: os.Getenv("OPENROUTER_API_KEY"),
		},
	}

	var err error
	if cfg.HTTP.ReadTimeout, err = envDuration("ARG_HTTP_READ_TIMEOUT", cfg.HTTP.ReadTimeout); err != nil {
		return Config{}, err
	}
	if cfg.HTTP.WriteTimeout, err = envDuration("ARG_HTTP_WRITE_TIMEOUT", cfg.HTTP.WriteTimeout); err != nil {
		return Config{}, err
	}
	if cfg.HTTP.IdleTimeout, err = envDuration("ARG_HTTP_IDLE_TIMEOUT", cfg.HTTP.IdleTimeout); err != nil {
		return Config{}, err
	}
	if cfg.HTTP.ShutdownTimeout, err = envDuration("ARG_HTTP_SHUTDOWN_TIMEOUT", cfg.HTTP.ShutdownTimeout); err != nil {
		return Config{}, err
	}
	if cfg.HTTP.MaximumBodyBytes, err = envInt64("ARG_HTTP_MAX_BODY_BYTES", cfg.HTTP.MaximumBodyBytes); err != nil {
		return Config{}, err
	}
	if cfg.Desktop.StartMinimized, err = envBool("ARG_DESKTOP_START_MINIMIZED", false); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Environment) == "" {
		return fmt.Errorf("%w: environment is empty", core.ErrInvalidConfiguration)
	}
	if strings.TrimSpace(c.HTTP.Address) == "" {
		return fmt.Errorf("%w: HTTP address is empty", core.ErrInvalidConfiguration)
	}
	if c.HTTP.ReadTimeout < 0 || c.HTTP.WriteTimeout < 0 || c.HTTP.IdleTimeout < 0 || c.HTTP.ShutdownTimeout <= 0 {
		return fmt.Errorf("%w: HTTP timeouts must be non-negative and shutdown timeout positive", core.ErrInvalidConfiguration)
	}
	if c.HTTP.MaximumBodyBytes < 1024 {
		return fmt.Errorf("%w: maximum request body must be at least 1024 bytes", core.ErrInvalidConfiguration)
	}
	return nil
}

func envString(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}
func envDuration(name string, fallback time.Duration) (time.Duration, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%w: %s: %v", core.ErrInvalidConfiguration, name, err)
	}
	return d, nil
}
func envInt64(name string, fallback int64) (int64, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %s: %v", core.ErrInvalidConfiguration, name, err)
	}
	return n, nil
}
func envBool(name string, fallback bool) (bool, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%w: %s: %v", core.ErrInvalidConfiguration, name, err)
	}
	return b, nil
}
