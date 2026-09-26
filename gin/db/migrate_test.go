package migrations

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moby/moby/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestFlywayMigrateTwice(t *testing.T) {
	ctx := context.Background()
	net, err := network.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = net.Remove(ctx) })

	pg, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("ticket"), postgres.WithUsername("ticket"), postgres.WithPassword("ticket"),
		network.WithNetwork([]string{"pg"}, net),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(90*time.Second)),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	dir, err := filepath.Abs("migrations")
	if err != nil {
		t.Fatal(err)
	}
	runFlyway := func() string {
		c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image: "flyway/flyway:10",
				Cmd: []string{"-url=jdbc:postgresql://pg:5432/ticket", "-user=ticket", "-password=ticket",
					"-locations=filesystem:/flyway/sql", "-connectRetries=10", "migrate"},
				Networks: []string{net.Name},
				HostConfigModifier: func(hc *container.HostConfig) {
					hc.Binds = append(hc.Binds, dir+":/flyway/sql:ro")
				},
				WaitingFor: wait.ForExit().WithExitTimeout(3 * time.Minute),
			},
			Started: true,
		})
		if err != nil {
			t.Fatalf("flyway: %v", err)
		}
		defer func() { _ = c.Terminate(ctx) }()
		rc, err := c.Logs(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		b, _ := io.ReadAll(rc)
		return string(b)
	}

	first := runFlyway()
	if !strings.Contains(first, "Successfully applied 5 migrations") {
		t.Fatalf("first run:\n%s", first)
	}
	second := runFlyway()
	if !strings.Contains(second, "No migration necessary") {
		t.Fatalf("second run:\n%s", second)
	}

	url, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var applied int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM flyway_schema_history WHERE success`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 5 {
		t.Fatalf("expected 5 successful migrations in history, got %d", applied)
	}
	var tables int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('ticket','staff','department','thread_entry','file','attachment','end_user')`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 7 {
		t.Fatalf("expected 7 core tables, got %d", tables)
	}
	var idx string
	if err := conn.QueryRow(ctx, `SELECT indexdef FROM pg_indexes WHERE tablename = 'ticket_event' AND indexname = 'ticket_event_created_idx'`).Scan(&idx); err != nil {
		t.Fatalf("ticket_event_created_idx: %v", err)
	}
	if !strings.Contains(idx, "(created_at)") {
		t.Fatalf("ticket_event_created_idx should cover created_at, got %s", idx)
	}
	var endUserIdx string
	if err := conn.QueryRow(ctx, `SELECT indexdef FROM pg_indexes WHERE tablename = 'end_user' AND indexname = 'end_user_email_idx'`).Scan(&endUserIdx); err != nil {
		t.Fatalf("end_user_email_idx: %v", err)
	}
	if !strings.Contains(endUserIdx, "lower(email)") {
		t.Fatalf("end_user_email_idx should cover lower(email), got %s", endUserIdx)
	}
}
