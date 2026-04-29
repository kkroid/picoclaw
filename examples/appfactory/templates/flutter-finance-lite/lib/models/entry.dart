enum EntryType {
  income,
  expense,
}

class BookkeepingEntry {
  const BookkeepingEntry({
    required this.id,
    required this.amount,
    required this.category,
    required this.date,
    required this.note,
    required this.type,
  });

  final String id;
  final double amount;
  final String category;
  final DateTime date;
  final String note;
  final EntryType type;

  bool get isExpense => type == EntryType.expense;

  Map<String, dynamic> toMap() {
    return {
      'id': id,
      'amount': amount,
      'category': category,
      'date': date.toIso8601String(),
      'note': note,
      'type': type.name,
    };
  }

  factory BookkeepingEntry.fromMap(Map<String, dynamic> map) {
    return BookkeepingEntry(
      id: map['id'] as String,
      amount: (map['amount'] as num).toDouble(),
      category: map['category'] as String,
      date: DateTime.parse(map['date'] as String),
      note: map['note'] as String? ?? '',
      type: (map['type'] as String) == EntryType.income.name
          ? EntryType.income
          : EntryType.expense,
    );
  }
}