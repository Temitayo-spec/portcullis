// Package server is the HTTP side of Portcullis (spec/gate-v1.md, R27–R28):
//
//	GET /v1/gate   the gate, as JSON, with X-Portcullis-State: current or stale
//	GET /health    whether the gate is current and its last good copy is saved
//
// Nothing here changes the gate. It is changed with gatectl, on the server,
// which keeps the power to stop every installed app behind SSH.
package server

import (
	"encoding/json"
	"net/http"

	"github.com/Temitayo-spec/portcullis/server/internal/gate"
	"github.com/Temitayo-spec/portcullis/server/internal/gatefile"
)

// New returns the handler. reads may be nil to count nothing.
func New(source *gatefile.Source, reads *Reads) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeJSON(w, http.StatusMethodNotAllowed, []byte(`{"error":"method_not_allowed"}`+"\n"))
			return
		}
		switch r.URL.Path {
		case "/v1/gate":
			if reads != nil {
				reads.Count(r.Header)
			}
			state := source.Current()
			// Never reused without asking: a phone, or a cache in between, that
			// kept an old answer would hold a pause after resume, or miss one.
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("X-Portcullis-State", stateWord(state.Stale))
			writeJSON(w, http.StatusOK, gate.Encode(state.Gate))
		case "/health":
			state := source.Current()
			lastGood := "saved"
			if !source.LastGoodSaved() {
				lastGood = "unsaved"
			}
			body, _ := json.Marshal(map[string]any{"ok": true, "gate": stateWord(state.Stale), "lastGood": lastGood})
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, http.StatusOK, append(body, '\n'))
		default:
			writeJSON(w, http.StatusNotFound, []byte(`{"error":"not_found"}`+"\n"))
		}
	})
}

func stateWord(stale bool) string {
	if stale {
		return "stale"
	}
	return "current"
}

func writeJSON(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	w.Write(body)
}
