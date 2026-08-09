//go:build linux

package credentials

func newPlatformCredentialBackend() (systemCredentialBackend, error) {
	return linuxCredentialBackend{connect: connectSecretService}, nil
}
