package releasepack

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

func aggregateRecords(records []FileRecord) string {
	sorted := append([]FileRecord(nil), records...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	hash := sha256.New()
	for _, record := range sorted {
		writeAggregateField(hash, record.Path)
		writeAggregateField(hash, strconv.FormatInt(record.Size, 10))
		writeAggregateField(hash, record.SHA256)
		writeAggregateField(hash, record.Mode)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func writeAggregateField(writer io.Writer, value string) {
	_, _ = io.WriteString(writer, strconv.Itoa(len(value)))
	_, _ = io.WriteString(writer, ":")
	_, _ = io.WriteString(writer, value)
	_, _ = io.WriteString(writer, "\n")
}

// replaceFile publishes a completed file while preserving the previous file if
// the final rename fails. This works on Windows, where Rename cannot replace an
// existing destination, and keeps all temporary files in the destination
// directory so the final publish remains on one filesystem.
func replaceFile(temporary, destination string) error {
	if temporary == destination {
		return fmt.Errorf("temporary and destination paths are identical")
	}
	if err := replaceNative(temporary, destination); err == nil {
		return syncDirectory(filepath.Dir(destination))
	}

	backup := destination + ".replace-backup"
	_ = os.Remove(backup)

	_, statErr := os.Stat(destination)
	destinationExists := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return fmt.Errorf("inspect destination: %w", statErr)
	}
	if destinationExists {
		if err := os.Rename(destination, backup); err != nil {
			return fmt.Errorf("stage previous destination: %w", err)
		}
	}
	if err := os.Rename(temporary, destination); err != nil {
		if destinationExists {
			_ = os.Rename(backup, destination)
		}
		return fmt.Errorf("publish replacement: %w", err)
	}
	if destinationExists {
		if err := os.Remove(backup); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove replacement backup: %w", err)
		}
	}
	return syncDirectory(filepath.Dir(destination))
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return nil // Some platforms do not permit opening directories for sync.
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return nil // File contents are already synced; directory sync is best effort.
	}
	return nil
}
