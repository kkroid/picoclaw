import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../providers/connection_provider.dart';

class ConnectionPage extends StatefulWidget {
  const ConnectionPage({super.key});

  @override
  State<ConnectionPage> createState() => _ConnectionPageState();
}

class _ConnectionPageState extends State<ConnectionPage> {
  final hostController = TextEditingController();
  final portController = TextEditingController(text: '8080');
  final tokenController = TextEditingController();

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final settings = context.read<ConnectionProvider>().settings;
    if (hostController.text.isEmpty) {
      hostController.text = settings.host;
      portController.text = settings.port.toString();
      tokenController.text = settings.token;
    }
  }

  @override
  void dispose() {
    hostController.dispose();
    portController.dispose();
    tokenController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('连接配置')),
      body: Consumer<ConnectionProvider>(
        builder: (context, connection, _) {
          return ListView(
            padding: const EdgeInsets.all(16),
            children: [
              TextField(key: const Key('host-field'), controller: hostController, decoration: const InputDecoration(labelText: 'PC host')),
              const SizedBox(height: 12),
              TextField(key: const Key('port-field'), controller: portController, keyboardType: TextInputType.number, decoration: const InputDecoration(labelText: '端口')),
              const SizedBox(height: 12),
              TextField(key: const Key('token-field'), controller: tokenController, decoration: const InputDecoration(labelText: '可选 token')),
              const SizedBox(height: 20),
              FilledButton.icon(
                key: const Key('connect-button'),
                onPressed: connection.status == ConnectionStatus.connecting
                    ? null
                    : () => connection.connect(host: hostController.text, port: int.tryParse(portController.text) ?? 8080, token: tokenController.text),
                icon: const Icon(Icons.lan),
                label: const Text('连接'),
              ),
              if (connection.status == ConnectionStatus.connecting) const LinearProgressIndicator(),
              if (connection.errorMessage != null) Padding(padding: const EdgeInsets.only(top: 12), child: Text(connection.errorMessage!, style: TextStyle(color: Theme.of(context).colorScheme.error))),
            ],
          );
        },
      ),
    );
  }
}
