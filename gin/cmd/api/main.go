// Command api runs the ticket API server and its maintenance subcommands.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/grandpine/ticket-api/internal/config"
	"github.com/grandpine/ticket-api/internal/server"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()
	switch cmd {
	case "serve":
		return serve(ctx, cfg, pool)
	default:
		return fmt.Errorf("unknown command %q (expected serve)", cmd)
	}
}

func serve(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) error {
	engine := server.New(server.Options{Pinger: pool, CORSOrigins: cfg.CORSOrigins})
	srv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.Port), Handler: engine, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	slog.Info("listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
