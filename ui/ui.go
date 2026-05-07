package ui

import "embed"

//go:embed all:dist
var UIBox embed.FS

//go:embed all:migrate
var MigrateBox embed.FS

//go:embed db-error
var DBErrorBox embed.FS

//go:embed setup
var SetupBox embed.FS
