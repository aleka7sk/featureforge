package ui

import (
	"embed"
	"net/http"
)

//go:embed static/style.css
var staticFiles embed.FS

// handleStaticCSS serves the one embedded stylesheet (FF-021 §8: no
// filesystem path configured, nothing else exposed). A fixed Content-Type
// and a long, safe cache lifetime are set directly: the file is embedded
// at build time, so its content can never change without a new binary.
func handleStaticCSS(w http.ResponseWriter, r *http.Request) {
	data, err := staticFiles.ReadFile("static/style.css")
	if err != nil {
		writeErrorPage(w, http.StatusInternalServerError, "Something went wrong", "An unexpected error occurred.")
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
