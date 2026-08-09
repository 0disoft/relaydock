package migrate

import (
	"testing"
	"testing/fstest"
)

func TestLoadOrdersAndPairsMigrations(t *testing.T) {
	files := fstest.MapFS{
		"migrations/000002_second.up.sql":  {Data: []byte("SELECT 2;")},
		"migrations/000001_first.down.sql": {Data: []byte("SELECT -1;")},
		"migrations/000001_first.up.sql":   {Data: []byte("SELECT 1;")},
		"migrations/README.md":             {Data: []byte("ignored")},
	}
	migrations, err := Load(files, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 2 || migrations[0].Version != 1 || migrations[1].Version != 2 {
		t.Fatalf("unexpected migrations: %#v", migrations)
	}
	if migrations[0].DownSQL == "" || migrations[0].UpChecksum == "" {
		t.Fatalf("migration pair or checksum missing: %#v", migrations[0])
	}
}

func TestLoadRejectsConflictingNames(t *testing.T) {
	files := fstest.MapFS{
		"migrations/000001_first.up.sql":       {Data: []byte("SELECT 1;")},
		"migrations/000001_different.down.sql": {Data: []byte("SELECT -1;")},
	}
	if _, err := Load(files, "migrations"); err == nil {
		t.Fatal("expected conflicting migration names to fail")
	}
}

func TestLoadRejectsEmptyUpMigration(t *testing.T) {
	files := fstest.MapFS{
		"migrations/000001_first.up.sql": {Data: []byte(" \n")},
	}
	if _, err := Load(files, "migrations"); err == nil {
		t.Fatal("expected empty migration to fail")
	}
}
