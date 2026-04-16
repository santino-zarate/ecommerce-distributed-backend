package inventory

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) GetByProductID(ctx context.Context, productID string) (*Inventory, error) {
	query := `SELECT product_id, quantity_available, reserved_quantity FROM inventory WHERE product_id = $1`
	row := r.db.QueryRow(ctx, query, productID)

	var i Inventory
	err := row.Scan(&i.ProductID, &i.QuantityAvailable, &i.ReservedQuantity)
	if err != nil {
		return nil, fmt.Errorf("error querying inventory: %w", err)
	}
	return &i, nil
}

func (r *PostgresRepository) Update(ctx context.Context, i *Inventory) error {
	query := `UPDATE inventory SET quantity_available = $1, reserved_quantity = $2 WHERE product_id = $3`
	_, err := r.db.Exec(ctx, query, i.QuantityAvailable, i.ReservedQuantity, i.ProductID)
	if err != nil {
		return fmt.Errorf("error updating inventory: %w", err)
	}
	return nil
}

// TryReserveStock intenta reservar stock de forma atómica para evitar overselling.
// Devuelve true si la reserva se aplicó, false si no había stock suficiente.
func (r *PostgresRepository) TryReserveStock(ctx context.Context, productID string, qty int) (bool, error) {
	query := `
		UPDATE inventory
		SET quantity_available = quantity_available - $1,
		    reserved_quantity  = reserved_quantity + $1
		WHERE product_id = $2
		  AND quantity_available >= $1
	`

	cmd, err := r.db.Exec(ctx, query, qty, productID)
	if err != nil {
		return false, fmt.Errorf("error reserving stock atomically: %w", err)
	}

	return cmd.RowsAffected() == 1, nil
}

// ReserveStockOnce aplica idempotencia + reserva atómica en una sola transacción.
func (r *PostgresRepository) ReserveStockOnce(ctx context.Context, eventID, productID string, qty int) (ReserveResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return ReserveInsufficient, fmt.Errorf("error starting reserve tx: %w", err)
	}
	defer tx.Rollback(ctx)

	insertProcessed := `
		INSERT INTO processed_events (event_id, consumer, processed_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (event_id, consumer) DO NOTHING
	`
	insertCmd, err := tx.Exec(ctx, insertProcessed, eventID, "inventory-consumer")
	if err != nil {
		return ReserveInsufficient, fmt.Errorf("error registering processed event: %w", err)
	}
	if insertCmd.RowsAffected() == 0 {
		return ReserveDuplicate, nil
	}

	reserveQuery := `
		UPDATE inventory
		SET quantity_available = quantity_available - $1,
		    reserved_quantity  = reserved_quantity + $1
		WHERE product_id = $2
		  AND quantity_available >= $1
	`
	reserveCmd, err := tx.Exec(ctx, reserveQuery, qty, productID)
	if err != nil {
		return ReserveInsufficient, fmt.Errorf("error reserving stock atomically: %w", err)
	}
	if reserveCmd.RowsAffected() == 0 {
		if err := tx.Commit(ctx); err != nil {
			return ReserveInsufficient, fmt.Errorf("error committing insufficient reserve tx: %w", err)
		}
		return ReserveInsufficient, nil
	}

	if err := tx.Commit(ctx); err != nil {
		return ReserveInsufficient, fmt.Errorf("error committing reserve tx: %w", err)
	}

	return ReserveApplied, nil
}
