package desktopwails

import (
	"context"
	"errors"
	"fmt"

	"github.com/0disoft/relaydock/internal/autostart"
	"github.com/0disoft/relaydock/internal/core"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const desktopAutostartIdentifier = "com.0disoft.relaydock"

var _ autostart.Service = (*wailsAutostartService)(nil)

type wailsAutostartService struct {
	manager *application.AutostartManager
}

func newWailsAutostartService(manager *application.AutostartManager) autostart.Service {
	return &wailsAutostartService{manager: manager}
}

func (s *wailsAutostartService) Enable(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.manager == nil {
		return fmt.Errorf("%w: Wails autostart manager", core.ErrInvalidConfiguration)
	}
	return s.manager.EnableWithOptions(application.AutostartOptions{
		Identifier: desktopAutostartIdentifier,
		Arguments:  []string{"--background"},
	})
}

func (s *wailsAutostartService) Disable(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.manager == nil {
		return fmt.Errorf("%w: Wails autostart manager", core.ErrInvalidConfiguration)
	}
	err := s.manager.Disable()
	if errors.Is(err, application.ErrAutostartNotSupported) {
		return nil
	}
	return err
}

func (s *wailsAutostartService) Enabled(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if s == nil || s.manager == nil {
		return false, fmt.Errorf("%w: Wails autostart manager", core.ErrInvalidConfiguration)
	}
	enabled, err := s.manager.IsEnabled()
	if errors.Is(err, application.ErrAutostartNotSupported) {
		return false, nil
	}
	return enabled, err
}
