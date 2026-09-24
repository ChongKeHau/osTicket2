// Package testutil starts one Postgres container per test binary, applies the
// Flyway migration files in order, and hands each test a rolled-back transaction.
package testutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	once     sync.Once
	pool     *pgxpool.Pool
	startErr error
)

// Pool returns the shared migrated pool, starting the container on first use.
func Pool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	once.Do(func() { pool, startErr = start(context.Background()) })
	if startErr != nil {
		t.Fatalf("test database: %v", startErr)
	}
	return pool
}

// Tx begins a transaction that is rolled back when the test ends.
func Tx(t testing.TB) pgx.Tx {
	t.Helper()
	tx, err := Pool(t).Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

func start(ctx context.Context) (*pgxpool.Pool, error) {
	ctr, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("ticket"),
		postgres.WithUsername("ticket"),
		postgres.WithPassword("ticket"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(90*time.Second)),
	)
	if err != nil {
		return nil, fmt.Errorf("start postgres: %w", err)
	}
	url, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, err
	}
	p, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := applyMigrations(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

var versionRe = regexp.MustCompile(`^V(\d+)__.*\.sql$`)

func applyMigrations(ctx context.Context, p *pgxpool.Pool) error {
	dir, err := MigrationsDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	type mig struct {
		v    int
		name string
	}
	var migs []mig
	for _, e := range entries {
		m := versionRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		v, _ := strconv.Atoi(m[1])
		migs = append(migs, mig{v, e.Name()})
	}
	sort.Slice(migs, func(i, j int) bool { return migs[i].v < migs[j].v })
	for _, m := range migs {
		sql, err := os.ReadFile(filepath.Join(dir, m.name))
		if err != nil {
			return err
		}
		if _, err := p.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("apply %s: %w", m.name, err)
		}
	}
	return nil
}

// MigrationsDir walks up from the working directory to find db/migrations.
func MigrationsDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		cand := filepath.Join(wd, "db", "migrations")
		if st, err := os.Stat(cand); err == nil && st.IsDir() {
			return cand, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return "", errors.New("db/migrations not found above working directory")
		}
		wd = parent
	}
}
