package main

import (
	"context"
	"e-commerce/pkg/metrics"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"e-commerce/pkg/rabbitmq"
	"e-commerce/services/inventory"
	"e-commerce/services/order"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 1. Configuración
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://user:password@localhost:5432/ecommerce_db"
	}
	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		rabbitURL = "amqp://guest:guest@localhost:5672/"
	}

	ctx := context.Background()
	workersCtx, cancelWorkers := context.WithCancel(ctx)
	defer cancelWorkers()

	// 2. Infra
	startupCtx, cancelStartup := context.WithTimeout(ctx, 45*time.Second)
	defer cancelStartup()

	var dbpool *pgxpool.Pool
	err := retryWithBackoff(startupCtx, "postgres connect", 6, func(attemptCtx context.Context) error {
		pool, err := pgxpool.New(attemptCtx, dbURL)
		if err != nil {
			return err
		}

		pingCtx, cancel := context.WithTimeout(attemptCtx, 3*time.Second)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			pool.Close()
			return err
		}

		dbpool = pool
		return nil
	})
	if err != nil {
		log.Fatalf("No se pudo conectar a PostgreSQL tras reintentos: %v", err)
	}

	err = retryWithBackoff(startupCtx, "schema init", 4, func(attemptCtx context.Context) error {
		opCtx, cancel := context.WithTimeout(attemptCtx, 5*time.Second)
		defer cancel()
		return ensureCoreSchema(opCtx, dbpool)
	})
	if err != nil {
		log.Fatalf("No se pudo preparar el schema base tras reintentos: %v", err)
	}

	var rabbitClient *rabbitmq.Client
	err = retryWithBackoff(startupCtx, "rabbitmq connect+topology", 6, func(attemptCtx context.Context) error {
		client, err := rabbitmq.NewClient(rabbitURL)
		if err != nil {
			return err
		}

		if err := client.SetupTopology(); err != nil {
			client.Close()
			return err
		}

		rabbitClient = client
		return nil
	})
	if err != nil {
		log.Fatalf("No se pudo conectar/configurar RabbitMQ tras reintentos: %v", err)
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
	go invConsumer.StartListening(workersCtx)
	go orderConsumer.StartListening(workersCtx)
	go orderRelay.Start(workersCtx)
	log.Println("Workers iniciados (Consumidores y Relay).")

	// 5. Servidor HTTP
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
	}))

	e.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{"status": "ok"})
	})
	e.GET("/metrics", func(c echo.Context) error {
		return c.JSON(200, metrics.Snapshot())
	})
	e.GET("/orders/:id", orderHandler.GetOrderByID)
	e.POST("/orders", orderHandler.CreateOrder)

	serverErrCh := make(chan error, 1)
	go func() {
		log.Printf("HTTP server listening on :%s", port)
		serverErrCh <- e.Start(":" + port)
	}()

	select {
	case <-rootCtx.Done():
		log.Println("Shutdown signal received, iniciando cierre ordenado...")
	case err := <-serverErrCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP server error: %v", err)
		}
	}

	// 1) Frenar workers que usan contexto
	cancelWorkers()

	// 2) Cerrar HTTP con timeout
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := e.Shutdown(shutdownCtx); err != nil {
		log.Printf("error during HTTP shutdown: %v", err)
	}

	// 3) Cerrar recursos compartidos (desbloquea consumers de Rabbit)
	rabbitClient.Close()
	dbpool.Close()
	log.Println("Shutdown completo.")
}

func ensureCoreSchema(ctx context.Context, dbpool *pgxpool.Pool) error {
	_, err := dbpool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS orders (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			product_id TEXT NOT NULL,
			quantity INT NOT NULL CHECK (quantity > 0),
			status TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS inventory (
			product_id TEXT PRIMARY KEY,
			quantity_available INT NOT NULL DEFAULT 0 CHECK (quantity_available >= 0),
			reserved_quantity INT NOT NULL DEFAULT 0 CHECK (reserved_quantity >= 0),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS outbox (
			id TEXT PRIMARY KEY,
			event_type TEXT NOT NULL,
			payload JSONB NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS processed_events (
			event_id TEXT NOT NULL,
			consumer TEXT NOT NULL,
			processed_at TIMESTAMP NOT NULL DEFAULT NOW(),
			PRIMARY KEY (event_id, consumer)
		);

		CREATE TABLE IF NOT EXISTS inventory_event_results (
			event_id TEXT PRIMARY KEY,
			order_id TEXT NOT NULL,
			product_id TEXT NOT NULL,
			outcome TEXT NOT NULL CHECK (outcome IN ('APPLIED', 'INSUFFICIENT')),
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_outbox_created_at ON outbox(created_at);
		CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
	`)
	return err
}

func retryWithBackoff(ctx context.Context, operation string, maxAttempts int, fn func(context.Context) error) error {
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := fn(ctx); err == nil {
			if attempt > 1 {
				log.Printf("%s recovered on attempt %d", operation, attempt)
			}
			metrics.Inc("startup_retry_success_total")
			return nil
		} else {
			lastErr = err
			metrics.Inc("startup_retry_error_total")
		}

		if attempt == maxAttempts {
			break
		}

		delay := startupBackoffDelay(attempt)
		log.Printf("%s attempt %d/%d failed: %v (retry in %s)", operation, attempt, maxAttempts, lastErr, delay)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	return lastErr
}

func startupBackoffDelay(attempt int) time.Duration {
	switch {
	case attempt <= 1:
		return 1 * time.Second
	case attempt == 2:
		return 2 * time.Second
	case attempt == 3:
		return 3 * time.Second
	default:
		return 5 * time.Second
	}
}
