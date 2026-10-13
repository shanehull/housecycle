// Command housecycle serves a dashboard of Australian housing buy signals:
// valuation, volume, stable prices, credit conditions, and the real cost of
// capital.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shanehull/housecycle/internal/buildinfo"
	"github.com/shanehull/housecycle/internal/cache"
	"github.com/shanehull/housecycle/internal/ingest"
	"github.com/shanehull/housecycle/internal/source"
	"github.com/shanehull/housecycle/internal/web"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.Error("housecycle exited", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	addr := flag.String("addr", env("HOUSECYCLE_ADDR", ":8080"), "HTTP listen address")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})).
		With(slog.String("version", buildinfo.GitTag))

	st := cache.New()

	cfg := source.Config{}
	fetchers := source.All()
	runIngest := func(ctx context.Context) error {
		results := ingest.Run(ctx, st, cfg, fetchers)
		var failed int
		for _, r := range results {
			if r.Err != nil {
				failed++
				log.WarnContext(ctx, "source failed", slog.String("source", r.Source), slog.Any("error", r.Err))
				continue
			}
			log.InfoContext(ctx, "source ok", slog.String("source", r.Source), slog.Int("series", r.Series))
		}
		if failed == len(results) {
			return errors.New("every source failed")
		}
		return nil
	}

	ingestCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	if err := runIngest(ingestCtx); err != nil {
		log.WarnContext(ctx, "initial ingest", slog.Any("error", err))
	}
	cancel()

	srv, err := web.New(st, runIngest, web.Options{
		Addr:   *addr,
		Logger: log,
	})
	if err != nil {
		return err
	}
	return srv.Run(ctx)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
