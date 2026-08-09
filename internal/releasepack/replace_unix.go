//go:build !windows

package releasepack

import "os"

func replaceNative(source, destination string) error {
	return os.Rename(source, destination)
}
