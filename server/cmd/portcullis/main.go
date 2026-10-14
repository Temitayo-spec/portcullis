// Command portcullis serves the gate (spec/gate-v1.md).
//
// Settings, all optional:
//
//	PORT       where to listen (default 8080)
//	GATE_FILE  the gate gatectl writes (default /data/gate.json)
//	STATE_DIR  the last good gate and install counts (default /state)
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Temitayo-spec/portcullis/server/internal/gatefile"
	"github.com/Temitayo-spec/portcullis/server/internal/server"
)

// CountWindow is how often install counts are logged and saved (R32).
const CountWindow = 10 * time.Minute

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	port := env("PORT", "8080")
	gateFile := env("GATE_FILE", "/data/gate.json")
	stateDir := env("STATE_DIR", "/state")

	source := gatefile.Open(gateFile, filepath.Join(stateDir, "last-good.json"), gatefile.Options{Log: log})
	reads := server.NewReads(filepath.Join(stateDir, "reads.json"), nil, log)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           server.New(source, reads),
		ReadHeaderTimeout: 5 * time.Second, // a slow client can't hold a connection open forever
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		ticker := time.NewTicker(CountWindow)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				reads.Flush()
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()

	state := source.Current()
	log.Info("listening", "port", port, "gate_file", gateFile, "state_dir", stateDir, "stale", state.Stale, "last_good_saved", source.LastGoodSaved())
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server_failed", "error", err.Error())
		os.Exit(1)
	}
	reads.Flush() // keep the last partial window's counts
	log.Info("stopped")
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
