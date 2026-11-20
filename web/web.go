// Package web embeds the browser remote.
package web

import (
	_ "embed"
	"net/http"
)

//go:embed remote.html
var remote []byte

//go:embed settings.html
var settings []byte

// Remote serves the single-page remote at "/" and the settings page at "/settings".
func Remote() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var page []byte
		switch r.URL.Path {
		case "/":
			page = remote
		case "/settings":
			page = settings
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(page)
	})
}
