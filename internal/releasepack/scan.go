package releasepack

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

var excludedDirectoryNames = map[string]struct{}{
	".git": {}, ".idea": {}, ".vscode": {}, "node_modules": {}, ".svelte-kit": {},
	"coverage": {}, ".cache": {}, "__pycache__": {},
}

var excludedFileNames = map[string]struct{}{
	".DS_Store": {}, "Thumbs.db": {},
}

func scanFiles(root string, includeManifest bool) ([]FileRecord, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	var records []FileRecord
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = normalizeRelativePath(relative)
		if entry.IsDir() {
			if shouldExcludeDirectory(relative, entry.Name()) {
				return filepath.SkipDir
			}
			if relative == manifestDirectory && !includeManifest {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("repository contains unsupported non-regular file %s", relative)
		}
		if _, excluded := excludedFileNames[entry.Name()]; excluded {
			return nil
		}
		if !includeManifest && relative == rootManifestPath {
			return nil
		}
		record, err := inspectFile(path, relative)
		if err != nil {
			return err
		}
		records = append(records, record)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan repository: %w", err)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Path < records[j].Path })
	return records, nil
}

func inspectFile(path, relative string) (FileRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return FileRecord{}, fmt.Errorf("open %s: %w", relative, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return FileRecord{}, fmt.Errorf("stat %s: %w", relative, err)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return FileRecord{}, fmt.Errorf("hash %s: %w", relative, err)
	}
	return FileRecord{
		Path: relative, Size: info.Size(), SHA256: hex.EncodeToString(hash.Sum(nil)),
		Mode: fmt.Sprintf("%04o", canonicalFileMode(relative, info.Mode())),
	}, nil
}

func canonicalFileMode(relative string, mode os.FileMode) os.FileMode {
	return canonicalFileModeForOS(relative, mode, runtime.GOOS)
}

func canonicalFileModeForOS(relative string, mode os.FileMode, goos string) os.FileMode {
	executable := mode.Perm()&0o111 != 0
	if goos == "windows" {
		// Windows does not expose POSIX execute bits. Shell sources are the only
		// executable files shipped by this repository outside built artifacts.
		executable = strings.EqualFold(filepath.Ext(relative), ".sh")
	}
	if executable {
		return 0o755
	}
	return 0o644
}

func enforceSizePolicy(records []FileRecord, policy SizePolicy) error {
	var violations []string
	known := make(map[string]struct{}, len(records))
	for _, record := range records {
		known[record.Path] = struct{}{}
		if record.Size <= policy.MaxBytes {
			continue
		}
		if _, allowed := policy.Allows(record.Path); allowed {
			continue
		}
		violations = append(violations, fmt.Sprintf("%s (%d bytes)", record.Path, record.Size))
	}
	for _, exception := range policy.Exceptions {
		if _, exists := known[exception.Path]; !exists {
			violations = append(violations, fmt.Sprintf("stale exception %s (%s)", exception.Path, exception.Reason))
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		return fmt.Errorf("repository file-size policy failed; limit=%d bytes: %s", policy.MaxBytes, strings.Join(violations, "; "))
	}
	return nil
}

func normalizeRelativePath(path string) string {
	path = filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return path
}

func shouldExcludeDirectory(relative, name string) bool {
	if _, excluded := excludedDirectoryNames[name]; excluded {
		return true
	}
	return relative == "bin" || relative == "dist"
}
