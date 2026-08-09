package compiler

import "github.com/0disoft/relaydock/internal/protocol/canonical"

type LossMode string

const (
	LossModeStrict      LossMode = "strict"
	LossModeCompatible  LossMode = "compatible"
	LossModePassthrough LossMode = "passthrough"
)

type Loss struct {
	Path                string
	Source, Destination canonical.Protocol
	Description         string
	Fatal               bool
}
type Report struct {
	Mode   LossMode
	Losses []Loss
}

func NewReport(mode LossMode) Report {
	if mode == "" {
		mode = LossModeStrict
	}
	return Report{Mode: mode}
}
func (r *Report) Add(path string, source, destination canonical.Protocol, description string) {
	r.Losses = append(r.Losses, Loss{Path: path, Source: source, Destination: destination, Description: description, Fatal: r.Mode == LossModeStrict})
}
func (r Report) HasFatalLoss() bool {
	for _, loss := range r.Losses {
		if loss.Fatal {
			return true
		}
	}
	return false
}
