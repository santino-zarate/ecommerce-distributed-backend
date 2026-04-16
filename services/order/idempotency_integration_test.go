package order

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupOrderPostgresForIdempotency(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	pgContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:15-alpine"),
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("user"),
		postgres.WithPassword("password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(15*time.Second)),
	)
	if err != nil {
		t.Skipf("docker/testcontainers unavailable, skipping: %v", err)
	}

	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	dbURL, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	dbpool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	t.Cleanup(dbpool.Close)

	_, err = dbpool.Exec(ctx, `
		CREATE TABLE orders (
			id TEXT PRIMARY KEY,
			user_id TEXT,
			product_id TEXT,
			quantity INT,
			status TEXT
		)
	`)
	require.NoError(t, err)

	_, err = dbpool.Exec(ctx, `
		CREATE TABLE processed_events (
			event_id TEXT NOT NULL,
			consumer TEXT NOT NULL,
			processed_at TIMESTAMP NOT NULL DEFAULT NOW(),
			PRIMARY KEY (event_id, consumer)
		)
	`)
	require.NoError(t, err)

	return dbpool, ctx
}

func TestApplyInventoryResultOnce_Idempotent(t *testing.T) {
	dbpool, ctx := setupOrderPostgresForIdempotency(t)
	repo := NewPostgresRepository(dbpool)

	const orderID = "order-1"
	_, err := dbpool.Exec(ctx, `
		INSERT INTO orders (id, user_id, product_id, quantity, status)
		VALUES ($1, $2, $3, $4, $5)
	`, orderID, "u1", "p1", 1, StatusPending)
	require.NoError(t, err)

	// 1) Primera vez: aplica cambio de estado.
	processed, err := repo.ApplyInventoryResultOnce(ctx, "evt-ord-1", orderID, StatusCreated, "order-consumer")
	require.NoError(t, err)
	require.True(t, processed)

	// 2) Duplicado del mismo evento: no reaplica.
	processed, err = repo.ApplyInventoryResultOnce(ctx, "evt-ord-1", orderID, StatusCreated, "order-consumer")
	require.NoError(t, err)
	require.False(t, processed)

	var status OrderStatus
	err = dbpool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&status)
	require.NoError(t, err)
	require.Equal(t, StatusCreated, status)

	var processedCount int
	err = dbpool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM processed_events
		WHERE event_id = $1 AND consumer = $2
	`, "evt-ord-1", "order-consumer").Scan(&processedCount)
	require.NoError(t, err)
	require.Equal(t, 1, processedCount)
}
