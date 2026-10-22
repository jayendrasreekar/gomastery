package web

import "embed"

// Files contains the embedded web assets (dashboard.html, etc.).
//
//go:embed *
var Files embed.FS
