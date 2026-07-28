package ui

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
)

//go:embed templates/*.html
var templateFiles embed.FS

// templates parses every page template once, at init, so a malformed
// template fails the build/startup rather than a request (FF-021 §14:
// "parse every template at init (fail fast)"). Every page template defines
// "title" and "content"; base.html supplies the shared shell around them.
var templates = template.Must(template.ParseFS(templateFiles, "templates/*.html"))

// render executes the named page template inside the shared base layout
// and writes it with status. It buffers first: html/template can fail
// mid-execution (a missing map key, a nil pointer), and writing directly to
// w would leave a half-written page with a 200 already sent. A render
// failure here is a bug, not a request error, so it falls back to the
// generic error page.
func render(w http.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, name, data); err != nil {
		writeErrorPage(w, http.StatusInternalServerError, "Something went wrong", "An unexpected error occurred.")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// errorPageData is base.html's shape when rendering a standalone error
// page (§8: no internal error leakage -- Detail carries only what §12
// classifies as client-safe).
type errorPageData struct {
	PageTitle string
	Heading   string
	Message   string
}

// writeErrorPage renders the generic error shell directly -- not through
// render, so an error page can never itself fail to render for the same
// reason the original request failed.
func writeErrorPage(w http.ResponseWriter, status int, heading, message string) {
	var buf bytes.Buffer
	data := errorPageData{PageTitle: heading, Heading: heading, Message: message}
	if err := templates.ExecuteTemplate(&buf, "error", data); err != nil {
		// templates is parsed at init and Must-panics on failure, so this
		// path is unreachable in practice; the fallback is plain text
		// rather than a second template call that could fail the same way.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(heading + ": " + message))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// notFoundPage renders the UI's own 404 -- distinct from the API's JSON
// 404, since this response is for a browser (FF-021 §4: "unknown UI routes
// return the UI-defined not-found behaviour").
func notFoundPage(w http.ResponseWriter, r *http.Request) {
	writeErrorPage(w, http.StatusNotFound, "Page not found", "There is no page at "+r.URL.Path+".")
}
