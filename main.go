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

	rabbitClient, err := rabbitmq.NewClient(rabbitURL)
	if err != nil {
		log.Fatalf("No se pudo conectar a RabbitMQ: %v", err)
	}
	defer rabbitClient.Close()

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
