import 'package:flutter/material.dart';

import '../providers/connection_provider.dart';

class ConnectionIndicator extends StatelessWidget {
  const ConnectionIndicator({super.key, required this.status, required this.hostPort});

  final ConnectionStatus status;
  final String hostPort;

  @override
  Widget build(BuildContext context) {
    final connected = status == ConnectionStatus.connected;
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(Icons.circle, color: connected ? Colors.green : Colors.red, size: 12),
        const SizedBox(width: 6),
        Text(hostPort, overflow: TextOverflow.ellipsis),
      ],
    );
  }
}
