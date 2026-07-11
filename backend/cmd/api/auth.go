package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type contextKey string

const authenticatedUserKey contextKey = "authenticatedUser"

type user struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type authenticatedUser struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type loginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string            `json:"token"`
	User  authenticatedUser `json:"user"`
}

type jwtClaims struct {
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

func (app *application) login(w http.ResponseWriter, r *http.Request) {
	var input loginInput

	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	input.Email = strings.ToLower(strings.TrimSpace(input.Email))

	if input.Email == "" || input.Password == "" {
		writeError(w, http.StatusBadRequest, "correo y contraseña son obligatorios")
		return
	}

	var account user
	var passwordHash string

	err := app.db.QueryRow(r.Context(), `
		SELECT
			id,
			name,
			email,
			password_hash,
			role,
			created_at
		FROM users
		WHERE LOWER(email) = $1
	`, input.Email).Scan(
		&account.ID,
		&account.Name,
		&account.Email,
		&passwordHash,
		&account.Role,
		&account.CreatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "correo o contraseña incorrectos")
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudo iniciar sesión")
		return
	}

	if err := bcrypt.CompareHashAndPassword(
		[]byte(passwordHash),
		[]byte(input.Password),
	); err != nil {
		writeError(w, http.StatusUnauthorized, "correo o contraseña incorrectos")
		return
	}

	token, err := app.createToken(account)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "no se pudo crear la sesión")
		return
	}

	writeJSON(w, http.StatusOK, loginResponse{
		Token: token,
		User: authenticatedUser{
			ID:    account.ID,
			Name:  account.Name,
			Email: account.Email,
			Role:  account.Role,
		},
	})
}

func (app *application) me(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := getAuthenticatedUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "sesión inválida")
		return
	}

	writeJSON(w, http.StatusOK, currentUser)
}

func (app *application) createToken(account user) (string, error) {
	now := time.Now()

	claims := jwtClaims{
		UserID: account.ID,
		Email:  account.Email,
		Role:   account.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   account.Email,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(8 * time.Hour)),
			Issuer:    "pedidos-api",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString(app.jwtSecret)
}

func (app *application) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authorization := strings.TrimSpace(
			r.Header.Get("Authorization"),
		)

		const bearerPrefix = "Bearer "

		if !strings.HasPrefix(authorization, bearerPrefix) {
			writeError(w, http.StatusUnauthorized, "autenticación requerida")
			return
		}

		tokenString := strings.TrimSpace(
			strings.TrimPrefix(authorization, bearerPrefix),
		)

		if tokenString == "" {
			writeError(w, http.StatusUnauthorized, "token inválido")
			return
		}

		claims := &jwtClaims{}

		token, err := jwt.ParseWithClaims(
			tokenString,
			claims,
			func(token *jwt.Token) (any, error) {
				if token.Method != jwt.SigningMethodHS256 {
					return nil, errors.New("método de firma no permitido")
				}

				return app.jwtSecret, nil
			},
			jwt.WithIssuer("pedidos-api"),
			jwt.WithExpirationRequired(),
		)

		if err != nil || !token.Valid {
			writeError(w, http.StatusUnauthorized, "sesión inválida o expirada")
			return
		}

		var currentUser authenticatedUser

		err = app.db.QueryRow(r.Context(), `
			SELECT id, name, email, role
			FROM users
			WHERE id = $1
		`, claims.UserID).Scan(
			&currentUser.ID,
			&currentUser.Name,
			&currentUser.Email,
			&currentUser.Role,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "usuario no encontrado")
			return
		}

		if err != nil {
			writeError(w, http.StatusInternalServerError, "no se pudo verificar la sesión")
			return
		}

		ctx := context.WithValue(
			r.Context(),
			authenticatedUserKey,
			currentUser,
		)

		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func getAuthenticatedUser(
	ctx context.Context,
) (authenticatedUser, bool) {
	currentUser, ok := ctx.Value(
		authenticatedUserKey,
	).(authenticatedUser)

	return currentUser, ok
}
