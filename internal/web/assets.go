package web

import (
	"embed"
	"net/http"
)

//go:embed *.html *.css *.js
var assets embed.FS

func Handler() http.Handler { return http.FileServer(http.FS(assets)) }
