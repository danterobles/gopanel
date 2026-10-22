package web

import "embed"

// Assets contains only compiled local assets. Run make generate before building.
//
//go:embed static/*
var Assets embed.FS
