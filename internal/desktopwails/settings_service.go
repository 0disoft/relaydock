package desktopwails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/0disoft/relaydock/internal/appdirs"
	"github.com/0disoft/relaydock/internal/autostart"
	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/persistence/atomicfile"
)

type Settings struct {
	StartAtLogin          bool   `json:"startAtLogin"`
	MinimizeToTray        bool   `json:"minimizeToTray"`
	DefaultRoute          string `json:"defaultRoute"`
	MaximumCostMinor      int64  `json:"maximumCostMinor"`
	GatewayPort           int    `json:"gatewayPort"`
	DefaultRepositoryRoot string `json:"defaultRepositoryRoot"`
	MCPBridgePath         string `json:"mcpBridgePath"`
}

type SettingsService struct {
	mu        sync.Mutex
	path      string
	autostart autostart.Service
}

func NewSettingsService() *SettingsService {
	return NewSettingsServiceWithRoot(os.Getenv("ARG_DESKTOP_DATA_DIR"))
}

func NewSettingsServiceWithRoot(root string) *SettingsService {
	path, err := appdirs.SettingsPath(root)
	if err != nil {
		// The service will surface the configuration error from Get/Save. A
		// non-empty sentinel avoids accidentally writing into the process CWD.
		path = ""
	}
	return &SettingsService{path: path}
}

func (s *SettingsService) Path() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.path
}

func (s *SettingsService) setAutostartService(service autostart.Service) {
	s.mu.Lock()
	s.autostart = service
	s.mu.Unlock()
}

// reconcileAutostart makes the persisted preference authoritative at startup.
// Re-registering an enabled entry is intentional: it refreshes a stale binary
// path after an application update and preserves the --background argument.
func (s *SettingsService) reconcileAutostart(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	settings, err := s.readLocked()
	if err != nil {
		return err
	}
	if s.autostart == nil {
		return nil
	}
	if settings.StartAtLogin {
		if err := s.autostart.Enable(ctx); err != nil {
			return fmt.Errorf("enable login autostart: %w", err)
		}
		return nil
	}
	if err := s.autostart.Disable(ctx); err != nil {
		return fmt.Errorf("disable login autostart: %w", err)
	}
	return nil
}

func (s *SettingsService) Get() (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings, err := s.readLocked()
	if err != nil {
		return Settings{}, err
	}
	if s.autostart != nil {
		enabled, err := s.autostart.Enabled(context.Background())
		if err != nil {
			return Settings{}, fmt.Errorf("read login autostart state: %w", err)
		}
		settings.StartAtLogin = enabled
	}
	return settings, nil
}

func (s *SettingsService) Save(settings Settings) error {
	if err := validateSettings(settings); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	previous, err := s.readLocked()
	if err != nil {
		return err
	}
	previousAutostart := previous.StartAtLogin
	if s.autostart != nil {
		previousAutostart, err = s.autostart.Enabled(context.Background())
		if err != nil {
			return fmt.Errorf("read login autostart state: %w", err)
		}
		if err := setAutostart(context.Background(), s.autostart, settings.StartAtLogin); err != nil {
			return err
		}
	}

	if err := s.writeLocked(settings); err != nil {
		if s.autostart == nil {
			return err
		}
		rollbackErr := setAutostart(context.Background(), s.autostart, previousAutostart)
		if rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("rollback login autostart: %w", rollbackErr))
		}
		return err
	}
	return nil
}

func (s *SettingsService) readLocked() (Settings, error) {
	if strings.TrimSpace(s.path) == "" {
		return Settings{}, fmt.Errorf("%w: settings path", core.ErrInvalidConfiguration)
	}
	settings := defaultSettings()
	raw, err := atomicfile.Read(s.path, 1<<20)
	if err != nil {
		return Settings{}, err
	}
	if len(raw) == 0 {
		return settings, nil
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return Settings{}, fmt.Errorf("decode settings: %w", err)
	}
	if err := validateSettings(settings); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

func (s *SettingsService) writeLocked(settings Settings) error {
	if strings.TrimSpace(s.path) == "" {
		return fmt.Errorf("%w: settings path", core.ErrInvalidConfiguration)
	}
	raw, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return atomicfile.Write(s.path, raw, 0o600)
}

func setAutostart(ctx context.Context, service autostart.Service, enabled bool) error {
	if enabled {
		if err := service.Enable(ctx); err != nil {
			return fmt.Errorf("enable login autostart: %w", err)
		}
		return nil
	}
	if err := service.Disable(ctx); err != nil {
		return fmt.Errorf("disable login autostart: %w", err)
	}
	return nil
}

func defaultSettings() Settings {
	root, _ := os.Getwd()
	return Settings{
		MinimizeToTray:        true,
		DefaultRoute:          "openai_api_pro",
		MaximumCostMinor:      0,
		GatewayPort:           10100,
		DefaultRepositoryRoot: root,
	}
}

func validateSettings(settings Settings) error {
	switch strings.TrimSpace(settings.DefaultRoute) {
	case "openai_api_pro", "chatgpt_web_handoff":
	default:
		return fmt.Errorf("%w: unsupported default route", core.ErrInvalidArgument)
	}
	if settings.MaximumCostMinor < 0 {
		return fmt.Errorf("%w: maximum cost cannot be negative", core.ErrInvalidArgument)
	}
	if settings.GatewayPort < 1024 || settings.GatewayPort > 65535 {
		return fmt.Errorf("%w: gateway port must be between 1024 and 65535", core.ErrInvalidArgument)
	}
	return nil
}
