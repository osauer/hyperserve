package scaffold

import "embed"

//go:embed templates/* templates/cmd/server/* templates/configs/* templates/internal/app/*
var templateFS embed.FS
