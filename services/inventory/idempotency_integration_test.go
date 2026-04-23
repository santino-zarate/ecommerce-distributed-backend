package inventory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReserveStockOnce_Idempotent(t *testing.T) {
	dbpool, ctx := setupInventoryPostgres(t)
	repo := NewPostgresRepository(dbpool)

	_, err := dbpool.Exec(ctx, `
		CREATE TABLE processed_events (
			event_id TEXT NOT NULL,
			consumer TEXT NOT NULL,
			processed_at TIMESTAMP NOT NULL DEFAULT NOW(),
			PRIMARY KEY (event_id, consumer)
		)
	`)
	require.NoError(t, err)

	_, err = dbpool.Exec(ctx, `
		CREATE TABLE inventory_event_results (
			event_id TEXT PRIMARY KEY,
			order_id TEXT NOT NULL,
			product_id TEXT NOT NULL,
			outcome TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		)
	`)
	require.NoError(t, err)

	const productID = "product-idempotent-1"
	_, err = dbpool.Exec(ctx, `
		INSERT INTO inventory (product_id, quantity_available, reserved_quantity)
		VALUES ($1, $2, $3)
	`, productID, 5, 0)
	require.NoError(t, err)

	// 1) Primera vez: aplica reserva
	result, duplicate, err := repo.ReserveStockOnce(ctx, "evt-1", "order-1", productID, 3)
	require.NoError(t, err)
	require.Equal(t, ReserveApplied, result)
	require.False(t, duplicate)

	// 2) Duplicado del mismo evento: no debe volver a reservar
	result, duplicate, err = repo.ReserveStockOnce(ctx, "evt-1", "order-1", productID, 3)
	require.NoError(t, err)
	require.Equal(t, ReserveApplied, result)
	require.True(t, duplicate)

	var available, reserved int
	err = dbpool.QueryRow(ctx, `
		SELECT quantity_available, reserved_quantity
		FROM inventory
		WHERE product_id = $1
	`, productID).Scan(&available, &reserved)
	require.NoError(t, err)
	require.Equal(t, 2, available)
	require.Equal(t, 3, reserved)

	// 3) Nuevo evento sin stock suficiente: debe fallar sin cambiar stock
	result, duplicate, err = repo.ReserveStockOnce(ctx, "evt-2", "order-2", productID, 3)
	require.NoError(t, err)
	require.Equal(t, ReserveInsufficient, result)
	require.False(t, duplicate)

	err = dbpool.QueryRow(ctx, `
		SELECT quantity_available, reserved_quantity
		FROM inventory
		WHERE product_id = $1
	`, productID).Scan(&available, &reserved)
	require.NoError(t, err)
	require.Equal(t, 2, available)
	require.Equal(t, 3, reserved)
}
