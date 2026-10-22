package scripts

import (
	"embed"
	"io/fs"
)

//go:embed schema_migrations.sql migrations/*.sql
var migrations embed.FS

// Migrations returns schema_migrations.sql and the migrations directory,
// laid out as in this directory.
func Migrations() fs.FS {
	return migrations
}
