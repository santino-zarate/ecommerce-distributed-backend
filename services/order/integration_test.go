package order

import (
	"context"
	"testing"
	"time"

	"e-commerce/pkg/rabbitmq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
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
	assert.NoError(t, err)
	defer pgContainer.Terminate(ctx)

	dbURL, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	assert.NoError(t, err)

	// Inicializamos DB
	dbpool, err := pgxpool.New(ctx, dbURL)
	assert.NoError(t, err)
	defer dbpool.Close()

	// Creamos la tabla
	_, err = dbpool.Exec(ctx, `CREATE TABLE orders (
		id TEXT PRIMARY KEY,
		user_id TEXT,
		product_id TEXT,
		quantity INT,
		status TEXT
	)`)
	assert.NoError(t, err)

	// 2. Levantamos RabbitMQ con wait strategy
	rabbitContainer, err := rmq.RunContainer(ctx,
		testcontainers.WithImage("rabbitmq:3-management"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("Server startup complete").
				WithStartupTimeout(15*time.Second)),
	)
	assert.NoError(t, err)
	defer rabbitContainer.Terminate(ctx)

	rabbitURL, err := rabbitContainer.AmqpURL(ctx)
	assert.NoError(t, err)

	rabbitClient, err := rabbitmq.NewClient(rabbitURL)
	assert.NoError(t, err)
	defer rabbitClient.Close()

	// 3. Ejecutamos Test
	repo := NewPostgresRepository(dbpool)
	service := NewService(repo, rabbitClient)

	order := &Order{
		UserID:    "550e8400-e29b-41d4-a716-446655440000",
		ProductID: "660e8400-e29b-41d4-a716-446655440000",
		Quantity:  1,
	}

	err = service.CreateOrder(ctx, order)
	assert.NoError(t, err)

	// Verificamos en DB
	savedOrder, err := repo.GetByID(ctx, order.ID)
	assert.NoError(t, err)
	assert.Equal(t, order.ID, savedOrder.ID)
}
