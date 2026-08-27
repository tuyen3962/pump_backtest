package web

import "embed"

// Dist is the Vite production build output (webapp → web/dist).
//
//go:embed all:dist
var Dist embed.FS
