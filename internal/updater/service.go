package updater

import "context"

type Info struct {
	Version      string `json:"version"`
	ReleaseNotes string `json:"releaseNotes,omitempty"`
	DownloadSize int64  `json:"downloadSize"`
	Available    bool   `json:"available"`
	SHA256       string `json:"sha256,omitempty"`
}

type Service interface {
	Check(context.Context) (Info, error)
	Download(context.Context, string) error
	InstallOnExit(context.Context, string) error
}
