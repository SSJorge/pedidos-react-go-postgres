package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
)

const ordersUpdatesTopic = "orders_updates"

type pushNotifier struct {
	client *messaging.Client
}

func newPushNotifier(
	ctx context.Context,
) (*pushNotifier, error) {
	credentialsPath := strings.TrimSpace(
		os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
	)

	if credentialsPath == "" {
		log.Println(
			"FCM deshabilitado: " +
				"GOOGLE_APPLICATION_CREDENTIALS no está configurado",
		)

		return nil, nil
	}

	firebaseApp, err := firebase.NewApp(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf(
			"inicializar Firebase: %w",
			err,
		)
	}

	client, err := firebaseApp.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"inicializar Firebase Messaging: %w",
			err,
		)
	}

	return &pushNotifier{
		client: client,
	}, nil
}

func (app *application) notifyOrderCreated(
	item order,
) {
	if app.push == nil {
		return
	}

	go app.push.send(
		"Nuevo pedido",
		fmt.Sprintf(
			"%s · %s · Cantidad: %d",
			item.PublicCode,
			item.ProductName,
			item.Quantity,
		),
		"order_created",
		item.PublicCode,
	)
}

func (app *application) notifyOrderStatusChanged(
	item order,
) {
	if app.push == nil {
		return
	}

	go app.push.send(
		"Estado de pedido actualizado",
		fmt.Sprintf(
			"%s ahora está %s",
			item.PublicCode,
			pushStatusLabel(item.Status),
		),
		"order_status_changed",
		item.PublicCode,
	)
}

func (notifier *pushNotifier) send(
	title string,
	body string,
	eventType string,
	publicCode string,
) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		12*time.Second,
	)
	defer cancel()

	message := &messaging.Message{
		Topic: ordersUpdatesTopic,

		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},

		Data: map[string]string{
			"type":        eventType,
			"public_code": publicCode,
			"screen":      "chat",
		},

		Android: &messaging.AndroidConfig{
			Priority: "high",
		},
	}

	messageID, err := notifier.client.Send(
		ctx,
		message,
	)
	if err != nil {
		log.Printf(
			"error enviando notificación push: %v",
			err,
		)
		return
	}

	log.Printf(
		"notificación push enviada: %s",
		messageID,
	)
}

func pushStatusLabel(status string) string {
	switch status {
	case "solicitado":
		return "solicitado"

	case "enviado":
		return "enviado"

	case "recibido":
		return "recibido"

	default:
		return status
	}
}
