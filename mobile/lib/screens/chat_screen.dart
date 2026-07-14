import 'package:flutter/material.dart';

import '../main.dart';
import '../services/api_service.dart';

enum MessageAuthor { user, bot }

class ChatMessage {
  const ChatMessage({required this.text, required this.author});

  final String text;
  final MessageAuthor author;
}

class ChatScreen extends StatefulWidget {
  const ChatScreen({required this.session, required this.api, super.key});

  final SessionController session;
  final ApiService api;

  @override
  State<ChatScreen> createState() => _ChatScreenState();
}

class _ChatScreenState extends State<ChatScreen> {
  static final RegExp _orderCodePattern = RegExp(
    r'^PED-[0-9]{4}-[ABCDEFGHJKLMNPQRSTUVWXYZ23456789]{5}$',
  );

  static const String _initialMessage =
      'Ingresa el código de tu pedido.\n\n'
      'Ejemplo: PED-2026-8F4K2';

  final TextEditingController _inputController = TextEditingController();

  final ScrollController _scrollController = ScrollController();

  final List<ChatMessage> _messages = [
    const ChatMessage(text: _initialMessage, author: MessageAuthor.bot),
  ];

  String? _publicCode;
  Order? _currentOrder;
  bool _loading = false;

  @override
  void dispose() {
    _inputController.dispose();
    _scrollController.dispose();
    super.dispose();
  }

  String _normalizeCode(String value) {
    return value.trim().toUpperCase().replaceAll(RegExp(r'\s+'), '');
  }

  Future<void> _sendMessage() async {
    final input = _inputController.text.trim();

    if (input.isEmpty || _loading) {
      return;
    }

    _inputController.clear();

    _appendMessage(ChatMessage(text: input, author: MessageAuthor.user));

    if (_publicCode == null) {
      await _processOrderCode(input);
      return;
    }

    await _processOption(input);
  }

  Future<void> _processOrderCode(String input) async {
    final publicCode = _normalizeCode(input);

    if (!_orderCodePattern.hasMatch(publicCode)) {
      _appendBotMessage(
        'El código no tiene un formato válido.\n\n'
        'Debe ser similar a: PED-2026-8F4K2',
      );
      return;
    }

    _setLoading(true);

    try {
      final order = await widget.api.getOrderByCode(
        token: widget.session.token!,
        publicCode: publicCode,
      );

      if (!mounted) {
        return;
      }

      setState(() {
        _publicCode = order.publicCode;
        _currentOrder = order;
      });

      _appendBotMessage(_optionsMessage(order.publicCode));
    } on ApiException catch (error) {
      await _handleApiError(error);
    } catch (_) {
      _appendBotMessage(
        'No se pudo conectar con el servidor. '
        'Revisa tu conexión e inténtalo nuevamente.',
      );
    } finally {
      _setLoading(false);
    }
  }

  Future<void> _processOption(String input) async {
    final normalized = input.trim().toLowerCase();

    if (normalized == '0' ||
        normalized == 'cambiar pedido' ||
        normalized == 'otro pedido') {
      _restartChat();
      return;
    }

    final option = int.tryParse(normalized);

    if (option == null || option < 1 || option > 6) {
      _appendBotMessage(
        'Selecciona una opción válida del 1 al 6.\n\n'
        'Escribe 0 para consultar otro pedido.',
      );
      return;
    }

    final publicCode = _publicCode;

    if (publicCode == null) {
      _restartChat();
      return;
    }

    _setLoading(true);

    try {
      // Se vuelve a consultar para que el estado y los demás
      // datos estén actualizados.
      final order = await widget.api.getOrderByCode(
        token: widget.session.token!,
        publicCode: publicCode,
      );

      if (!mounted) {
        return;
      }

      setState(() {
        _currentOrder = order;
      });

      final answer = _answerForOption(order, option);

      _appendBotMessage(
        '$answer\n\n'
        'Puedes ingresar otra opción del 1 al 6.\n'
        'Escribe 0 para consultar otro pedido.',
      );
    } on ApiException catch (error) {
      await _handleApiError(error);
    } catch (_) {
      _appendBotMessage(
        'No se pudo conectar con el servidor. '
        'Inténtalo nuevamente.',
      );
    } finally {
      _setLoading(false);
    }
  }

  String _answerForOption(Order order, int option) {
    switch (option) {
      case 1:
        return 'Cliente: ${order.customerName}';

      case 2:
        return 'Correo: ${order.customerEmail}';

      case 3:
        return 'Cantidad: ${order.quantity}';

      case 4:
        return 'Dirección: ${order.shippingAddress}';

      case 5:
        final notes = order.notes.trim();

        return notes.isEmpty ? 'Notas: Sin notas' : 'Notas: $notes';

      case 6:
        return 'Estado: ${_statusLabel(order.status)}';

      default:
        return 'Opción inválida.';
    }
  }

  String _statusLabel(String status) {
    switch (status.toLowerCase()) {
      case 'solicitado':
        return 'Solicitado';

      case 'enviado':
        return 'Enviado';

      case 'recibido':
        return 'Recibido';

      default:
        return status;
    }
  }

  String _optionsMessage(String publicCode) {
    return 'Pedido encontrado: $publicCode\n\n'
        '¿Qué deseas saber sobre tu pedido?\n\n'
        '1.- Cliente\n'
        '2.- Correo\n'
        '3.- Cantidad\n'
        '4.- Dirección\n'
        '5.- Notas\n'
        '6.- Estado\n\n'
        'Escribe 0 para consultar otro pedido.';
  }

  Future<void> _handleApiError(ApiException error) async {
    if (error.isUnauthorized) {
      await widget.session.logout();
      return;
    }

    _appendBotMessage(error.message);
  }

  void _restartChat() {
    setState(() {
      _publicCode = null;
      _currentOrder = null;

      _messages
        ..clear()
        ..add(
          const ChatMessage(text: _initialMessage, author: MessageAuthor.bot),
        );
    });

    _scrollToBottom();
  }

  void _appendBotMessage(String text) {
    _appendMessage(ChatMessage(text: text, author: MessageAuthor.bot));
  }

  void _appendMessage(ChatMessage message) {
    if (!mounted) {
      return;
    }

    setState(() {
      _messages.add(message);
    });

    _scrollToBottom();
  }

  void _setLoading(bool value) {
    if (!mounted) {
      return;
    }

    setState(() {
      _loading = value;
    });
  }

  void _scrollToBottom() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!_scrollController.hasClients) {
        return;
      }

      _scrollController.animateTo(
        _scrollController.position.maxScrollExtent,
        duration: const Duration(milliseconds: 260),
        curve: Curves.easeOut,
      );
    });
  }

  @override
  Widget build(BuildContext context) {
    final waitingForCode = _publicCode == null;

    return Scaffold(
      backgroundColor: const Color(0xFFF1F3F7),
      appBar: AppBar(
        title: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text('Consulta de pedidos'),
            Text(
              _currentOrder == null
                  ? 'Asistente automático'
                  : _currentOrder!.publicCode,
              style: const TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.normal,
              ),
            ),
          ],
        ),
        actions: [
          IconButton(
            tooltip: 'Consultar otro pedido',
            onPressed: _restartChat,
            icon: const Icon(Icons.refresh),
          ),
          IconButton(
            tooltip: 'Cerrar sesión',
            onPressed: widget.session.logout,
            icon: const Icon(Icons.logout),
          ),
        ],
      ),
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: ListView.builder(
                controller: _scrollController,
                padding: const EdgeInsets.fromLTRB(14, 18, 14, 10),
                itemCount: _messages.length,
                itemBuilder: (context, index) {
                  return _MessageBubble(message: _messages[index]);
                },
              ),
            ),
            if (_loading) const LinearProgressIndicator(minHeight: 2),
            Material(
              elevation: 8,
              color: Colors.white,
              child: Padding(
                padding: const EdgeInsets.fromLTRB(12, 10, 12, 12),
                child: Row(
                  children: [
                    Expanded(
                      child: TextField(
                        controller: _inputController,
                        enabled: !_loading,
                        textInputAction: TextInputAction.send,
                        textCapitalization: waitingForCode
                            ? TextCapitalization.characters
                            : TextCapitalization.none,
                        decoration: InputDecoration(
                          hintText: waitingForCode
                              ? 'Código del pedido'
                              : 'Escribe una opción del 1 al 6',
                          filled: true,
                          fillColor: const Color(0xFFF1F3F7),
                          border: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(24),
                            borderSide: BorderSide.none,
                          ),
                          contentPadding: const EdgeInsets.symmetric(
                            horizontal: 18,
                            vertical: 12,
                          ),
                        ),
                        onSubmitted: (_) => _sendMessage(),
                      ),
                    ),
                    const SizedBox(width: 8),
                    IconButton.filled(
                      tooltip: 'Enviar',
                      onPressed: _loading ? null : _sendMessage,
                      icon: const Icon(Icons.send),
                    ),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _MessageBubble extends StatelessWidget {
  const _MessageBubble({required this.message});

  final ChatMessage message;

  @override
  Widget build(BuildContext context) {
    final isUser = message.author == MessageAuthor.user;

    final colorScheme = Theme.of(context).colorScheme;

    return Align(
      alignment: isUser ? Alignment.centerRight : Alignment.centerLeft,
      child: Container(
        constraints: BoxConstraints(
          maxWidth: MediaQuery.sizeOf(context).width * 0.80,
        ),
        margin: const EdgeInsets.only(bottom: 12),
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        decoration: BoxDecoration(
          color: isUser ? colorScheme.primary : Colors.white,
          borderRadius: BorderRadius.only(
            topLeft: const Radius.circular(18),
            topRight: const Radius.circular(18),
            bottomLeft: Radius.circular(isUser ? 18 : 4),
            bottomRight: Radius.circular(isUser ? 4 : 18),
          ),
          boxShadow: const [
            BoxShadow(
              color: Colors.black12,
              blurRadius: 5,
              offset: Offset(0, 2),
            ),
          ],
        ),
        child: Text(
          message.text,
          style: TextStyle(
            height: 1.35,
            color: isUser ? colorScheme.onPrimary : const Color(0xFF1F2937),
          ),
        ),
      ),
    );
  }
}
