package order

import (
	"context"
	"encoding/json"
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

func setupRelayInfra(t *testing.T) (context.Context, *pgxpool.Pool, *rabbitmq.Client) {
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
		t.Skipf("docker/testcontainers unavailable for postgres, skipping: %v", err)
	}
	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	dbURL, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	dbpool, err := pgxpool.New(ctx, dbURL)
	require.NoError(t, err)
	t.Cleanup(dbpool.Close)

	_, err = dbpool.Exec(ctx, `
		CREATE TABLE outbox (
			id TEXT PRIMARY KEY,
			event_type TEXT NOT NULL,
			payload JSONB NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		)
	`)
	require.NoError(t, err)

	rabbitContainer, err := rmq.RunContainer(ctx,
		testcontainers.WithImage("rabbitmq:3-management"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("Server startup complete").
				WithStartupTimeout(20*time.Second)),
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

	return ctx, dbpool, rabbitClient
}

func TestRelayProcessOutbox_PublishesAndDeletes(t *testing.T) {
	ctx, dbpool, rabbitClient := setupRelayInfra(t)

	relay := NewRelay(dbpool, rabbitClient)

	outboxID := "evt-1"
	payloadMap := map[string]any{
		"orderId": "550e8400-e29b-41d4-a716-446655440000",
	}
	payload, err := json.Marshal(payloadMap)
	require.NoError(t, err)

	_, err = dbpool.Exec(ctx, `
		INSERT INTO outbox (id, event_type, payload, created_at)
		VALUES ($1, $2, $3, NOW())
	`, outboxID, rabbitmq.OrderCreatedKey, payload)
	require.NoError(t, err)

	deliveries, err := rabbitClient.Consume(rabbitmq.InventoryQueue, "relay-test-consumer")
	require.NoError(t, err)

	relay.processOutbox(ctx)

	var remaining int
	err = dbpool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox WHERE id = $1`, outboxID).Scan(&remaining)
	require.NoError(t, err)
	require.Equal(t, 0, remaining, "relay should delete outbox row after successful publish")

	select {
	case msg := <-deliveries:
		require.Equal(t, rabbitmq.OrderCreatedKey, msg.RoutingKey)
		require.JSONEq(t, string(payload), string(msg.Body))
		require.Equal(t, outboxID, msg.Headers["correlation_id"])
		require.NoError(t, msg.Ack(false))
	case <-time.After(5 * time.Second):
		t.Fatal("expected message from relay publish, but timed out")
	}
}
