package order

import (
	"context"
	"e-commerce/pkg/logx"
	"e-commerce/pkg/metrics"
	"e-commerce/pkg/rabbitmq"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Relay se encarga de publicar eventos pendientes en la tabla outbox.
type Relay struct {
	db           *pgxpool.Pool
	rabbitClient *rabbitmq.Client
}

const outboxProcessTimeout = 5 * time.Second

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
	processCtx, cancel := context.WithTimeout(ctx, outboxProcessTimeout)
	defer cancel()

	tx, err := r.db.Begin(processCtx)
	if err != nil {
		metrics.Inc("outbox_relay_tx_start_error_total")
		logx.Error("error starting outbox tx", err, map[string]interface{}{
			"component": "outbox-relay",
		})
		return
	}
	defer tx.Rollback(processCtx)

	// 1. Obtener eventos pendientes con lock para evitar doble procesamiento.
	rows, err := tx.Query(processCtx, `
		SELECT id, event_type, payload
		FROM outbox
		ORDER BY created_at
		LIMIT 10
		FOR UPDATE SKIP LOCKED
	`)
	if err != nil {
		metrics.Inc("outbox_relay_query_error_total")
		logx.Error("error querying outbox", err, map[string]interface{}{
			"component": "outbox-relay",
		})
		return
	}
	defer rows.Close()

	processedIDs := make([]string, 0, 10)

	for rows.Next() {
		var id, eventType string
		var payload []byte
		if err := rows.Scan(&id, &eventType, &payload); err != nil {
			metrics.Inc("outbox_relay_scan_error_total")
			logx.Error("error scanning outbox row", err, map[string]interface{}{
				"component": "outbox-relay",
			})
			continue
		}

		// 2. Publicar en RabbitMQ usando el ID como correlation_id
		headers := map[string]interface{}{
			"correlation_id": id,
		}
		if err := r.rabbitClient.Publish(rabbitmq.OrdersExchange, eventType, payload, headers); err != nil {
			metrics.Inc("outbox_relay_publish_error_total")
			logx.Error("error publishing outbox event", err, map[string]interface{}{
				"component":      "outbox-relay",
				"event_id":       id,
				"event_type":     eventType,
				"correlation_id": id,
			})
			continue
		}

		// 3. Borrar de la outbox (solo si se publicó)
		_, err = tx.Exec(processCtx, "DELETE FROM outbox WHERE id = $1", id)
		if err != nil {
			metrics.Inc("outbox_relay_delete_error_total")
			logx.Error("error deleting outbox event", err, map[string]interface{}{
				"component":      "outbox-relay",
				"event_id":       id,
				"event_type":     eventType,
				"correlation_id": id,
			})
		} else {
			metrics.Inc("outbox_relay_published_total")
			logx.Info("outbox event published and deleted", map[string]interface{}{
				"component":      "outbox-relay",
				"event_id":       id,
				"event_type":     eventType,
				"correlation_id": id,
			})
			processedIDs = append(processedIDs, id)
		}
	}

	if err := rows.Err(); err != nil {
		metrics.Inc("outbox_relay_iter_error_total")
		logx.Error("error iterating outbox rows", err, map[string]interface{}{
			"component": "outbox-relay",
		})
		return
	}

	if err := tx.Commit(processCtx); err != nil {
		metrics.Inc("outbox_relay_commit_error_total")
		logx.Error("error committing outbox tx", err, map[string]interface{}{
			"component": "outbox-relay",
			"processed": len(processedIDs),
		})
		return
	}

	if len(processedIDs) > 0 {
		metrics.Add("outbox_relay_batch_processed_total", int64(len(processedIDs)))
		logx.Info("outbox batch committed", map[string]interface{}{
			"component": "outbox-relay",
			"processed": len(processedIDs),
		})
	}
}
