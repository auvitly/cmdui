package ui

import (
	"embed"
	"io/fs"
	"strings"
)

//go:embed static/*
var staticFiles embed.FS

func StaticFiles() fs.FS {
	files, _ := fs.Sub(staticFiles, "static")
	return files
}

func withSharedStylesheet(markup string) string {
	return strings.Replace(markup, "</head>", `<link rel="stylesheet" href="/static/app.css"></head>`, 1)
}
