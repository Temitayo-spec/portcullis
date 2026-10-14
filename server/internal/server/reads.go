package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/Temitayo-spec/portcullis/server/internal/gate"
	"github.com/Temitayo-spec/portcullis/server/internal/gatefile"
)

// MaxKeys caps how many different store/version/build rows are counted at
// once, so odd headers can't grow memory without end.
const MaxKeys = 500

var (
	contractPattern = regexp.MustCompile(`^\d{1,3}$`)
	buildPattern    = regexp.MustCompile(`^\d{1,10}$`)
)

// ReadKey is one row of install counts. It holds nothing about the person.
type ReadKey struct {
	Contract string `json:"contract"`
	Store    string `json:"store"`
	Version  string `json:"version"`
	Build    string `json:"build"`
}

// Row is a ReadKey and how many reads it had.
type Row struct {
	ReadKey
	Reads int `json:"reads"`
}

// Snapshot is one counting window, as logged and as saved for gatectl.
type Snapshot struct {
	Since time.Time `json:"since"`
	Until time.Time `json:"until"`
	Rows  []Row     `json:"rows"`
}

// Reads counts gate reads by contract, store, version and build (R32).
type Reads struct {
	snapshotPath string
	now          func() time.Time
	log          *slog.Logger

	mu     sync.Mutex
	counts map[ReadKey]int
	since  time.Time
}

// NewReads starts counting. snapshotPath may be empty to only log.
func NewReads(snapshotPath string, now func() time.Time, log *slog.Logger) *Reads {
	if now == nil {
		now = time.Now
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Reads{snapshotPath: snapshotPath, now: now, log: log, counts: map[ReadKey]int{}, since: now()}
}

// KeyFrom reads the R9 headers, squeezing each value into a small set of
// shapes, so a request can't put arbitrary text into the logs.
func KeyFrom(h http.Header) ReadKey {
	k := ReadKey{
		Contract: h.Get("X-Portcullis-Contract"),
		Store:    h.Get("X-Portcullis-Store"),
		Version:  h.Get("X-Portcullis-Version"),
		Build:    h.Get("X-Portcullis-Build"),
	}
	if !contractPattern.MatchString(k.Contract) {
		k.Contract = "none"
	}
	if len(k.Store) > 100 || !gate.IsStoreKey(k.Store) {
		k.Store = "unknown"
	}
	if !gate.IsPlainVersion(k.Version) {
		k.Version = "other"
	}
	if !buildPattern.MatchString(k.Build) {
		k.Build = "other"
	}
	return k
}

// Count records one read.
func (r *Reads) Count(h http.Header) {
	key := KeyFrom(h)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, seen := r.counts[key]; seen || len(r.counts) < MaxKeys {
		r.counts[key]++
	}
}

// Flush logs the window's counts, busiest first, saves them for gatectl, and
// starts a new window.
func (r *Reads) Flush() Snapshot {
	r.mu.Lock()
	snap := Snapshot{Since: r.since, Until: r.now()}
	for key, n := range r.counts {
		snap.Rows = append(snap.Rows, Row{ReadKey: key, Reads: n})
	}
	r.counts = map[ReadKey]int{}
	r.since = snap.Until
	r.mu.Unlock()

	sort.Slice(snap.Rows, func(i, j int) bool {
		a, b := snap.Rows[i], snap.Rows[j]
		if a.Reads != b.Reads {
			return a.Reads > b.Reads
		}
		return a.Store+a.Version+a.Build < b.Store+b.Version+b.Build
	})
	if len(snap.Rows) > 0 {
		r.log.Info("gate_reads", "since", snap.Since, "rows", snap.Rows)
	}
	if r.snapshotPath != "" {
		body, _ := json.MarshalIndent(snap, "", "  ")
		err := os.MkdirAll(filepath.Dir(r.snapshotPath), 0o755)
		if err == nil {
			err = gatefile.WriteAtomic(r.snapshotPath, append(body, '\n'))
		}
		if err != nil {
			r.log.Error("reads_unwritable", "path", r.snapshotPath, "error", err.Error())
		}
	}
	return snap
}
