package order

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository implementa la interfaz Repository usando pgxpool.
type PostgresRepository struct {
	db *pgxpool.Pool
}

// NewPostgresRepository crea una nueva instancia.
func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// Save persiste la orden en Postgres.
func (r *PostgresRepository) Save(ctx context.Context, o *Order) error {
	query := `INSERT INTO orders (id, user_id, product_id, quantity, status) VALUES ($1, $2, $3, $4, $5)`
	_, err := r.db.Exec(ctx, query, o.ID, o.UserID, o.ProductID, o.Quantity, o.Status)
	if err != nil {
		return fmt.Errorf("error saving order: %w", err)
	}
	return nil
}

// SaveTransactional guarda orden y evento en una sola transacción.
func (r *PostgresRepository) SaveTransactional(ctx context.Context, o *Order, eventType string, eventPayload interface{}) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. Guardar Orden
	queryOrder := `INSERT INTO orders (id, user_id, product_id, quantity, status) VALUES ($1, $2, $3, $4, $5)`
	_, err = tx.Exec(ctx, queryOrder, o.ID, o.UserID, o.ProductID, o.Quantity, o.Status)
	if err != nil {
		return err
	}

	// 2. Guardar en Outbox
	payload, _ := json.Marshal(eventPayload)
	queryOutbox := `INSERT INTO outbox (id, event_type, payload, created_at) VALUES ($1, $2, $3, NOW())`
	_, err = tx.Exec(ctx, queryOutbox, uuid.New().String(), eventType, payload)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetByID busca una orden por ID en Postgres.
func (r *PostgresRepository) GetByID(ctx context.Context, id string) (*Order, error) {
	query := `SELECT id, user_id, product_id, quantity, status FROM orders WHERE id = $1`
	row := r.db.QueryRow(ctx, query, id)

	var o Order
	err := row.Scan(&o.ID, &o.UserID, &o.ProductID, &o.Quantity, &o.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("error querying order: %w", err)
	}
	return &o, nil
}

// UpdateStatus actualiza el estado de la orden en Postgres.
func (r *PostgresRepository) UpdateStatus(ctx context.Context, id string, status OrderStatus) error {
	query := `UPDATE orders SET status = $1 WHERE id = $2`
	_, err := r.db.Exec(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("error updating order status: %w", err)
	}
	return nil
}

// ApplyInventoryResultOnce aplica idempotencia por evento y actualiza el estado final de orden.
func (r *PostgresRepository) ApplyInventoryResultOnce(ctx context.Context, eventID, orderID string, targetStatus OrderStatus, consumer string) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("error starting apply-result tx: %w", err)
	}
	defer tx.Rollback(ctx)

	insertProcessed := `
		INSERT INTO processed_events (event_id, consumer, processed_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (event_id, consumer) DO NOTHING
	`
	insertCmd, err := tx.Exec(ctx, insertProcessed, eventID, consumer)
	if err != nil {
		return false, fmt.Errorf("error registering processed event: %w", err)
	}
	if insertCmd.RowsAffected() == 0 {
		return false, nil
	}

	_, err = tx.Exec(ctx,
		`UPDATE orders SET status = $1 WHERE id = $2 AND status = $3`,
		targetStatus,
		orderID,
		StatusPending,
	)
	if err != nil {
		return false, fmt.Errorf("error applying inventory result to order: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("error committing apply-result tx: %w", err)
	}

	return true, nil
}
