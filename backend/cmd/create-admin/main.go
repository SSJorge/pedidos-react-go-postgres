// package createadmin
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	if len(os.Args) != 4 {
		log.Fatal(
			"uso: go run ./cmd/create-admin \"Nombre\" correo@ejemplo.com \"contraseña\"",
		)
	}

	name := strings.TrimSpace(os.Args[1])
	email := strings.ToLower(strings.TrimSpace(os.Args[2]))
	password := os.Args[3]

	if name == "" {
		log.Fatal("el nombre es obligatorio")
	}

	if email == "" {
		log.Fatal("el correo es obligatorio")
	}

	if len(password) < 8 {
		log.Fatal("la contraseña debe tener al menos 8 caracteres")
	}

	databaseURL := envOrDefault(
		"DATABASE_URL",
		"postgres://pedidos_admin:pedidos_clave_2026@127.0.0.1:55432/pedidos?sslmode=disable",
	)

	db, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		log.Fatal(err)
	}

	var id int64

	err = db.QueryRow(context.Background(), `
		INSERT INTO users (
			name,
			email,
			password_hash,
			role
		)
		VALUES ($1, $2, $3, 'admin')
		ON CONFLICT ((LOWER(email)))
		DO UPDATE SET
			name = EXCLUDED.name,
			password_hash = EXCLUDED.password_hash
		RETURNING id
	`, name, email, string(passwordHash)).Scan(&id)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("administrador creado o actualizado con ID %d\n", id)
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
