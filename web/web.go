// Package web embeds the browser remote.
package web

import (
	_ "embed"
	"net/http"
)

//go:embed remote.html
var remote []byte

// Remote serves the single-page remote at "/".
func Remote() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(remote)
	})
}
