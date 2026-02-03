package main

import (
	"embed"
)

//go:embed frontend/*
var EmbeddedFrontend embed.FS
