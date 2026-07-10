CREATE TYPE order_status AS ENUM ('solicitado', 'enviado', 'recibido');

CREATE TABLE IF NOT EXISTS orders (
    id BIGSERIAL PRIMARY KEY,
    customer_name VARCHAR(120) NOT NULL,
    customer_email VARCHAR(180) NOT NULL,
    product_name VARCHAR(180) NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    shipping_address TEXT NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    status order_status NOT NULL DEFAULT 'solicitado',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    shipped_at TIMESTAMPTZ,
    received_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_orders_created_at
ON orders (created_at DESC);

CREATE INDEX IF NOT EXISTS idx_orders_status
ON orders (status);
