package atomicfile

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Write replaces path with data while keeping the previous complete file
// visible until the new file has been flushed. The fallback path preserves a
// backup when the platform cannot replace an existing destination directly.
func Write(path string, data []byte, mode fs.FileMode) error {
	if path == "" {
		return fmt.Errorf("atomicfile: empty path")
	}
	if mode == 0 {
		mode = 0o600
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("atomicfile: create directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("atomicfile: create temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("atomicfile: set permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("atomicfile: write: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("atomicfile: sync: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("atomicfile: close: %w", err)
	}
	if err := replace(temporaryPath, path); err != nil {
		return err
	}
	removeTemporary = false
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("atomicfile: sync directory: %w", err)
	}
	return nil
}

// Read returns nil, nil when the file does not exist. Callers can therefore
// distinguish an empty initial store without relying on a race-prone Stat.
func Read(path string, maximumBytes int64) ([]byte, error) {
	if maximumBytes <= 0 {
		maximumBytes = 64 << 20
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("atomicfile: open: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("atomicfile: stat: %w", err)
	}
	if info.Size() > maximumBytes {
		return nil, fmt.Errorf("atomicfile: file exceeds %d bytes", maximumBytes)
	}
	buffer, err := io.ReadAll(io.LimitReader(file, maximumBytes+1))
	if err != nil {
		return nil, fmt.Errorf("atomicfile: read: %w", err)
	}
	if int64(len(buffer)) > maximumBytes {
		return nil, fmt.Errorf("atomicfile: file exceeds %d bytes", maximumBytes)
	}
	return buffer, nil
}

func replace(source, destination string) error {
	if err := replaceNative(source, destination); err == nil {
		return nil
	}

	// Some platforms do not replace an existing destination. Move the previous
	// file aside and restore it if installing the new file fails.
	backup := destination + ".bak"
	_ = os.Remove(backup)
	if _, err := os.Stat(destination); err == nil {
		if err := os.Rename(destination, backup); err != nil {
			return fmt.Errorf("atomicfile: move previous file aside: %w", err)
		}
	}
	if err := os.Rename(source, destination); err != nil {
		restoreErr := os.Rename(backup, destination)
		if restoreErr != nil && !errors.Is(restoreErr, os.ErrNotExist) {
			return errors.Join(fmt.Errorf("atomicfile: install replacement: %w", err), fmt.Errorf("atomicfile: restore previous file: %w", restoreErr))
		}
		return fmt.Errorf("atomicfile: install replacement: %w", err)
	}
	_ = os.Remove(backup)
	return nil
}
