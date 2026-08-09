//go:build !windows

package atomicfile

import "os"

func restrictFilePermissions(*os.File) error { return nil }
