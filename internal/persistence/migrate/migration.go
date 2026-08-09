package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Migration struct {
	Version      int64
	Name         string
	UpSQL        string
	DownSQL      string
	UpChecksum   string
	DownChecksum string
}

func Load(source fs.FS, directory string) ([]Migration, error) {
	entries, err := fs.ReadDir(source, directory)
	if err != nil {
		return nil, fmt.Errorf("read migration directory: %w", err)
	}
	byVersion := make(map[int64]*Migration)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		version, name, direction, ok := parseFilename(entry.Name())
		if !ok {
			continue
		}
		raw, err := fs.ReadFile(source, filepath.ToSlash(filepath.Join(directory, entry.Name())))
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		if strings.TrimSpace(string(raw)) == "" {
			return nil, fmt.Errorf("migration %s is empty", entry.Name())
		}
		migration := byVersion[version]
		if migration == nil {
			migration = &Migration{Version: version, Name: name}
			byVersion[version] = migration
		} else if migration.Name != name {
			return nil, fmt.Errorf("migration version %d has conflicting names %q and %q", version, migration.Name, name)
		}
		switch direction {
		case "up":
			if migration.UpSQL != "" {
				return nil, fmt.Errorf("migration version %d has multiple up files", version)
			}
			migration.UpSQL = string(raw)
			migration.UpChecksum = checksum(raw)
		case "down":
			if migration.DownSQL != "" {
				return nil, fmt.Errorf("migration version %d has multiple down files", version)
			}
			migration.DownSQL = string(raw)
			migration.DownChecksum = checksum(raw)
		}
	}
	migrations := make([]Migration, 0, len(byVersion))
	for _, migration := range byVersion {
		if migration.UpSQL == "" {
			return nil, fmt.Errorf("migration version %d has no up file", migration.Version)
		}
		migrations = append(migrations, *migration)
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	for index, migration := range migrations {
		if index > 0 && migration.Version == migrations[index-1].Version {
			return nil, fmt.Errorf("duplicate migration version %d", migration.Version)
		}
	}
	return migrations, nil
}

func parseFilename(name string) (version int64, migrationName, direction string, ok bool) {
	var suffix string
	switch {
	case strings.HasSuffix(name, ".up.sql"):
		direction, suffix = "up", ".up.sql"
	case strings.HasSuffix(name, ".down.sql"):
		direction, suffix = "down", ".down.sql"
	default:
		return 0, "", "", false
	}
	stem := strings.TrimSuffix(name, suffix)
	versionText, migrationName, found := strings.Cut(stem, "_")
	if !found || migrationName == "" {
		return 0, "", "", false
	}
	version, err := strconv.ParseInt(versionText, 10, 64)
	if err != nil || version <= 0 {
		return 0, "", "", false
	}
	return version, migrationName, direction, true
}

func checksum(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
