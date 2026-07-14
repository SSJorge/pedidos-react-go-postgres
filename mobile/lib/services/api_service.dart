import 'dart:convert';

import 'package:http/http.dart' as http;

class ApiService {
  ApiService({required String baseUrl})
    : _baseUrl = _removeTrailingSlash(baseUrl);

  final String _baseUrl;

  static String _removeTrailingSlash(String value) {
    final normalized = value.trim();

    if (normalized.endsWith('/')) {
      return normalized.substring(0, normalized.length - 1);
    }

    return normalized;
  }

  Uri _endpoint(String path) {
    return Uri.parse('$_baseUrl/$path');
  }

  Future<String> login({
    required String email,
    required String password,
  }) async {
    final response = await http
        .post(
          _endpoint('auth/login'),
          headers: const {'Content-Type': 'application/json'},
          body: jsonEncode({'email': email.trim(), 'password': password}),
        )
        .timeout(const Duration(seconds: 25));

    final data = _decodeResponse(response);

    if (!_isSuccessful(response.statusCode)) {
      throw ApiException(
        _readError(data, 'No se pudo iniciar sesión.'),
        statusCode: response.statusCode,
      );
    }

    final token = data['token'];

    if (token is! String || token.trim().isEmpty) {
      throw const ApiException('El servidor no entregó un token válido.');
    }

    return token;
  }

  Future<void> validateToken(String token) async {
    final response = await http
        .get(_endpoint('auth/me'), headers: {'Authorization': 'Bearer $token'})
        .timeout(const Duration(seconds: 20));

    final data = _decodeResponse(response);

    if (!_isSuccessful(response.statusCode)) {
      throw ApiException(
        _readError(data, 'La sesión no es válida.'),
        statusCode: response.statusCode,
      );
    }
  }

  Future<Order> getOrderByCode({
    required String token,
    required String publicCode,
  }) async {
    final encodedCode = Uri.encodeComponent(publicCode);

    final response = await http
        .get(
          _endpoint('orders/code/$encodedCode'),
          headers: {'Authorization': 'Bearer $token'},
        )
        .timeout(const Duration(seconds: 25));

    final data = _decodeResponse(response);

    if (!_isSuccessful(response.statusCode)) {
      throw ApiException(
        _readError(data, 'No se pudo consultar el pedido.'),
        statusCode: response.statusCode,
      );
    }

    return Order.fromJson(data);
  }

  static bool _isSuccessful(int statusCode) {
    return statusCode >= 200 && statusCode < 300;
  }

  static Map<String, dynamic> _decodeResponse(http.Response response) {
    if (response.body.trim().isEmpty) {
      return <String, dynamic>{};
    }

    try {
      final decoded = jsonDecode(response.body);

      if (decoded is Map<String, dynamic>) {
        return decoded;
      }

      throw ApiException(
        'El servidor entregó una respuesta inválida.',
        statusCode: response.statusCode,
      );
    } on FormatException {
      throw ApiException(
        'El servidor no respondió en formato JSON.',
        statusCode: response.statusCode,
      );
    }
  }

  static String _readError(Map<String, dynamic> data, String fallback) {
    final message = data['error'];

    if (message is String && message.trim().isNotEmpty) {
      return message;
    }

    return fallback;
  }
}

class ApiException implements Exception {
  const ApiException(this.message, {this.statusCode = 0});

  final String message;
  final int statusCode;

  bool get isUnauthorized => statusCode == 401;

  @override
  String toString() => message;
}

class Order {
  const Order({
    required this.id,
    required this.publicCode,
    required this.customerName,
    required this.customerEmail,
    required this.productName,
    required this.quantity,
    required this.shippingAddress,
    required this.notes,
    required this.status,
  });

  final int id;
  final String publicCode;
  final String customerName;
  final String customerEmail;
  final String productName;
  final int quantity;
  final String shippingAddress;
  final String notes;
  final String status;

  factory Order.fromJson(Map<String, dynamic> json) {
    return Order(
      id: (json['id'] as num?)?.toInt() ?? 0,
      publicCode: json['public_code'] as String? ?? '',
      customerName: json['customer_name'] as String? ?? '',
      customerEmail: json['customer_email'] as String? ?? '',
      productName: json['product_name'] as String? ?? '',
      quantity: (json['quantity'] as num?)?.toInt() ?? 0,
      shippingAddress: json['shipping_address'] as String? ?? '',
      notes: json['notes'] as String? ?? '',
      status: json['status'] as String? ?? '',
    );
  }
}
