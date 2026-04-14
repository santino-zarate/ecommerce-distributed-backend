package order

import (
	"context"
	"e-commerce/pkg/rabbitmq"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Relay se encarga de publicar eventos pendientes en la tabla outbox.
type Relay struct {
	db           *pgxpool.Pool
	rabbitClient *rabbitmq.Client
}

// NewRelay crea una nueva instancia de Relay.
func NewRelay(db *pgxpool.Pool, rc *rabbitmq.Client) *Relay {
	return &Relay{db: db, rabbitClient: rc}
}

// Start arranca el bucle de procesamiento de outbox.
func (r *Relay) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.processOutbox(ctx)
		}
	}
}

func (r *Relay) processOutbox(ctx context.Context) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		log.Printf("error starting outbox tx: %v", err)
		return
	}
	defer tx.Rollback(ctx)

	// 1. Obtener eventos pendientes con lock para evitar doble procesamiento.
	rows, err := tx.Query(ctx, `
		SELECT id, event_type, payload
		FROM outbox
		ORDER BY created_at
		LIMIT 10
		FOR UPDATE SKIP LOCKED
	`)
	if err != nil {
		log.Printf("error querying outbox: %v", err)
		return
	}
	defer rows.Close()

	processedIDs := make([]string, 0, 10)

	for rows.Next() {
		var id, eventType string
		var payload []byte
		if err := rows.Scan(&id, &eventType, &payload); err != nil {
			log.Printf("error scanning outbox row: %v", err)
			continue
		}

		// 2. Publicar en RabbitMQ usando el ID como correlation_id
		headers := map[string]interface{}{
			"correlation_id": id,
		}
		if err := r.rabbitClient.Publish(rabbitmq.OrdersExchange, eventType, payload, headers); err != nil {
			log.Printf("error publishing event %s: %v", id, err)
			continue
		}

		// 3. Borrar de la outbox (solo si se publicó)
		_, err = tx.Exec(ctx, "DELETE FROM outbox WHERE id = $1", id)
		if err != nil {
			log.Printf("error deleting outbox event %s: %v", id, err)
		} else {
			log.Printf("Event %s processed and deleted from outbox", id)
			processedIDs = append(processedIDs, id)
		}
	}

	if err := rows.Err(); err != nil {
		log.Printf("error iterating outbox rows: %v", err)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("error committing outbox tx (processed=%d): %v", len(processedIDs), err)
		return
	}
}
