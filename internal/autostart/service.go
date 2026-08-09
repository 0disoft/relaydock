package autostart

import "context"

type Service interface {
	Enable(context.Context) error
	Disable(context.Context) error
	Enabled(context.Context) (bool, error)
}
