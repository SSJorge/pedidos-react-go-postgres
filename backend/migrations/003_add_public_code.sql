ALTER TABLE orders
ADD COLUMN IF NOT EXISTS public_code VARCHAR(20);

CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_public_code
ON orders (public_code);