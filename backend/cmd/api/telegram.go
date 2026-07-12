package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

var publicOrderCodePattern = regexp.MustCompile(
	`^PED-[0-9]{4}-[ABCDEFGHJKLMNPQRSTUVWXYZ23456789]{5}$`,
)

type telegramSession struct {
	PublicCode string
}

type telegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *telegramMessage `json:"message"`
}

type telegramMessage struct {
	MessageID int64        `json:"message_id"`
	Chat      telegramChat `json:"chat"`
	Text      string       `json:"text"`
}

type telegramChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type telegramSendMessageRequest struct {
	ChatID      int64  `json:"chat_id"`
	Text        string `json:"text"`
	ReplyMarkup any    `json:"reply_markup,omitempty"`
}

type telegramAPIResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

type telegramReplyKeyboard struct {
	Keyboard        [][]telegramKeyboardButton `json:"keyboard"`
	ResizeKeyboard  bool                       `json:"resize_keyboard"`
	OneTimeKeyboard bool                       `json:"one_time_keyboard"`
}

type telegramKeyboardButton struct {
	Text string `json:"text"`
}

type telegramRemoveKeyboard struct {
	RemoveKeyboard bool `json:"remove_keyboard"`
}

func (app *application) telegramWebhook(
	w http.ResponseWriter,
	r *http.Request,
) {
	if app.telegramToken == "" ||
		app.telegramWebhookSecret == "" {
		http.Error(
			w,
			"Telegram no está configurado",
			http.StatusServiceUnavailable,
		)
		return
	}

	receivedSecret := r.Header.Get(
		"X-Telegram-Bot-Api-Secret-Token",
	)

	if !secureStringsEqual(
		receivedSecret,
		app.telegramWebhookSecret,
	) {
		http.Error(w, "No autorizado", http.StatusUnauthorized)
		return
	}

	var update telegramUpdate

	decoder := json.NewDecoder(
		http.MaxBytesReader(w, r.Body, 1<<20),
	)

	if err := decoder.Decode(&update); err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}

	// Telegram también puede enviar otros tipos de actualizaciones.
	if update.Message == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	if update.Message.Chat.Type != "private" {
		_ = app.sendTelegramMessage(
			r.Context(),
			update.Message.Chat.ID,
			"Este bot solo funciona mediante conversaciones privadas.",
			nil,
		)

		w.WriteHeader(http.StatusOK)
		return
	}

	text := strings.TrimSpace(update.Message.Text)

	if text == "" {
		_ = app.sendTelegramMessage(
			r.Context(),
			update.Message.Chat.ID,
			"Envía el código de tu pedido como texto.",
			nil,
		)

		w.WriteHeader(http.StatusOK)
		return
	}

	if err := app.processTelegramMessage(
		r.Context(),
		update.Message.Chat.ID,
		text,
	); err != nil {
		log.Printf(
			"error procesando mensaje de Telegram: %v",
			err,
		)

		_ = app.sendTelegramMessage(
			r.Context(),
			update.Message.Chat.ID,
			"No se pudo procesar la consulta. Intenta nuevamente.",
			nil,
		)
	}

	w.WriteHeader(http.StatusOK)
}

func (app *application) processTelegramMessage(
	ctx context.Context,
	chatID int64,
	text string,
) error {
	command := strings.ToLower(strings.TrimSpace(text))

	switch command {
	case "/start", "/consultar", "/reiniciar":
		app.clearTelegramSession(chatID)

		return app.askForOrderCode(ctx, chatID)
	}

	session, hasSession := app.getTelegramSession(chatID)

	if !hasSession {
		return app.processTelegramOrderCode(
			ctx,
			chatID,
			text,
		)
	}

	return app.processTelegramOption(
		ctx,
		chatID,
		session,
		text,
	)
}

func (app *application) askForOrderCode(
	ctx context.Context,
	chatID int64,
) error {
	return app.sendTelegramMessage(
		ctx,
		chatID,
		"Ingresa el código de tu pedido.\n\n"+
			"Ejemplo: PED-2026-8F4K2",
		telegramRemoveKeyboard{
			RemoveKeyboard: true,
		},
	)
}

func (app *application) processTelegramOrderCode(
	ctx context.Context,
	chatID int64,
	text string,
) error {
	publicCode := normalizePublicOrderCode(text)

	if !publicOrderCodePattern.MatchString(publicCode) {
		return app.sendTelegramMessage(
			ctx,
			chatID,
			"El código no tiene un formato válido.\n\n"+
				"Debe ser similar a: PED-2026-8F4K2",
			telegramRemoveKeyboard{
				RemoveKeyboard: true,
			},
		)
	}

	_, err := app.findOrderByPublicCode(ctx, publicCode)

	if errors.Is(err, pgx.ErrNoRows) {
		return app.sendTelegramMessage(
			ctx,
			chatID,
			"No encontré un pedido con ese código.\n\n"+
				"Revisa el código e inténtalo nuevamente.",
			telegramRemoveKeyboard{
				RemoveKeyboard: true,
			},
		)
	}

	if err != nil {
		return fmt.Errorf(
			"consultar pedido %s: %w",
			publicCode,
			err,
		)
	}

	app.setTelegramSession(
		chatID,
		telegramSession{
			PublicCode: publicCode,
		},
	)

	return app.sendTelegramOptions(
		ctx,
		chatID,
		publicCode,
	)
}

func (app *application) processTelegramOption(
	ctx context.Context,
	chatID int64,
	session telegramSession,
	text string,
) error {
	if strings.EqualFold(text, "Cambiar pedido") ||
		text == "0" {
		app.clearTelegramSession(chatID)

		return app.askForOrderCode(ctx, chatID)
	}

	item, err := app.findOrderByPublicCode(
		ctx,
		session.PublicCode,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		app.clearTelegramSession(chatID)

		return app.sendTelegramMessage(
			ctx,
			chatID,
			"El pedido ya no está disponible. "+
				"Ingresa nuevamente un código.",
			telegramRemoveKeyboard{
				RemoveKeyboard: true,
			},
		)
	}

	if err != nil {
		return fmt.Errorf(
			"consultar pedido guardado %s: %w",
			session.PublicCode,
			err,
		)
	}

	var answer string

	switch strings.TrimSpace(text) {
	case "1":
		answer = "Cliente: " + item.CustomerName

	case "2":
		answer = "Correo: " + item.CustomerEmail

	case "3":
		answer = fmt.Sprintf(
			"Cantidad: %d",
			item.Quantity,
		)

	case "4":
		answer = "Dirección: " + item.ShippingAddress

	case "5":
		notes := strings.TrimSpace(item.Notes)

		if notes == "" {
			notes = "Sin notas"
		}

		answer = "Notas: " + notes

	default:
		return app.sendTelegramMessage(
			ctx,
			chatID,
			"Selecciona una opción válida del 1 al 5.",
			telegramOptionsKeyboard(),
		)
	}

	answer += "\n\nSelecciona otra opción o pulsa «Cambiar pedido»."

	return app.sendTelegramMessage(
		ctx,
		chatID,
		answer,
		telegramOptionsKeyboard(),
	)
}

func (app *application) sendTelegramOptions(
	ctx context.Context,
	chatID int64,
	publicCode string,
) error {
	message := "Pedido encontrado: " + publicCode + "\n\n" +
		"¿Qué deseas saber sobre tu pedido?\n\n" +
		"1.- Cliente\n" +
		"2.- Correo\n" +
		"3.- Cantidad\n" +
		"4.- Dirección\n" +
		"5.- Notas"

	return app.sendTelegramMessage(
		ctx,
		chatID,
		message,
		telegramOptionsKeyboard(),
	)
}

func telegramOptionsKeyboard() telegramReplyKeyboard {
	return telegramReplyKeyboard{
		Keyboard: [][]telegramKeyboardButton{
			{
				{Text: "1"},
				{Text: "2"},
				{Text: "3"},
			},
			{
				{Text: "4"},
				{Text: "5"},
			},
			{
				{Text: "Cambiar pedido"},
			},
		},
		ResizeKeyboard:  true,
		OneTimeKeyboard: false,
	}
}

func (app *application) findOrderByPublicCode(
	ctx context.Context,
	publicCode string,
) (order, error) {
	var item order

	err := app.db.QueryRow(
		ctx,
		`
			SELECT
				id,
				public_code,
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
			WHERE public_code = $1
		`,
		publicCode,
	).Scan(
		&item.ID,
		&item.PublicCode,
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

	return item, err
}

func (app *application) sendTelegramMessage(
	ctx context.Context,
	chatID int64,
	text string,
	replyMarkup any,
) error {
	requestBody := telegramSendMessageRequest{
		ChatID:      chatID,
		Text:        text,
		ReplyMarkup: replyMarkup,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf(
			"codificar mensaje de Telegram: %w",
			err,
		)
	}

	url := fmt.Sprintf(
		"https://api.telegram.org/bot%s/sendMessage",
		app.telegramToken,
	)

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		url,
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf(
			"crear petición a Telegram: %w",
			err,
		)
	}

	request.Header.Set(
		"Content-Type",
		"application/json",
	)

	response, err := app.telegramClient.Do(request)
	if err != nil {
		return fmt.Errorf(
			"enviar mensaje a Telegram: %w",
			err,
		)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(
		io.LimitReader(response.Body, 1<<20),
	)
	if err != nil {
		return fmt.Errorf(
			"leer respuesta de Telegram: %w",
			err,
		)
	}

	var telegramResponse telegramAPIResponse

	if err := json.Unmarshal(
		responseBody,
		&telegramResponse,
	); err != nil {
		return fmt.Errorf(
			"interpretar respuesta de Telegram: %w",
			err,
		)
	}

	if response.StatusCode < 200 ||
		response.StatusCode >= 300 ||
		!telegramResponse.OK {
		return fmt.Errorf(
			"Telegram respondió %d: %s",
			response.StatusCode,
			telegramResponse.Description,
		)
	}

	return nil
}

func (app *application) getTelegramSession(
	chatID int64,
) (telegramSession, bool) {
	app.telegramSessionsMu.RLock()
	defer app.telegramSessionsMu.RUnlock()

	session, exists := app.telegramSessions[chatID]

	return session, exists
}

func (app *application) setTelegramSession(
	chatID int64,
	session telegramSession,
) {
	app.telegramSessionsMu.Lock()
	defer app.telegramSessionsMu.Unlock()

	app.telegramSessions[chatID] = session
}

func (app *application) clearTelegramSession(chatID int64) {
	app.telegramSessionsMu.Lock()
	defer app.telegramSessionsMu.Unlock()

	delete(app.telegramSessions, chatID)
}

func secureStringsEqual(first, second string) bool {
	if first == "" || second == "" {
		return false
	}

	return subtle.ConstantTimeCompare(
		[]byte(first),
		[]byte(second),
	) == 1
}
