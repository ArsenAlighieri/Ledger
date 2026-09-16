package web

import (
	"embed"
	"io/fs"
)

//go:embed static/* templates/* templates/**/*
var EmbeddedFiles embed.FS

// TemplateFS returns the sub filesystem for templates
func TemplateFS() embed.FS {
	return EmbeddedFiles
}

// StaticFS returns the sub filesystem for static assets
func StaticFS() fs.FS {
	sub, err := fs.Sub(EmbeddedFiles, "static")
	if err != nil {
		panic(err)
	}
	return sub
}
