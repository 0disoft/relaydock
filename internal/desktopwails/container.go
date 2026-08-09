package desktopwails

import (
	"os"

	"github.com/0disoft/relaydock/internal/appdirs"
	expertapp "github.com/0disoft/relaydock/internal/expert/app"
	"github.com/0disoft/relaydock/internal/localruntime"
)

type Container struct {
	App           *expertapp.App
	Local         *localruntime.Runtime
	Runtime       *RuntimeService
	Consultations *ConsultationService
	Settings      *SettingsService
}

func NewContainer() (*Container, error) {
	dataRoot := os.Getenv("ARG_DESKTOP_DATA_DIR")
	settingsService := NewSettingsServiceWithRoot(dataRoot)
	settings, err := settingsService.Get()
	if err != nil {
		return nil, err
	}
	statePath, err := appdirs.ExpertStatePath(dataRoot)
	if err != nil {
		return nil, err
	}
	application, err := expertapp.NewLocal(statePath)
	if err != nil {
		return nil, err
	}
	local, err := localruntime.New(application, settings.DefaultRepositoryRoot)
	if err != nil {
		return nil, err
	}
	return &Container{
		App:           application,
		Local:         local,
		Runtime:       NewRuntimeService(local.Handler()),
		Consultations: NewConsultationService(application),
		Settings:      settingsService,
	}, nil
}
