// Package gatefile decides which gate the server serves while the gate file
// on disk is fine, broken, refused or missing (spec/gate-v1.md, R29–R31).
//
// The rule underneath all of it: a file that cannot be used changes nothing.
// A typo made while the app is paused must not lift the pause, and a restart
// with a broken file must not either, so the last good gate is served
// (marked stale) and also kept on disk. Only a deliberate delete, gone for
// two seconds, lifts every stop.
package gatefile

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Temitayo-spec/portcullis/server/internal/gate"
)

// MissingGrace is how long the file must stay missing before that counts as
// a delete (R31). An editor that saves by renaming the old file away is back
// well within it.
const MissingGrace = 2 * time.Second

// State is what the server serves right now.
type State struct {
	Gate  gate.Gate
	Stale bool // true while the last good gate is served instead of the file
}

// Source re-reads the gate file whenever it changes, and keeps the last good
// gate in memory and in lastGoodPath. It is safe for concurrent use.
type Source struct {
	path, lastGoodPath string
	now                func() time.Time
	log                *slog.Logger

	mu           sync.Mutex
	state        State
	stamp        string    // what the file looked like at the last read
	missingSince time.Time // when it was first seen missing, while it stays missing
	seenPresent  bool      // only a file this process saw can go missing "briefly"
	saveWorked   bool
}

// Options are optional settings for Open. The zero value is fine.
type Options struct {
	Now func() time.Time // swapped in tests
	Log *slog.Logger
}

// Open starts serving from path. lastGoodPath may be empty to keep the last
// good gate in memory only.
func Open(path, lastGoodPath string, opts Options) *Source {
	s := &Source{path: path, lastGoodPath: lastGoodPath, now: opts.Now, log: opts.Log, saveWorked: true}
	if s.now == nil {
		s.now = time.Now
	}
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}
	// R30: start from the saved gate, marked stale until the file confirms it.
	// With nothing saved, start from {}: run.
	if saved, ok := ReadLastGood(lastGoodPath, s.now()); ok {
		s.state = State{Gate: saved, Stale: true}
	} else {
		s.state = State{Stale: true}
	}
	s.mu.Lock()
	s.refresh()
	s.save(s.state.Gate) // proves at startup that the last good gate can be saved
	s.mu.Unlock()
	return s
}

// Current returns the gate to serve, re-reading the file first if it changed.
func (s *Source) Current() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh()
	return s.state
}

// LastGoodSaved reports whether the last good gate could be written, the last
// time it was.
func (s *Source) LastGoodSaved() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveWorked
}

func (s *Source) refresh() {
	info, err := os.Stat(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		s.missing()
		return
	}
	if err != nil {
		s.unusable("stat:"+err.Error(), "gate_unreadable", err)
		return
	}
	s.missingSince = time.Time{}
	s.seenPresent = true
	// The mode is part of the stamp, so fixing permissions is noticed too.
	stamp := fmt.Sprintf("%d:%d:%v", info.ModTime().UnixNano(), info.Size(), info.Mode())
	if stamp == s.stamp {
		return
	}

	raw, err := os.ReadFile(s.path)
	if err != nil {
		s.unusable(stamp, "gate_unreadable", err)
		return
	}
	r := gate.Validate(raw, s.now())
	if !r.OK() {
		s.stamp = stamp
		s.keepLastGood("gate_refused", slog.Any("problems", r.Problems))
		return
	}
	s.stamp = stamp
	s.state = State{Gate: r.Gate}
	s.save(r.Gate)
	s.log.Info("gate_loaded", "path", s.path, "effect", gate.Describe(r.Gate), "warnings", r.Warnings)
}

// missing handles a gate file that isn't there (R31).
func (s *Source) missing() {
	if s.stamp == "missing" {
		return
	}
	if s.seenPresent {
		at := s.now()
		if s.missingSince.IsZero() {
			s.missingSince = at
			s.stamp = "" // so the file is read again when it comes back, even unchanged
			s.keepLastGood("gate_missing_briefly")
			return
		}
		if at.Sub(s.missingSince) < MissingGrace {
			return
		}
	}
	s.stamp = "missing"
	s.state = State{}
	s.save(s.state.Gate)
	s.log.Info("gate_missing", "path", s.path, "effect", gate.Describe(s.state.Gate))
}

func (s *Source) unusable(stamp, event string, err error) {
	if stamp == s.stamp {
		return
	}
	s.stamp = stamp
	s.keepLastGood(event, slog.String("error", err.Error()))
}

// keepLastGood serves the last good gate, marked stale (R29).
func (s *Source) keepLastGood(event string, attrs ...any) {
	s.state.Stale = true
	s.log.Warn(event, append([]any{"path", s.path, "effect", "kept the last good gate: " + gate.Describe(s.state.Gate)}, attrs...)...)
}

func (s *Source) save(g gate.Gate) {
	if s.lastGoodPath == "" {
		return
	}
	err := os.MkdirAll(filepath.Dir(s.lastGoodPath), 0o755)
	if err == nil {
		err = WriteAtomic(s.lastGoodPath, gate.Encode(g))
	}
	switch {
	case err != nil && s.saveWorked:
		s.log.Error("last_good_unwritable", "path", s.lastGoodPath, "error", err.Error())
	case err == nil && !s.saveWorked:
		s.log.Info("last_good_saved_again", "path", s.lastGoodPath)
	}
	s.saveWorked = err == nil
}

// ReadLastGood loads a saved gate. A missing or unusable file gives ok=false.
func ReadLastGood(path string, now time.Time) (gate.Gate, bool) {
	if path == "" {
		return gate.Gate{}, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return gate.Gate{}, false
	}
	r := gate.Validate(raw, now)
	return r.Gate, r.OK()
}

// WriteAtomic replaces path with data in one step: a reader sees the old file
// or the new one, never half of each. The new file is readable by everyone,
// so a server running as another user can still read what gatectl wrote.
func WriteAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // a no-op once the rename has happened
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
