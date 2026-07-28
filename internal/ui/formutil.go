package ui

import (
	"net/http"
	"strings"
)

// maxFormBytes bounds a UI POST body the same way the API bounds a JSON
// request body (FF-021 §11).
const maxFormBytes = 1 << 20

// parseForm reads and parses r's URL-encoded body behind a size bound.
// This is form *syntax* parsing only -- no field is validated against a
// domain rule here; that authority belongs entirely to the API command
// this form's handler goes on to call (AD-028: "parse only form syntax").
func parseForm(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		writeErrorPage(w, http.StatusBadRequest, "Request could not be completed", "The submitted form could not be read.")
		return false
	}
	return true
}

// splitLines is this package's one convention for a list-valued form field
// (functional behaviours, constraints, alternatives, ...): a <textarea>,
// one item per line. There is no dynamic add/remove control, because that
// would require JavaScript (FF-021 §8); a static multi-line field is the
// no-JS equivalent.
func splitLines(s string) []string {
	var out []string
	for line := range strings.SplitSeq(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// formValues snapshots exactly the named posted fields, for re-rendering a
// form with its submitted values preserved after a correctable failure
// (FF-021 §12: "input preserved").
func formValues(r *http.Request, keys ...string) map[string]string {
	values := make(map[string]string, len(keys))
	for _, k := range keys {
		values[k] = r.FormValue(k)
	}
	return values
}

// redirectAfterCommand issues the 303 See Other AD-028 requires after a
// successful command (POST-Redirect-GET), so a page refresh never
// resubmits the form.
func redirectAfterCommand(w http.ResponseWriter, r *http.Request, target string) {
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// pageProblem is what a page loader returns instead of data when it could
// not assemble a page -- either this package's own plumbing failed
// (internal) or the API itself rejected the request (api), matching
// writeInternalErrorPage/writeAPIErrorPage's existing split.
type pageProblem struct {
	internal bool
	api      apiResult
}

func (p *pageProblem) write(w http.ResponseWriter) {
	if p.internal {
		writeInternalErrorPage(w)
		return
	}
	writeAPIErrorPage(w, p.api)
}
