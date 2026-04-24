-- 001_demo.sql
-- Datos demo reproducibles para portfolio (flujo éxito/fallo de saga)

-- IDs estables para pruebas de demo
-- user_id demo:    11111111-1111-1111-1111-111111111111
-- product_id OK:   22222222-2222-2222-2222-222222222222 (con stock)
-- product_id FAIL: 33333333-3333-3333-3333-333333333333 (sin stock)

INSERT INTO inventory (product_id, quantity_available, reserved_quantity, updated_at)
VALUES
  ('22222222-2222-2222-2222-222222222222', 999, 0, NOW()),
  ('33333333-3333-3333-3333-333333333333', 0, 0, NOW())
ON CONFLICT (product_id) DO UPDATE SET
  quantity_available = EXCLUDED.quantity_available,
  reserved_quantity = EXCLUDED.reserved_quantity,
  updated_at = NOW();

