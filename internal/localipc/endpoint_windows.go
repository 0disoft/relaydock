//go:build windows

package localipc

import "os/user"

func DefaultEndpoint() string {
	current, err := user.Current()
	if err != nil {
		return `\\.\pipe\relaydock`
	}
	return `\\.\pipe\relaydock-` + current.Uid
}
