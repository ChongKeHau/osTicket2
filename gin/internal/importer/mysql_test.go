package importer

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/grandpine/ticket-api/internal/db/testutil"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/wait"
)

const fixturePrefix = "ost_"

var (
	mysqlOnce sync.Once
	mysqlDSNv string
	mysqlErr  error
)

// mysqlDSN starts one MySQL container per test binary, loaded with the osTicket fixture.
func mysqlDSN(t *testing.T) string {
	t.Helper()
	mysqlOnce.Do(func() {
		ctx := context.Background()
		mig, err := testutil.MigrationsDir()
		if err != nil {
			mysqlErr = err
			return
		}
		data := filepath.Join(filepath.Dir(mig), "testdata", "osticket-mysql")
		// mysql.WithScripts preserves each script's basename as its container filename, and
		// the image's docker-entrypoint runs /docker-entrypoint-initdb.d/* in alphabetical
		// glob order — so "fixtures.sql" would run before "schema.sql" regardless of the
		// order passed here. Use WithFiles with numbered container paths to force schema
		// before fixtures, while keeping the required host filenames.
		ctr, err := mysql.Run(ctx, "mysql:8.0",
			mysql.WithDatabase("osticket"),
			mysql.WithUsername("ost"),
			mysql.WithPassword("ost"),
			testcontainers.WithFiles(
				testcontainers.ContainerFile{
					HostFilePath:      filepath.Join(data, "schema.sql"),
					ContainerFilePath: "/docker-entrypoint-initdb.d/01-schema.sql",
					FileMode:          0o755,
				},
				testcontainers.ContainerFile{
					HostFilePath:      filepath.Join(data, "fixtures.sql"),
					ContainerFilePath: "/docker-entrypoint-initdb.d/02-fixtures.sql",
					FileMode:          0o755,
				},
			),
			testcontainers.WithWaitStrategy(wait.ForLog("port: 3306  MySQL Community Server").WithStartupTimeout(180*time.Second)),
		)
		if err != nil {
			mysqlErr = err
			return
		}
		mysqlDSNv, mysqlErr = ctr.ConnectionString(ctx)
	})
	if mysqlErr != nil {
		t.Fatalf("mysql container: %v", mysqlErr)
	}
	return mysqlDSNv
}

func openTestSource(t *testing.T) *Source {
	t.Helper()
	s, err := OpenSource(context.Background(), mysqlDSN(t), fixturePrefix, time.UTC)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
