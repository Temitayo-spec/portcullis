package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Temitayo-spec/portcullis/server/internal/gatefile"
)

func serve(t *testing.T, gateJSON string) (http.Handler, *Reads, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gate.json")
	if gateJSON != "" {
		if err := os.WriteFile(path, []byte(gateJSON), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	source := gatefile.Open(path, filepath.Join(dir, "state", "last-good.json"), gatefile.Options{})
	reads := NewReads(filepath.Join(dir, "state", "reads.json"), nil, nil)
	return New(source, reads), reads, dir
}

func get(h http.Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestGate(t *testing.T) {
	h, _, _ := serve(t, `{"maintenance":{"active":true,"reason":"upgrade"}}`)
	res := get(h, "/v1/gate", nil)
	if res.Code != 200 {
		t.Fatalf("status %d", res.Code)
	}
	for header, want := range map[string]string{
		"Content-Type":       "application/json; charset=utf-8",
		"Cache-Control":      "no-cache",
		"X-Portcullis-State": "current",
	} {
		if got := res.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if got := res.Body.String(); got != `{"maintenance":{"active":true,"reason":"upgrade"}}`+"\n" {
		t.Errorf("body %s", got)
	}
}

func TestBrokenGateIsStale(t *testing.T) {
	h, _, _ := serve(t, `{`)
	res := get(h, "/v1/gate", nil)
	if got := res.Header().Get("X-Portcullis-State"); got != "stale" {
		t.Errorf("state %q, want stale", got)
	}
	if got := strings.TrimSpace(res.Body.String()); got != "{}" {
		t.Errorf("body %s, want {} when nothing was ever good", got)
	}
}

func TestHealth(t *testing.T) {
	h, _, _ := serve(t, `{}`)
	res := get(h, "/health", nil)
	var body map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["ok"] != true || body["gate"] != "current" || body["lastGood"] != "saved" {
		t.Errorf("health %v", body)
	}
	if got := res.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control %q", got)
	}
}

func TestOtherRequests(t *testing.T) {
	h, _, _ := serve(t, `{}`)
	if res := get(h, "/v1/nope", nil); res.Code != 404 {
		t.Errorf("unknown path: %d", res.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/gate", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 405 || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Errorf("POST: %d, Allow %q", rec.Code, rec.Header().Get("Allow"))
	}
}

func TestReadsAreCountedAndSaved(t *testing.T) {
	h, reads, dir := serve(t, `{}`)
	play := map[string]string{"X-Portcullis-Contract": "1", "X-Portcullis-Store": "play", "X-Portcullis-Version": "2.4.0", "X-Portcullis-Build": "120"}
	for i := 0; i < 3; i++ {
		get(h, "/v1/gate", play)
	}
	get(h, "/v1/gate", map[string]string{"X-Portcullis-Store": "<script>", "X-Portcullis-Version": "2.4.0 (beta)", "X-Portcullis-Build": "x"})
	get(h, "/health", play) // health checks are not reads

	snap := reads.Flush()
	if len(snap.Rows) != 2 {
		t.Fatalf("%d rows, want 2: %+v", len(snap.Rows), snap.Rows)
	}
	if got := snap.Rows[0]; got.Store != "play" || got.Version != "2.4.0" || got.Build != "120" || got.Reads != 3 {
		t.Errorf("busiest row %+v", got)
	}
	if got := snap.Rows[1].ReadKey; got != (ReadKey{Contract: "none", Store: "unknown", Version: "other", Build: "other"}) {
		t.Errorf("odd headers became %+v", got)
	}
	saved, err := os.ReadFile(filepath.Join(dir, "state", "reads.json"))
	if err != nil || !strings.Contains(string(saved), `"reads": 3`) {
		t.Errorf("reads.json: %s %v", saved, err)
	}
	if again := reads.Flush(); len(again.Rows) != 0 {
		t.Errorf("a flush should start a new window, got %d rows", len(again.Rows))
	}
}

func TestReadsAreCapped(t *testing.T) {
	reads := NewReads("", func() time.Time { return time.Unix(0, 0) }, nil)
	for i := 0; i < MaxKeys+50; i++ {
		h := http.Header{}
		h.Set("X-Portcullis-Build", strconv.Itoa(i+1)) // every read a different build
		reads.Count(h)
	}
	if n := len(reads.Flush().Rows); n != MaxKeys {
		t.Errorf("%d rows, want exactly %d", n, MaxKeys)
	}
}
