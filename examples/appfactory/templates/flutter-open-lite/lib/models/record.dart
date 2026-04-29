enum RecordStatus {
  inbox,
  inProgress,
  done,
}

class AppRecord {
  const AppRecord({
    required this.id,
    required this.title,
    required this.category,
    required this.updatedAt,
    required this.note,
    required this.status,
  });

  final String id;
  final String title;
  final String category;
  final DateTime updatedAt;
  final String note;
  final RecordStatus status;

  bool get isDone => status == RecordStatus.done;

  AppRecord copyWith({
    String? title,
    String? category,
    DateTime? updatedAt,
    String? note,
    RecordStatus? status,
  }) {
    return AppRecord(
      id: id,
      title: title ?? this.title,
      category: category ?? this.category,
      updatedAt: updatedAt ?? this.updatedAt,
      note: note ?? this.note,
      status: status ?? this.status,
    );
  }

  Map<String, dynamic> toMap() {
    return {
      'id': id,
      'title': title,
      'category': category,
      'updated_at': updatedAt.toIso8601String(),
      'note': note,
      'status': status.name,
    };
  }

  factory AppRecord.fromMap(Map<String, dynamic> map) {
    return AppRecord(
      id: map['id'] as String,
      title: map['title'] as String,
      category: map['category'] as String,
      updatedAt: DateTime.parse(map['updated_at'] as String),
      note: map['note'] as String? ?? '',
      status: switch (map['status'] as String? ?? '') {
        'done' => RecordStatus.done,
        'inProgress' => RecordStatus.inProgress,
        _ => RecordStatus.inbox,
      },
    );
  }
}