// Command agent is the single domain-monitoring process.
//
// One instance checks every domain and subdomain in list-domain.txt from the
// outside: reachability, TLS certificate, DNS and registration expiry. Unlike
// the server-monitor fleet there is nothing to install per target, because
// none of these checks need to run on the machine being watched.
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"domain-monitor/backend/internal/config"
	"domain-monitor/backend/internal/handler"
	"domain-monitor/backend/internal/incident"
	"domain-monitor/backend/internal/notify"
	"domain-monitor/backend/internal/scheduler"
	"domain-monitor/backend/internal/state"
	"domain-monitor/backend/internal/store"
	"domain-monitor/backend/internal/target"
	"domain-monitor/backend/internal/websocket"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	cfg := config.Load()
	switch cfg.LogLevel {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	// 1. Target list. Without it there is nothing to monitor, so a bad path is
	//    fatal rather than a silently empty dashboard.
	loader := target.NewLoader(cfg.TargetsFile)
	set, err := loader.Load()
	if err != nil {
		log.Fatal().Err(err).Str("file", cfg.TargetsFile).Msg("could not read the target list")
	}
	if len(set.Targets) == 0 {
		log.Fatal().Str("file", cfg.TargetsFile).Msg("target list parsed but contained no domains")
	}

	log.Info().
		Str("addr", cfg.HTTPAddr).
		Str("targets_file", cfg.TargetsFile).
		Str("db", cfg.DBPath).
		Int("endpoints", len(set.Targets)).
		Int("domains", len(set.Apexes)).
		Dur("http_interval", cfg.HTTPInterval).
		Msg("starting domain-monitor agent")

	// 2. History. SQLite keeps uptime percentages and incidents across restarts.
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatal().Err(err).Str("db", cfg.DBPath).Msg("could not open the history database")
	}
	defer st.Close()

	// 3. Live state, shared by the API and the WebSocket hub.
	snap := state.New(state.Thresholds{
		LatencyWarnMS:  cfg.LatencyWarnMS,
		LatencyCritMS:  cfg.LatencyCritMS,
		CertWarnDays:   cfg.CertWarnDays,
		CertCritDays:   cfg.CertCritDays,
		DomainWarnDays: cfg.DomainWarnDays,
		DomainCritDays: cfg.DomainCritDays,
		AcceptStatus:   cfg.AcceptStatus,
	})

	hub := websocket.NewHub(func() any { return snap.Full() })
	go hub.Run()

	tracker := incident.NewTracker(cfg.FailThreshold)

	// 4. Scheduler. Targets are installed before the warm start so the restored
	//    records have entries to attach to.
	var notifier notify.Notifier = notify.Nop{}
	if cfg.WebhookEnabled {
		notifier = notify.NewWebhookNotifier(cfg)
		log.Info().
			Str("webhook_url", cfg.WebhookURL).
			Str("channel", cfg.WebhookChannel).
			Str("capture_url", cfg.CaptureURL).
			Msg("monitoring alert webhook notifier enabled")
	}

	sched := scheduler.New(cfg, snap, st, tracker, hub, loader, notifier)
	sched.SetTargets(set)
	sched.WarmStart()

	ctx, cancel := context.WithCancel(context.Background())
	go sched.Run(ctx)

	// 5. HTTP and WebSocket surface.
	srv := &http.Server{
		Addr:        cfg.HTTPAddr,
		Handler:     handler.NewRouter(cfg, snap, st, hub, sched),
		ReadTimeout: 15 * time.Second,
		// No write timeout: a WebSocket connection is a long-lived write, and
		// a deadline here would cut every dashboard off on a timer.
		IdleTimeout: 60 * time.Second,
	}

	go func() {
		log.Info().Str("addr", cfg.HTTPAddr).Msg("HTTP and WebSocket server listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("HTTP server encountered a fatal error")
		}
	}()

	// 6. Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down gracefully...")

	cancel()
	hub.Stop()

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP server forced to shut down")
	}

	log.Info().Msg("domain-monitor exited cleanly")
}
