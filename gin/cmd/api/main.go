// Command api runs the ticket API server and its maintenance subcommands.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/attachment"
	"github.com/grandpine/ticket-api/internal/auth"
	"github.com/grandpine/ticket-api/internal/config"
	"github.com/grandpine/ticket-api/internal/dashboard"
	"github.com/grandpine/ticket-api/internal/db"
	"github.com/grandpine/ticket-api/internal/dept"
	"github.com/grandpine/ticket-api/internal/mail"
	"github.com/grandpine/ticket-api/internal/server"
	"github.com/grandpine/ticket-api/internal/staff"
	"github.com/grandpine/ticket-api/internal/ticket"
	"github.com/grandpine/ticket-api/internal/topic"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	accessTTL  = 15 * time.Minute
	refreshTTL = 14 * 24 * time.Hour
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(os.Args[1:]); err != nil {
		var ee *exitError
		if errors.As(err, &ee) {
			if ee.err != nil {
				slog.Error("fatal", "err", ee.err)
			}
			os.Exit(ee.code)
		}
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
	switch cmd {
	case "serve", "create-admin", "gc-files", "import-osticket", "mail-worker", "mail-test":
	default:
		return fmt.Errorf("unknown command %q (expected serve, create-admin, gc-files, import-osticket, mail-worker or mail-test)", cmd)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if cmd == "import-osticket" {
		return importOsticket(ctx, args)
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	switch cmd {
	case "serve":
		return serve(ctx, cfg, pool)
	case "create-admin":
		return createAdmin(ctx, pool, args)
	case "gc-files":
		return gcFiles(ctx, cfg, pool, args)
	case "mail-worker":
		return mailWorker(ctx, cfg, pool)
	case "mail-test":
		return mailTest(ctx, cfg, args)
	default:
		return fmt.Errorf("unknown command %q (expected serve, create-admin, gc-files, import-osticket, mail-worker or mail-test)", cmd)
	}
}

func serve(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) error {
	store, err := attachment.NewLocalStorage(cfg.StorageDir)
	if err != nil {
		return err
	}
	tokens := auth.NewTokens(cfg.JWTSecret, accessTTL)
	authSvc := auth.NewService(pool, tokens, refreshTTL)
	authH := auth.NewHandler(authSvc)
	deptH := dept.NewHandler(dept.NewService(pool))
	topicH := topic.NewHandler(topic.NewService(pool))
	staffH := staff.NewHandler(staff.NewService(pool))
	dashH := dashboard.NewHandler(dashboard.NewService(pool))
	notifier, renderer := buildMail(cfg)
	ticketH := ticket.NewHandler(ticket.NewService(pool, ticket.WithNotifier(notifier)))
	mailH := mail.NewHandler(pool, renderer)
	fileH := attachment.NewHandler(attachment.NewService(pool, store, cfg.MaxUploadBytes, cfg.AllowedMIME), cfg.MaxUploadBytes)

	engine := server.New(server.Options{
		Pinger:         pool,
		CORSOrigins:    cfg.CORSOrigins,
		TrustedProxies: cfg.TrustedProxies,
		RequireAuth:    auth.RequireAuth(tokens, authSvc),
		Mount: func(public, private *gin.RouterGroup) {
			authH.Mount(public, private)
			deptH.Mount(private)
			topicH.Mount(private)
			staffH.Mount(private)
			ticketH.Mount(private)
			fileH.Mount(private)
			mailH.Mount(private)
			dashH.Mount(private)
		},
	})
	// The mail loops get their own cancel so every return path below (bind
	// failure, Shutdown error, normal exit) stops them and waits before run's
	// deferred pool.Close.
	mailCtx, cancelMail := context.WithCancel(ctx)
	waitMail := startMailLoops(mailCtx, cfg, pool, store, notifier)
	defer func() { cancelMail(); waitMail() }()
	srv := &http.Server{
		Addr: fmt.Sprintf(":%d", cfg.Port), Handler: engine,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
		// No WriteTimeout: file downloads stream and may legitimately take
		// longer than any single-request deadline we'd want to impose here.
	}
	done := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		done <- srv.Shutdown(shutdownCtx)
	}()
	slog.Info("listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	// ListenAndServe returns ErrServerClosed as soon as Shutdown is called,
	// while Shutdown itself is still draining in-flight requests in the
	// goroutine above. Wait for it to finish before returning, so callers
	// (main's pool.Close and process exit) don't run out from under it.
	if err := <-done; err != nil {
		return err
	}
	return nil
}

func createAdmin(ctx context.Context, pool *pgxpool.Pool, args []string) error {
	fs := flag.NewFlagSet("create-admin", flag.ContinueOnError)
	username := fs.String("username", "", "admin username")
	email := fs.String("email", "", "admin email")
	password := fs.String("password", "", "admin password (min 8 chars)")
	firstName := fs.String("first-name", "", "admin first name (defaults to username)")
	lastName := fs.String("last-name", "", "admin last name (defaults to empty)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *username == "" || *email == "" || *password == "" {
		return errors.New("create-admin requires --username, --email and --password")
	}
	id, err := auth.CreateAdmin(ctx, pool, *username, *email, *password, *firstName, *lastName)
	if err != nil {
		return err
	}
	fmt.Printf("created admin %s with id %d\n", *username, id)
	return nil
}

func gcFiles(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, args []string) error {
	fs := flag.NewFlagSet("gc-files", flag.ContinueOnError)
	olderThan := fs.Duration("older-than", 24*time.Hour, "delete unattached files older than this")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *olderThan < time.Hour {
		return errors.New("--older-than must be at least 1h")
	}
	store, err := attachment.NewLocalStorage(cfg.StorageDir)
	if err != nil {
		return err
	}
	svc := attachment.NewService(pool, store, cfg.MaxUploadBytes, cfg.AllowedMIME)
	n, err := svc.GC(ctx, *olderThan)
	if err != nil {
		return err
	}
	fmt.Printf("removed %d unattached files\n", n)
	return nil
}
