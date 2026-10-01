package frontend

import "embed"

// Assets contains embedded frontend assets for Wails.
//
//go:embed index.html tailwind.min.js css js
var Assets embed.FS
