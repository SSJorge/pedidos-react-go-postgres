package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type whatsappSession struct {
	PublicCode string
}

type whatsappWebhookPayload struct {
	Object string          `json:"object"`
	Entry  []whatsappEntry `json:"entry"`
}

type whatsappEntry struct {
	ID      string           `json:"id"`
	Changes []whatsappChange `json:"changes"`
}

type whatsappChange struct {
	Field string               `json:"field"`
	Value whatsappWebhookValue `json:"value"`
}

type whatsappWebhookValue struct {
	MessagingProduct string                   `json:"messaging_product"`
	Contacts         []whatsappContact        `json:"contacts"`
	Messages         []whatsappInboundMessage `json:"messages"`
}

type whatsappContact struct {
	WaID         string `json:"wa_id"`
	UserID       string `json:"user_id"`
	ParentUserID string `json:"parent_user_id"`
	Username     string `json:"username"`

	Profile struct {
		Name string `json:"name"`
	} `json:"profile"`
}

type whatsappInboundMessage struct {
	From      string `json:"from"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`

	Text *whatsappText `json:"text,omitempty"`
}

type whatsappText struct {
	Body string `json:"body"`
}

type whatsappRecipientAddress struct {
	Value     string
	UsesBSUID bool
}

type whatsappSendMessageRequest struct {
	MessagingProduct string `json:"messaging_product"`
	RecipientType    string `json:"recipient_type"`

	// Para números telefónicos tradicionales.
	To string `json:"to,omitempty"`

	// Para BSUID y nombres de usuario.
	Recipient string `json:"recipient,omitempty"`

	Type string               `json:"type"`
	Text whatsappOutboundText `json:"text"`
}

type whatsappOutboundText struct {
	PreviewURL bool   `json:"preview_url"`
	Body       string `json:"body"`
}

type whatsappAPIResponse struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`

	Error *struct {
		Message      string `json:"message"`
		Type         string `json:"type"`
		Code         int    `json:"code"`
		ErrorSubcode int    `json:"error_subcode"`
	} `json:"error,omitempty"`
}

func (app *application) verifyWhatsAppWebhook(
	w http.ResponseWriter,
	r *http.Request,
) {
	mode := strings.TrimSpace(
		r.URL.Query().Get("hub.mode"),
	)

	token := strings.TrimSpace(
		r.URL.Query().Get("hub.verify_token"),
	)

	challenge := r.URL.Query().Get("hub.challenge")

	if mode != "subscribe" ||
		!secureStringsEqual(token, app.whatsappVerifyToken) {
		http.Error(
			w,
			"Verificación rechazada",
			http.StatusForbidden,
		)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write([]byte(challenge)); err != nil {
		log.Printf(
			"error respondiendo verificación de WhatsApp: %v",
			err,
		)
	}
}

func (app *application) whatsappWebhook(
	w http.ResponseWriter,
	r *http.Request,
) {
	if !app.whatsappConfigured() {
		http.Error(
			w,
			"WhatsApp no está configurado",
			http.StatusServiceUnavailable,
		)
		return
	}

	body, err := io.ReadAll(
		http.MaxBytesReader(w, r.Body, 1<<20),
	)
	if err != nil {
		http.Error(
			w,
			"no se pudo leer el webhook",
			http.StatusBadRequest,
		)
		return
	}

	signature := r.Header.Get("X-Hub-Signature-256")

	if !verifyWhatsAppSignature(
		body,
		signature,
		app.whatsappAppSecret,
	) {
		http.Error(
			w,
			"firma inválida",
			http.StatusUnauthorized,
		)
		return
	}

	var payload whatsappWebhookPayload

	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(
			w,
			"JSON inválido",
			http.StatusBadRequest,
		)
		return
	}

	// Se responde inmediatamente para evitar reintentos de Meta.
	w.WriteHeader(http.StatusOK)

	go app.processWhatsAppWebhook(payload)
}

func (app *application) processWhatsAppWebhook(
	payload whatsappWebhookPayload,
) {
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if change.Field != "messages" {
				continue
			}

			for _, message := range change.Value.Messages {
				recipient := resolveWhatsAppRecipient(
					change.Value,
					message,
				)

				if recipient.Value == "" {
					log.Printf(
						"mensaje de WhatsApp sin destinatario: %s",
						message.ID,
					)
					continue
				}

				ctx, cancel := context.WithTimeout(
					context.Background(),
					15*time.Second,
				)

				if message.Type != "text" ||
					message.Text == nil {
					err := app.sendWhatsAppText(
						ctx,
						recipient,
						"Por ahora solo puedo procesar mensajes de texto.\n\n"+
							"Escribe «inicio» para consultar un pedido.",
					)
					cancel()

					if err != nil {
						log.Printf(
							"error respondiendo mensaje no textual: %v",
							err,
						)
					}

					continue
				}

				text := strings.TrimSpace(
					message.Text.Body,
				)

				err := app.processWhatsAppMessage(
					ctx,
					recipient,
					text,
				)

				cancel()

				if err != nil {
					log.Printf(
						"error procesando WhatsApp %s: %v",
						message.ID,
						err,
					)
				}
			}
		}
	}
}

func (app *application) processWhatsAppMessage(
	ctx context.Context,
	recipient whatsappRecipientAddress,
	text string,
) error {
	command := strings.ToLower(
		strings.TrimSpace(text),
	)

	switch command {
	case "hola",
		"inicio",
		"menu",
		"menú",
		"reiniciar",
		"/start":

		app.clearWhatsAppSession(
			recipient.sessionKey(),
		)

		return app.askWhatsAppOrderCode(
			ctx,
			recipient,
		)
	}

	session, exists := app.getWhatsAppSession(
		recipient.sessionKey(),
	)

	if !exists {
		return app.processWhatsAppOrderCode(
			ctx,
			recipient,
			text,
		)
	}

	return app.processWhatsAppOption(
		ctx,
		recipient,
		session,
		text,
	)
}

func (app *application) askWhatsAppOrderCode(
	ctx context.Context,
	recipient whatsappRecipientAddress,
) error {
	return app.sendWhatsAppText(
		ctx,
		recipient,
		"Ingresa el código de tu pedido.\n\n"+
			"Ejemplo: PED-2026-8F4K2",
	)
}

func (app *application) processWhatsAppOrderCode(
	ctx context.Context,
	recipient whatsappRecipientAddress,
	text string,
) error {
	publicCode := normalizePublicOrderCode(text)

	if !publicOrderCodePattern.MatchString(publicCode) {
		return app.sendWhatsAppText(
			ctx,
			recipient,
			"El código no tiene un formato válido.\n\n"+
				"Debe ser similar a: PED-2026-8F4K2",
		)
	}

	_, err := app.findOrderByPublicCode(
		ctx,
		publicCode,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return app.sendWhatsAppText(
			ctx,
			recipient,
			"No encontré un pedido con ese código.\n\n"+
				"Revisa el código e inténtalo nuevamente.",
		)
	}

	if err != nil {
		return fmt.Errorf(
			"consultar pedido %s: %w",
			publicCode,
			err,
		)
	}

	app.setWhatsAppSession(
		recipient.sessionKey(),
		whatsappSession{
			PublicCode: publicCode,
		},
	)

	return app.sendWhatsAppOptions(
		ctx,
		recipient,
		publicCode,
	)
}

func (app *application) processWhatsAppOption(
	ctx context.Context,
	recipient whatsappRecipientAddress,
	session whatsappSession,
	text string,
) error {
	normalizedText := strings.ToLower(
		strings.TrimSpace(text),
	)

	if normalizedText == "0" ||
		normalizedText == "cambiar pedido" ||
		normalizedText == "otro pedido" {

		app.clearWhatsAppSession(
			recipient.sessionKey(),
		)

		return app.askWhatsAppOrderCode(
			ctx,
			recipient,
		)
	}

	item, err := app.findOrderByPublicCode(
		ctx,
		session.PublicCode,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		app.clearWhatsAppSession(
			recipient.sessionKey(),
		)

		return app.sendWhatsAppText(
			ctx,
			recipient,
			"El pedido ya no está disponible.\n\n"+
				"Ingresa nuevamente el código del pedido.",
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
		return app.sendWhatsAppText(
			ctx,
			recipient,
			"Selecciona una opción válida del 1 al 5.\n\n"+
				"Escribe 0 para consultar otro pedido.",
		)
	}

	answer += "\n\n" +
		"Puedes ingresar otra opción del 1 al 5.\n" +
		"Escribe 0 para consultar otro pedido."

	return app.sendWhatsAppText(
		ctx,
		recipient,
		answer,
	)
}

func (app *application) sendWhatsAppOptions(
	ctx context.Context,
	recipient whatsappRecipientAddress,
	publicCode string,
) error {
	message := "Pedido encontrado: " + publicCode + "\n\n" +
		"¿Qué deseas saber sobre tu pedido?\n\n" +
		"1.- Cliente\n" +
		"2.- Correo\n" +
		"3.- Cantidad\n" +
		"4.- Dirección\n" +
		"5.- Notas\n\n" +
		"Escribe 0 para consultar otro pedido."

	return app.sendWhatsAppText(
		ctx,
		recipient,
		message,
	)
}

func resolveWhatsAppRecipient(
	value whatsappWebhookValue,
	message whatsappInboundMessage,
) whatsappRecipientAddress {
	for _, contact := range value.Contacts {
		sameContact := contact.WaID == message.From ||
			len(value.Contacts) == 1

		if !sameContact {
			continue
		}

		// Nuevo identificador compatible con nombres de usuario.
		if strings.TrimSpace(contact.UserID) != "" {
			return whatsappRecipientAddress{
				Value: strings.TrimSpace(
					contact.UserID,
				),
				UsesBSUID: true,
			}
		}

		// Compatibilidad con webhooks anteriores.
		if strings.TrimSpace(contact.WaID) != "" {
			return whatsappRecipientAddress{
				Value: strings.TrimSpace(
					contact.WaID,
				),
				UsesBSUID: false,
			}
		}
	}

	// Último fallback para payloads antiguos.
	return whatsappRecipientAddress{
		Value:     strings.TrimSpace(message.From),
		UsesBSUID: false,
	}
}

func (recipient whatsappRecipientAddress) sessionKey() string {
	if recipient.UsesBSUID {
		return "bsuid:" + recipient.Value
	}

	return "phone:" + recipient.Value
}

func (app *application) sendWhatsAppText(
	ctx context.Context,
	recipient whatsappRecipientAddress,
	text string,
) error {
	requestBody := whatsappSendMessageRequest{
		MessagingProduct: "whatsapp",
		RecipientType:    "individual",
		Type:             "text",
		Text: whatsappOutboundText{
			PreviewURL: false,
			Body:       text,
		},
	}

	if recipient.UsesBSUID {
		requestBody.Recipient = recipient.Value
	} else {
		requestBody.To = recipient.Value
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf(
			"codificar mensaje de WhatsApp: %w",
			err,
		)
	}

	graphVersion := strings.Trim(
		app.whatsappGraphAPIVersion,
		"/ ",
	)

	url := fmt.Sprintf(
		"https://graph.facebook.com/%s/%s/messages",
		graphVersion,
		app.whatsappPhoneNumberID,
	)

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		url,
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf(
			"crear petición de WhatsApp: %w",
			err,
		)
	}

	request.Header.Set(
		"Authorization",
		"Bearer "+app.whatsappAccessToken,
	)

	request.Header.Set(
		"Content-Type",
		"application/json",
	)

	response, err := app.whatsappClient.Do(request)
	if err != nil {
		return fmt.Errorf(
			"enviar mensaje de WhatsApp: %w",
			err,
		)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(
		io.LimitReader(response.Body, 1<<20),
	)
	if err != nil {
		return fmt.Errorf(
			"leer respuesta de WhatsApp: %w",
			err,
		)
	}

	var apiResponse whatsappAPIResponse

	if len(responseBody) > 0 {
		if err := json.Unmarshal(
			responseBody,
			&apiResponse,
		); err != nil {
			return fmt.Errorf(
				"interpretar respuesta de WhatsApp: %w",
				err,
			)
		}
	}

	if response.StatusCode < 200 ||
		response.StatusCode >= 300 {

		if apiResponse.Error != nil {
			return fmt.Errorf(
				"WhatsApp respondió %d, código %d: %s",
				response.StatusCode,
				apiResponse.Error.Code,
				apiResponse.Error.Message,
			)
		}

		return fmt.Errorf(
			"WhatsApp respondió HTTP %d",
			response.StatusCode,
		)
	}

	return nil
}

func verifyWhatsAppSignature(
	body []byte,
	signatureHeader string,
	appSecret string,
) bool {
	const prefix = "sha256="

	if appSecret == "" ||
		!strings.HasPrefix(signatureHeader, prefix) {
		return false
	}

	providedSignature, err := hex.DecodeString(
		strings.TrimPrefix(
			signatureHeader,
			prefix,
		),
	)
	if err != nil {
		return false
	}

	mac := hmac.New(
		sha256.New,
		[]byte(appSecret),
	)

	if _, err := mac.Write(body); err != nil {
		return false
	}

	expectedSignature := mac.Sum(nil)

	return hmac.Equal(
		providedSignature,
		expectedSignature,
	)
}

func (app *application) whatsappConfigured() bool {
	return app.whatsappAccessToken != "" &&
		app.whatsappPhoneNumberID != "" &&
		app.whatsappVerifyToken != "" &&
		app.whatsappAppSecret != "" &&
		app.whatsappGraphAPIVersion != ""
}

func (app *application) getWhatsAppSession(
	key string,
) (whatsappSession, bool) {
	app.whatsappSessionsMu.RLock()
	defer app.whatsappSessionsMu.RUnlock()

	session, exists := app.whatsappSessions[key]

	return session, exists
}

func (app *application) setWhatsAppSession(
	key string,
	session whatsappSession,
) {
	app.whatsappSessionsMu.Lock()
	defer app.whatsappSessionsMu.Unlock()

	app.whatsappSessions[key] = session
}

func (app *application) clearWhatsAppSession(
	key string,
) {
	app.whatsappSessionsMu.Lock()
	defer app.whatsappSessionsMu.Unlock()

	delete(app.whatsappSessions, key)
}
