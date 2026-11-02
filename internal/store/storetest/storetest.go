// Package storetest creates throwaway Postgres databases for tests.
package storetest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

var seq atomic.Int64

// URL creates an empty database on the server in CSDC_TEST_DATABASE_URL and
// returns its URL; the database is dropped when the test ends. The test is
// skipped when the variable is not set.
func URL(t testing.TB) string {
	t.Helper()
	base := os.Getenv("CSDC_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("CSDC_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)

	name := fmt.Sprintf("csdc_test_%d_%d", time.Now().UnixNano(), seq.Add(1))
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(ctx, base)
		if err != nil {
			return
		}
		defer c.Close(ctx)
		_, _ = c.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)")
	})

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	return u.String()
}
