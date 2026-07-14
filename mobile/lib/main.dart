import 'dart:async';

import 'package:firebase_core/firebase_core.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter/material.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import 'firebase_options.dart';
import 'screens/chat_screen.dart';
import 'screens/login_screen.dart';
import 'services/api_service.dart';
import 'services/notification_service.dart';

const String apiBaseUrl = String.fromEnvironment(
  'API_BASE_URL',
  defaultValue: 'https://REEMPLAZA-TU-BACKEND.onrender.com/api',
);

@pragma('vm:entry-point')
Future<void> firebaseMessagingBackgroundHandler(RemoteMessage message) async {
  await Firebase.initializeApp(options: DefaultFirebaseOptions.currentPlatform);
}

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();

  await Firebase.initializeApp(options: DefaultFirebaseOptions.currentPlatform);

  FirebaseMessaging.onBackgroundMessage(firebaseMessagingBackgroundHandler);

  final api = ApiService(baseUrl: apiBaseUrl);

  final notifications = NotificationService();

  await notifications.initializeInteractions();

  final session = SessionController(api: api, notifications: notifications);

  runApp(PedidosMobileApp(api: api, session: session));

  unawaited(session.restore());
}

enum SessionStatus { loading, loggedOut, loggedIn }

class SessionController extends ChangeNotifier {
  SessionController({required this.api, required this.notifications});

  static const String _tokenKey = 'pedidos_access_token';

  final ApiService api;
  final NotificationService notifications;

  final FlutterSecureStorage _storage = const FlutterSecureStorage();

  SessionStatus _status = SessionStatus.loading;
  String? _token;

  SessionStatus get status => _status;
  String? get token => _token;

  Future<void> restore() async {
    final storedToken = await _storage.read(key: _tokenKey);

    if (storedToken == null || storedToken.isEmpty) {
      _status = SessionStatus.loggedOut;
      notifyListeners();
      return;
    }

    try {
      await api.validateToken(storedToken);

      _token = storedToken;
      _status = SessionStatus.loggedIn;
      notifyListeners();

      unawaited(notifications.enableForLoggedUser());
    } catch (_) {
      await _storage.delete(key: _tokenKey);

      _token = null;
      _status = SessionStatus.loggedOut;
      notifyListeners();
    }
  }

  Future<String?> login({
    required String email,
    required String password,
  }) async {
    try {
      final newToken = await api.login(email: email, password: password);

      await _storage.write(key: _tokenKey, value: newToken);

      _token = newToken;
      _status = SessionStatus.loggedIn;
      notifyListeners();

      unawaited(notifications.enableForLoggedUser());

      return null;
    } on ApiException catch (error) {
      return error.message;
    } catch (_) {
      return 'No se pudo conectar con el servidor.';
    }
  }

  Future<void> logout() async {
    _token = null;
    _status = SessionStatus.loggedOut;
    notifyListeners();

    await _storage.delete(key: _tokenKey);

    await notifications.disableForLoggedUser();
  }
}

class PedidosMobileApp extends StatelessWidget {
  const PedidosMobileApp({required this.api, required this.session, super.key});

  final ApiService api;
  final SessionController session;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Consulta de pedidos',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xFF2857C5)),
        useMaterial3: true,
      ),
      home: AnimatedBuilder(
        animation: session,
        builder: (context, _) {
          switch (session.status) {
            case SessionStatus.loading:
              return const Scaffold(
                body: Center(child: CircularProgressIndicator()),
              );

            case SessionStatus.loggedOut:
              return LoginScreen(session: session);

            case SessionStatus.loggedIn:
              return ChatScreen(session: session, api: api);
          }
        },
      ),
    );
  }
}
