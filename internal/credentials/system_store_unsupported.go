//go:build !windows && !darwin && !linux

package credentials

import "fmt"

func newPlatformCredentialBackend() (systemCredentialBackend, error) {
	return nil, fmt.Errorf("%w: native adapter is not implemented on this platform", ErrSystemStoreUnavailable)
}
