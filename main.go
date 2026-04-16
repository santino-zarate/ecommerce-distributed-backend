package main

import (
	"context"
	"log"
	"os"

	"e-commerce/pkg/rabbitmq"
	"e-commerce/services/inventory"
	"e-commerce/services/order"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	// 1. Configuración
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://user:password@localhost:5432/ecommerce_db"
	}
	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		rabbitURL = "amqp://guest:guest@localhost:5672/"
	}

	ctx := context.Background()

	// 2. Infra
	dbpool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("No se pudo conectar a la base de datos: %v", err)
	}
	defer dbpool.Close()

	if err := ensureInfraTables(ctx, dbpool); err != nil {
		log.Fatalf("No se pudo preparar tablas de infraestructura: %v", err)
	}

	rabbitClient, err := rabbitmq.NewClient(rabbitURL)
	if err != nil {
		log.Fatalf("No se pudo conectar a RabbitMQ: %v", err)
	}
	defer rabbitClient.Close()

	if err := rabbitClient.SetupTopology(); err != nil {
		log.Fatalf("No se pudo configurar la topología de RabbitMQ: %v", err)
	}

	// 3. Wiring
	orderRepo := order.NewPostgresRepository(dbpool)
	orderService := order.NewService(orderRepo, rabbitClient)
	orderHandler := order.NewHandler(orderService)
	orderConsumer := order.NewConsumer(rabbitClient, orderService)
	orderRelay := order.NewRelay(dbpool, rabbitClient)

	invRepo := inventory.NewPostgresRepository(dbpool)
	invService := inventory.NewService(invRepo, rabbitClient)
	invConsumer := inventory.NewConsumer(rabbitClient, invService)

	// 4. Arrancar workers en segundo plano
	go invConsumer.StartListening()
	go orderConsumer.StartListening()
	go orderRelay.Start(ctx)
	log.Println("Workers iniciados (Consumidores y Relay).")

	// 5. Servidor HTTP
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.POST("/orders", orderHandler.CreateOrder)

	log.Fatal(e.Start(":8080"))
}

func ensureInfraTables(ctx context.Context, dbpool *pgxpool.Pool) error {
	_, err := dbpool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS processed_events (
			event_id TEXT NOT NULL,
			consumer TEXT NOT NULL,
			processed_at TIMESTAMP NOT NULL DEFAULT NOW(),
			PRIMARY KEY (event_id, consumer)
		)
	`)
	return err
}
