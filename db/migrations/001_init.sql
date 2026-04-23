-- 001_init.sql
-- Schema base para correr el sistema de e-commerce (SAGA + Outbox)

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
