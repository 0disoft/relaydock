//go:build windows

package localipc

import "os/user"

func DefaultEndpoint() string {
	current, err := user.Current()
	if err != nil {
		return `\\.\pipe\ai-runtime-gateway`
	}
	return `\\.\pipe\ai-runtime-gateway-` + current.Uid
}
