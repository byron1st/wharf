package db_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// postgresImage matches the major version of the Supabase database.
const postgresImage = "postgres:17-alpine"

const testPassword = "test"

// server is the Postgres server of the container every test shares, as a URL
// without a password and database. Each test gets its own database on it.
var server url.URL

// TestMain starts one Postgres container for the package. Docker must be
// running: without it the tests fail rather than skip, so a missing daemon is
// never mistaken for a passing run.
func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()

	container, err := postgres.Run(ctx, postgresImage,
		postgres.WithUsername("postgres"),
		postgres.WithPassword(testPassword),
		postgres.WithDatabase("postgres"),
		postgres.BasicWaitStrategies(),
	)
	defer func() { _ = testcontainers.TerminateContainer(container) }()

	if err != nil {
		fmt.Fprintf(os.Stderr, "db tests need a running Docker daemon to start %s: %v\n", postgresImage, err)
		return 1
	}

	endpoint, err := container.PortEndpoint(ctx, "5432/tcp", "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to find the Postgres port: %v\n", err)
		return 1
	}

	server = url.URL{Scheme: "postgres", User: url.User("postgres"), Host: endpoint, RawQuery: "sslmode=disable"}

	return m.Run()
}

// newDatabase creates an empty database for t, dropped when t ends, and
// returns its URL without a password.
func newDatabase(t *testing.T) string {
	t.Helper()

	databaseCount++
	name := fmt.Sprintf("test_%d", databaseCount)
	admin := openURL(t, withPassword(server, "postgres"))

	_, err := admin.ExecContext(context.Background(), "CREATE DATABASE "+name)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)")
	})

	u := server
	u.Path = "/" + name

	return u.String()
}

// open connects to the database at dsn, a URL from newDatabase, with the
// wharf schema on the search dsn, so tests can read rows directly.
func open(t *testing.T, dsn string) *sql.DB {
	t.Helper()

	u, err := url.Parse(dsn)
	require.NoError(t, err)

	query := u.Query()
	query.Set("search_path", "wharf")
	u.RawQuery = query.Encode()

	return openURL(t, withPassword(*u, u.Path[1:]))
}

func openURL(t *testing.T, dsn string) *sql.DB {
	t.Helper()

	conn, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

// withPassword returns u with the test password and database name.
func withPassword(u url.URL, database string) string {
	u.User = url.UserPassword(u.User.Username(), testPassword)
	u.Path = "/" + database

	return u.String()
}

// databaseCount numbers the databases of newDatabase.
var databaseCount int
