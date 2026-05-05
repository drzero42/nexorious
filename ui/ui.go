package ui

import "embed"

//go:embed all:dist
var UIBox embed.FS

//go:embed all:migrate
var MigrateBox embed.FS
