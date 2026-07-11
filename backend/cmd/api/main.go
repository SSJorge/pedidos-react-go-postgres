package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type application struct {
	db        *pgxpool.Pool
	jwtSecret []byte
}

type order struct {
	ID              int64      `json:"id"`
	CustomerName    string     `json:"customer_name"`
	CustomerEmail   string     `json:"customer_email"`
	ProductName     string     `json:"product_name"`
	Quantity        int        `json:"quantity"`
	ShippingAddress string     `json:"shipping_address"`
	Notes           string     `json:"notes"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	ShippedAt       *time.Time `json:"shipped_at"`
	ReceivedAt      *time.Time `json:"received_at"`
}

type createOrderInput struct {
	CustomerName    string `json:"customer_name"`
	CustomerEmail   string `json:"customer_email"`
	ProductName     string `json:"product_name"`
	Quantity        int    `json:"quantity"`
	ShippingAddress string `json:"shipping_address"`
	Notes           string `json:"notes"`
}

type updateStatusInput struct {
	Status string `json:"status"`
}

func main() {
	databaseURL := envOrDefault(
		"DATABASE_URL",
		"postgres://pedidos_admin:pedidos_clave_2026@127.0.0.1:55432/pedidos?sslmode=disable",
	)

	log.Println("DATABASE_URL utilizada:", databaseURL)

	db, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.Ping(ctx); err != nil {
		log.Fatal("no se pudo conectar a PostgreSQL: ", err)
	}

	jwtSecret := envOrDefault(
		"JWT_SECRET",
		"cambiar-esta-clave-en-produccion-por-una-muy-larga",
	)

	app := &application{
		db:        db,
		jwtSecret: []byte(jwtSecret),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", app.health)

	mux.HandleFunc("POST /api/auth/login", app.login)
	mux.HandleFunc("GET /api/auth/me", app.requireAuth(app.me))

	mux.HandleFunc(
		"GET /api/orders",
		app.requireAuth(app.listOrders),
	)

	mux.HandleFunc(
		"POST /api/orders",
		app.requireAuth(app.createOrder),
	)

	mux.HandleFunc(
		"PATCH /api/orders/{id}/status",
		app.requireAuth(app.updateOrderStatus),
	)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           corsMiddleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("API disponible en http://localhost:8080")
	log.Fatal(server.ListenAndServe())
}

func (app *application) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (app *application) listOrders(w http.ResponseWriter, r *http.Request) {
	rows, err := app.db.Query(r.Context(), `
		SELECT
			id,
			customer_name,
			customer_email,
			product_name,
			quantity,
			shipping_address,
			notes,
			status,
			created_at,
			shipped_at,
			received_at
		FROM orders
		ORDER BY created_at DESC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudieron obtener los pedidos")
		return
	}
	defer rows.Close()

	orders := make([]order, 0)

	for rows.Next() {
		var item order

		err := rows.Scan(
			&item.ID,
			&item.CustomerName,
			&item.CustomerEmail,
			&item.ProductName,
			&item.Quantity,
			&item.ShippingAddress,
			&item.Notes,
			&item.Status,
			&item.CreatedAt,
			&item.ShippedAt,
			&item.ReceivedAt,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "no se pudo leer un pedido")
			return
		}

		orders = append(orders, item)
	}

	writeJSON(w, http.StatusOK, orders)
}

func (app *application) createOrder(w http.ResponseWriter, r *http.Request) {
	var input createOrderInput

	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	input.CustomerName = strings.TrimSpace(input.CustomerName)
	input.CustomerEmail = strings.TrimSpace(input.CustomerEmail)
	input.ProductName = strings.TrimSpace(input.ProductName)
	input.ShippingAddress = strings.TrimSpace(input.ShippingAddress)
	input.Notes = strings.TrimSpace(input.Notes)

	switch {
	case input.CustomerName == "":
		writeError(w, http.StatusBadRequest, "el nombre del cliente es obligatorio")
		return
	case input.CustomerEmail == "":
		writeError(w, http.StatusBadRequest, "el correo del cliente es obligatorio")
		return
	case input.ProductName == "":
		writeError(w, http.StatusBadRequest, "el producto es obligatorio")
		return
	case input.Quantity <= 0:
		writeError(w, http.StatusBadRequest, "la cantidad debe ser mayor que cero")
		return
	case input.ShippingAddress == "":
		writeError(w, http.StatusBadRequest, "la dirección es obligatoria")
		return
	}

	var created order

	err := app.db.QueryRow(r.Context(), `
		INSERT INTO orders (
			customer_name,
			customer_email,
			product_name,
			quantity,
			shipping_address,
			notes
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING
			id,
			customer_name,
			customer_email,
			product_name,
			quantity,
			shipping_address,
			notes,
			status,
			created_at,
			shipped_at,
			received_at
	`,
		input.CustomerName,
		input.CustomerEmail,
		input.ProductName,
		input.Quantity,
		input.ShippingAddress,
		input.Notes,
	).Scan(
		&created.ID,
		&created.CustomerName,
		&created.CustomerEmail,
		&created.ProductName,
		&created.Quantity,
		&created.ShippingAddress,
		&created.Notes,
		&created.Status,
		&created.CreatedAt,
		&created.ShippedAt,
		&created.ReceivedAt,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudo crear el pedido")
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

func (app *application) updateOrderStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "ID inválido")
		return
	}

	var input updateStatusInput

	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	input.Status = strings.TrimSpace(input.Status)

	if input.Status != "solicitado" &&
		input.Status != "enviado" &&
		input.Status != "recibido" {
		writeError(w, http.StatusBadRequest, "estado no permitido")
		return
	}

	var updated order

	err = app.db.QueryRow(r.Context(), `
		UPDATE orders
		SET
			status = $2::order_status,
			shipped_at = CASE
				WHEN $2 = 'enviado' AND shipped_at IS NULL THEN NOW()
				WHEN $2 = 'solicitado' THEN NULL
				ELSE shipped_at
			END,
			received_at = CASE
				WHEN $2 = 'recibido' AND received_at IS NULL THEN NOW()
				WHEN $2 <> 'recibido' THEN NULL
				ELSE received_at
			END
		WHERE id = $1
		RETURNING
			id,
			customer_name,
			customer_email,
			product_name,
			quantity,
			shipping_address,
			notes,
			status,
			created_at,
			shipped_at,
			received_at
	`, id, input.Status).Scan(
		&updated.ID,
		&updated.CustomerName,
		&updated.CustomerEmail,
		&updated.ProductName,
		&updated.Quantity,
		&updated.ShippingAddress,
		&updated.Notes,
		&updated.Status,
		&updated.CreatedAt,
		&updated.ShippedAt,
		&updated.ReceivedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "pedido no encontrado")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudo actualizar el pedido")
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(destination)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Println("error escribiendo JSON:", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		w.Header().Set(
			"Access-Control-Allow-Headers",
			"Content-Type, Authorization",
		)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func debug(value any) {
	fmt.Printf("%+v\n", value)
}
