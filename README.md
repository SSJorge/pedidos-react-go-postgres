# Proyecto básico de pedidos

MVP con:

- React + Vite
- Go
- PostgreSQL
- Sin pagos
- Sin autenticación
- Estados manuales: `solicitado`, `enviado`, `recibido`

## Flujo

1. Un cliente registra una solicitud.
2. La solicitud aparece en el panel.
3. El administrador la marca como enviada.
4. El administrador la marca como recibida.

## 1. Iniciar PostgreSQL

Desde la raíz:

```bash
docker compose up -d
```

La base de datos queda disponible en:

```text
postgres://postgres:postgres@localhost:5432/pedidos
```

## 2. Iniciar backend

```bash
cd backend
go mod tidy
go run ./cmd/api
```

Backend:

```text
http://localhost:8080
```

## 3. Iniciar frontend

En otra terminal:

```bash
cd frontend
npm install
npm run dev
```

Frontend:

```text
http://localhost:5173
```

## API

### Crear pedido

```http
POST /api/orders
Content-Type: application/json
```

```json
{
  "customer_name": "Jorge Moreno",
  "customer_email": "jorge@example.com",
  "product_name": "Teclado mecánico",
  "quantity": 1,
  "shipping_address": "Valparaíso, Chile",
  "notes": "Entregar durante la tarde"
}
```

### Listar pedidos

```http
GET /api/orders
```

### Cambiar estado

```http
PATCH /api/orders/{id}/status
Content-Type: application/json
```

```json
{
  "status": "enviado"
}
```

Estados permitidos:

- `solicitado`
- `enviado`
- `recibido`
