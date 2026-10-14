package gatefile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Temitayo-spec/portcullis/server/internal/gate"
)

const (
	paused = `{"maintenance":{"active":true,"reason":"outage"}}`
	normal = `{}`
)

// clock is a fake clock tests move by hand, so the 2-second rule needs no sleep.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

type fixture struct {
	t                   *testing.T
	dir, path, lastGood string
	clock               *clock
}

func newFixture(t *testing.T) *fixture {
	dir := t.TempDir()
	return &fixture{
		t: t, dir: dir,
		path:     filepath.Join(dir, "data", "gate.json"),
		lastGood: filepath.Join(dir, "state", "last-good.json"),
		clock:    &clock{t: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)},
	}
}

func (f *fixture) open() *Source {
	return Open(f.path, f.lastGood, Options{Now: f.clock.now})
}

// write changes the file and moves its modification time on, as a real edit would.
func (f *fixture) write(content string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(f.path, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
	f.clock.advance(time.Second)
	if err := os.Chtimes(f.path, f.clock.t, f.clock.t); err != nil {
		f.t.Fatal(err)
	}
}

func expect(t *testing.T, s State, wantPaused, wantStale bool) {
	t.Helper()
	if got := s.Gate.Maintenance != nil; got != wantPaused {
		t.Errorf("paused = %v, want %v (gate: %s)", got, wantPaused, gate.Describe(s.Gate))
	}
	if s.Stale != wantStale {
		t.Errorf("stale = %v, want %v", s.Stale, wantStale)
	}
}

func TestValidFileIsServedAndSaved(t *testing.T) {
	f := newFixture(t)
	f.write(paused)
	s := f.open()
	expect(t, s.Current(), true, false)
	if g, ok := ReadLastGood(f.lastGood, f.clock.now()); !ok || g.Maintenance == nil {
		t.Error("the pause was not saved as the last good gate")
	}
	if !s.LastGoodSaved() {
		t.Error("LastGoodSaved = false")
	}
}

// R29: a broken file changes nothing.
func TestBrokenFileKeepsThePause(t *testing.T) {
	for name, content := range map[string]string{
		"not JSON":  `{`,
		"empty":     ``,
		"refused":   `{"maintenance":{"active":"true"}}`,
		"a message": `{"message":"Download here"}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.write(paused)
			s := f.open()
			f.write(content)
			expect(t, s.Current(), true, true)
			f.write(normal) // fixing the file brings back "current"
			expect(t, s.Current(), false, false)
		})
	}
}

// R30: a restart with a broken file still serves the pause.
func TestRestartWithBrokenFileKeepsThePause(t *testing.T) {
	f := newFixture(t)
	f.write(paused)
	f.open().Current()
	f.write(`{`)
	expect(t, f.open().Current(), true, true) // a new Source is a restart
}

// R30: with nothing ever good, a broken file serves {} marked stale.
func TestBrokenFileWithNoLastGood(t *testing.T) {
	f := newFixture(t)
	f.write(`{`)
	expect(t, f.open().Current(), false, true)
}

// R31: a file gone for under 2 seconds is not a delete.
func TestBriefDeleteKeepsThePause(t *testing.T) {
	f := newFixture(t)
	f.write(paused)
	s := f.open()
	aside := f.path + ".saving"
	if err := os.Rename(f.path, aside); err != nil {
		t.Fatal(err)
	}
	expect(t, s.Current(), true, true)
	f.clock.advance(1500 * time.Millisecond)
	expect(t, s.Current(), true, true)
	if err := os.Rename(aside, f.path); err != nil { // the editor puts it back, unchanged
		t.Fatal(err)
	}
	expect(t, s.Current(), true, false)
}

// R31: a file gone for 2 seconds or more lifts every stop, and that is saved.
func TestDeleteLiftsEverything(t *testing.T) {
	f := newFixture(t)
	f.write(paused)
	s := f.open()
	if err := os.Remove(f.path); err != nil {
		t.Fatal(err)
	}
	expect(t, s.Current(), true, true)
	f.clock.advance(MissingGrace)
	expect(t, s.Current(), false, false)
	expect(t, f.open().Current(), false, false) // still lifted after a restart
}

// A file missing when the server starts was never seen, so it gets no grace.
func TestMissingAtStartIsEmpty(t *testing.T) {
	f := newFixture(t)
	expect(t, f.open().Current(), false, false)
}

func TestUnreadableFileKeepsThePause(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read any file")
	}
	f := newFixture(t)
	f.write(paused)
	s := f.open()
	if err := os.Chmod(f.path, 0o000); err != nil {
		t.Fatal(err)
	}
	expect(t, s.Current(), true, true)
	if err := os.Chmod(f.path, 0o644); err != nil { // fixing access is noticed with no other edit
		t.Fatal(err)
	}
	expect(t, s.Current(), true, false)
}

func TestUnwritableStateIsReported(t *testing.T) {
	f := newFixture(t)
	f.write(normal)
	f.lastGood = filepath.Join(f.path, "cannot-be-a-folder", "last-good.json") // a file sits where a folder should
	if f.open().LastGoodSaved() {
		t.Error("LastGoodSaved = true; want false")
	}
}

func TestWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gate.json")
	for _, content := range []string{"{}\n", paused + "\n"} {
		if err := WriteAtomic(path, []byte(content)); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(path)
		if string(got) != content {
			t.Errorf("file holds %q, want %q", got, content)
		}
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode %v, want 0644 so the server can read it", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("%d files left in the folder; the temp file should be gone", len(entries))
	}
}
