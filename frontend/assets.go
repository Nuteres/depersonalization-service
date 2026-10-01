package frontend

import "embed"

// Assets contains embedded frontend assets for Wails.
//
//go:embed all:dist
var Assets embed.FS
