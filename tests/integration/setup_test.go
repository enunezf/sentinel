package integration_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	redisv9 "github.com/redis/go-redis/v9"
)

var (
	testDB          *pgxpool.Pool
	testRedisClient *redisv9.Client
	pgContainer     *tcpostgres.PostgresContainer
	redisContainer  *tcredis.RedisContainer
)

// TestMain starts real PostgreSQL 15 and Redis 7 containers, applies migrations,
// runs all integration tests, then tears down the containers.
func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error

	// ---- PostgreSQL container ----
	pgContainer, err = tcpostgres.Run(ctx,
		"postgres:15-alpine",
		tcpostgres.WithDatabase("sentinel_test"),
		tcpostgres.WithUsername("sentinel"),
		tcpostgres.WithPassword("sentinel_test"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: start postgres container: %v\n", err)
		os.Exit(1)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: get connection string: %v\n", err)
		os.Exit(1)
	}

	testDB, err = pgxpool.New(ctx, connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: connect to postgres: %v\n", err)
		os.Exit(1)
	}

	// Wait for the DB to accept connections.
	for i := 0; i < 20; i++ {
		if err := testDB.Ping(ctx); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Apply migrations.
	if err := runMigrations(ctx, testDB); err != nil {
		fmt.Fprintf(os.Stderr, "integration: run migrations: %v\n", err)
		os.Exit(1)
	}

	// ---- Redis container ----
	redisContainer, err = tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: start redis container: %v\n", err)
		os.Exit(1)
	}

	redisEndpoint, err := redisContainer.Endpoint(ctx, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: get redis endpoint: %v\n", err)
		os.Exit(1)
	}

	testRedisClient = redisv9.NewClient(&redisv9.Options{
		Addr: redisEndpoint,
	})

	// Verify Redis connectivity.
	if _, err := testRedisClient.Ping(ctx).Result(); err != nil {
		fmt.Fprintf(os.Stderr, "integration: ping redis: %v\n", err)
		os.Exit(1)
	}

	// Run tests.
	code := m.Run()

	// Teardown.
	_ = testDB.Close
	testDB.Close()
	_ = testRedisClient.Close()
	_ = pgContainer.Terminate(ctx)
	_ = redisContainer.Terminate(ctx)

	os.Exit(code)
}

// runMigrations executes the SQL migration files against the test database.
func runMigrations(ctx context.Context, db *pgxpool.Pool) error {
	migrations := []string{
		"../../migrations/001_initial_schema.sql",
		"../../migrations/002_seed_permissions.sql",
	}

	for _, path := range migrations {
		sql, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", path, err)
		}
		if _, err := db.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("exec migration %s: %w", path, err)
		}
	}
	return nil
}

// truncateTable deletes all rows from a table (for test isolation).
func truncateTable(ctx context.Context, t *testing.T, table string) {
	t.Helper()
	_, err := testDB.Exec(ctx, "TRUNCATE TABLE "+table+" CASCADE")
	if err != nil {
		t.Fatalf("truncate %s: %v", table, err)
	}
}
