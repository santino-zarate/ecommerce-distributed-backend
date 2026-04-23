package order

import (
	"context"
	"testing"
	"time"

	"e-commerce/pkg/rabbitmq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	rmq "github.com/testcontainers/testcontainers-go/modules/rabbitmq"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestCreateOrderIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()

	// 1. Levantamos Postgres con wait strategy
	pgContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:15-alpine"),
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("user"),
		postgres.WithPassword("password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(10*time.Second)),
	)
	if err != nil {
		t.Skipf("docker/testcontainers unavailable for postgres, skipping: %v", err)
	}
	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	dbURL, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	// Inicializamos DB
	dbpool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	t.Cleanup(dbpool.Close)

	// Creamos tablas necesarias para SaveTransactional (orders + outbox).
	_, err = dbpool.Exec(ctx, `CREATE TABLE orders (
		id TEXT PRIMARY KEY,
		user_id TEXT,
		product_id TEXT,
		quantity INT,
		status TEXT
	)`)
	require.NoError(t, err)
	_, err = dbpool.Exec(ctx, `CREATE TABLE outbox (
		id TEXT PRIMARY KEY,
		event_type TEXT NOT NULL,
		payload JSONB NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT NOW()
	)`)
	require.NoError(t, err)

	// 2. Levantamos RabbitMQ con wait strategy
	rabbitContainer, err := rmq.RunContainer(ctx,
		testcontainers.WithImage("rabbitmq:3-management"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("Server startup complete").
				WithStartupTimeout(15*time.Second)),
	)
	if err != nil {
		t.Skipf("docker/testcontainers unavailable for rabbitmq, skipping: %v", err)
	}
	t.Cleanup(func() { _ = rabbitContainer.Terminate(ctx) })

	rabbitURL, err := rabbitContainer.AmqpURL(ctx)
	require.NoError(t, err)

	rabbitClient, err := rabbitmq.NewClient(rabbitURL)
	require.NoError(t, err)
	t.Cleanup(rabbitClient.Close)
	require.NoError(t, rabbitClient.SetupTopology())

	// 3. Ejecutamos Test
	repo := NewPostgresRepository(dbpool)
	service := NewService(repo, rabbitClient)

	order := &Order{
		UserID:    "550e8400-e29b-41d4-a716-446655440000",
		ProductID: "660e8400-e29b-41d4-a716-446655440000",
		Quantity:  1,
	}

	err = service.CreateOrder(ctx, order)
	require.NoError(t, err)

	// Verificamos en DB
	savedOrder, err := repo.GetByID(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, order.ID, savedOrder.ID)
}
