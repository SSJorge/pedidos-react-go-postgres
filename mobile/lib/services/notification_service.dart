import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter/foundation.dart';

class NotificationService {
  static const String ordersTopic = 'orders_updates';

  final FirebaseMessaging _messaging = FirebaseMessaging.instance;

  Future<void> initializeInteractions() async {
    // Se ejecuta cuando una notificación abrió la aplicación
    // desde un estado completamente cerrado.
    await _messaging.getInitialMessage();

    // Se ejecuta cuando una notificación abrió la aplicación
    // desde segundo plano.
    FirebaseMessaging.onMessageOpenedApp.listen((message) {
      debugPrint('Notificación abierta: ${message.data}');

      // No hacemos navegación porque la única pantalla
      // posterior al login es el chat.
    });
  }

  Future<void> enableForLoggedUser() async {
    try {
      final settings = await _messaging.requestPermission(
        alert: true,
        badge: true,
        sound: true,
        announcement: false,
        carPlay: false,
        criticalAlert: false,
        provisional: false,
      );

      if (settings.authorizationStatus == AuthorizationStatus.denied) {
        return;
      }

      await _messaging.subscribeToTopic(ordersTopic);

      debugPrint('Suscrito al tema $ordersTopic');
    } catch (error) {
      debugPrint('No se pudieron activar las notificaciones: $error');
    }
  }

  Future<void> disableForLoggedUser() async {
    try {
      await _messaging.unsubscribeFromTopic(ordersTopic);
    } catch (error) {
      debugPrint('No se pudo cancelar la suscripción: $error');
    }
  }
}
