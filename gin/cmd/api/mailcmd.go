package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/grandpine/ticket-api/internal/attachment"
	"github.com/grandpine/ticket-api/internal/config"
	"github.com/grandpine/ticket-api/internal/inbound"
	"github.com/grandpine/ticket-api/internal/mail"
	"github.com/jackc/pgx/v5/pgxpool"
)

// buildMail returns the notifier and renderer for the configuration.
func buildMail(cfg config.Config) (mail.Notifier, *mail.Renderer) {
	r := mail.NewRenderer(time.Minute)
	if !cfg.Mail.Enabled {
		return mail.Disabled{}, r
	}
	return mail.NewNotifier(cfg.Mail, r), r
}

// startMailLoops runs the sender and poller until ctx is done. The returned
// func blocks until both have stopped. It is a no-op when mail is disabled.
func startMailLoops(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, store attachment.Storage, notifier mail.Notifier) func() {
	if !cfg.Mail.Enabled {
		return func() {}
	}
	m := cfg.Mail
	slog.Info("mail enabled", "smtp", fmt.Sprintf("%s:%d", m.SMTPHost, m.SMTPPort), "smtp_user", m.SMTPUser, "imap", fmt.Sprintf("%s:%d", m.IMAPHost, m.IMAPPort), "imap_user", m.IMAPUser, "folder", m.IMAPFolder)
	sender := mail.NewSender(pool, mail.NewSMTP(mail.SMTPOptions{Host: m.SMTPHost, Port: m.SMTPPort, TLS: m.SMTPTLS, User: m.SMTPUser, Password: m.SMTPPassword}), mail.Address{Name: m.FromName, Address: m.FromAddress}, 20)
	proc := inbound.NewProcessor(pool, store, notifier, inbound.Options{OwnAddress: m.FromAddress, DefaultDeptID: m.DefaultDeptID, MaxAttachment: cfg.MaxUploadBytes, AllowedMIME: cfg.AllowedMIME})
	poller := inbound.NewPoller(inbound.NewIMAP(inbound.IMAPOptions{Host: m.IMAPHost, Port: m.IMAPPort, TLS: m.IMAPTLS, User: m.IMAPUser, Password: m.IMAPPassword, Folder: m.IMAPFolder}), proc)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); sender.Run(ctx, m.SendInterval) }()
	go func() { defer wg.Done(); poller.Run(ctx, m.PollInterval) }()
	return wg.Wait
}

func mailWorker(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) error {
	if !cfg.Mail.Enabled {
		return errors.New("MAIL_ENABLED is not true")
	}
	store, err := attachment.NewLocalStorage(cfg.StorageDir)
	if err != nil {
		return err
	}
	notifier, _ := buildMail(cfg)
	wait := startMailLoops(ctx, cfg, pool, store, notifier)
	slog.Info("mail worker running")
	<-ctx.Done()
	wait()
	return nil
}

func parseMailTestFlags(args []string) (string, error) {
	fs := flag.NewFlagSet("mail-test", flag.ContinueOnError)
	to := fs.String("to", "", "recipient address")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if *to == "" {
		return "", errors.New("--to is required")
	}
	return *to, nil
}

// testMessage builds the fixed message mail-test sends.
func testMessage(m config.MailConfig, to string) ([]byte, error) {
	mid, err := mail.NewMessageID(nil, m.Domain)
	if err != nil {
		return nil, err
	}
	body := fmt.Sprintf("This is a test message from %s. If you can read this, outbound mail works.", m.SiteName)
	return mail.Build(mail.Outgoing{
		From: mail.Address{Name: m.FromName, Address: m.FromAddress}, To: mail.Address{Address: to},
		Subject: fmt.Sprintf("[%s] Test message", m.SiteName), MessageID: mid,
		Text: body, HTML: "<p>" + body + "</p>",
	})
}

func mailTest(ctx context.Context, cfg config.Config, args []string) error {
	to, err := parseMailTestFlags(args)
	if err != nil {
		return err
	}
	if !cfg.Mail.Enabled {
		return errors.New("MAIL_ENABLED is not true")
	}
	m := cfg.Mail
	raw, err := testMessage(m, to)
	if err != nil {
		return err
	}
	tr := mail.NewSMTP(mail.SMTPOptions{Host: m.SMTPHost, Port: m.SMTPPort, TLS: m.SMTPTLS, User: m.SMTPUser, Password: m.SMTPPassword})
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	sess, err := tr.Open(ctx)
	if err != nil {
		return err
	}
	defer sess.Close()
	if err := sess.Send(ctx, m.FromAddress, to, raw); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "sent test message to %s via %s:%d\n", to, m.SMTPHost, m.SMTPPort)
	return nil
}
