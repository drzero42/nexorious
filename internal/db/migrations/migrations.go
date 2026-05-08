package migrations

import (
	"embed"

	bunmigrate "github.com/uptrace/bun/migrate"
)

//go:embed *.sql
var FS embed.FS

var Migrations = bunmigrate.NewMigrations()

func init() {
	if err := Migrations.Discover(FS); err != nil {
		panic(err)
	}
}
