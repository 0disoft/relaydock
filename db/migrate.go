package dbassets

import "embed"

// Migrations contains the authoritative forward and rollback SQL files.
// Keeping them embedded allows the migration command to run from a packaged
// binary without depending on the repository working directory.
//
//go:embed migrations/*.up.sql migrations/*.down.sql
var Migrations embed.FS
