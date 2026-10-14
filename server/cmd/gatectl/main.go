// Command gatectl changes the gate. Run `gatectl help` for the commands.
package main

import (
	"os"
	"time"

	"github.com/Temitayo-spec/portcullis/server/internal/ctl"
)

func main() {
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}
	os.Exit(ctl.Run(os.Args[1:], ctl.Env{
		GateFile: env("GATE_FILE", "/data/gate.json"),
		StateDir: env("STATE_DIR", "/state"),
		User:     user,
		Now:      time.Now,
		Local:    time.Local, // set TZ to show times in your own zone too
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
	}))
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
