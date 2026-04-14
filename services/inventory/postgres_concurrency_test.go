package inventory

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupInventoryPostgres(t *testing.T) (*pgxpool.Pool, context.Context) {
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

	t.Cleanup(func() {
		_ = pgContainer.Terminate(ctx)
	})

	dbURL, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	dbpool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)

	t.Cleanup(dbpool.Close)

	_, err = dbpool.Exec(ctx, `
		CREATE TABLE inventory (
			product_id TEXT PRIMARY KEY,
			quantity_available INT NOT NULL,
			reserved_quantity INT NOT NULL
		)
	`)
	require.NoError(t, err)

	return dbpool, ctx
}

func TestTryReserveStockConcurrent_NoOversell(t *testing.T) {
	dbpool, ctx := setupInventoryPostgres(t)
	repo := NewPostgresRepository(dbpool)

	const productID = "product-1"

	_, err := dbpool.Exec(ctx, `
		INSERT INTO inventory (product_id, quantity_available, reserved_quantity)
		VALUES ($1, $2, $3)
	`, productID, 2, 0)
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan bool, 2)
	errs := make(chan error, 2)

	var wg sync.WaitGroup
	wg.Add(2)

	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			<-start
			ok, reserveErr := repo.TryReserveStock(ctx, productID, 2)
			results <- ok
			errs <- reserveErr
		}()
	}

	close(start)
	wg.Wait()
	close(results)
	close(errs)

	successCount := 0
	for reserveErr := range errs {
		require.NoError(t, reserveErr)
	}
	for ok := range results {
		if ok {
			successCount++
		}
	}

	require.Equal(t, 1, successCount, "only one concurrent reservation must succeed")

	var available, reserved int
	err = dbpool.QueryRow(ctx, `
		SELECT quantity_available, reserved_quantity
		FROM inventory
		WHERE product_id = $1
	`, productID).Scan(&available, &reserved)
	require.NoError(t, err)
	require.Equal(t, 0, available)
	require.Equal(t, 2, reserved)
}
