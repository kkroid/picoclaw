import 'entry.dart';

class Summary {
  const Summary({
    required this.income,
    required this.expense,
    required this.balance,
    required this.entryCount,
  });

  final double income;
  final double expense;
  final double balance;
  final int entryCount;

  factory Summary.fromEntries(List<BookkeepingEntry> entries) {
    var income = 0.0;
    var expense = 0.0;
    for (final entry in entries) {
      if (entry.type == EntryType.income) {
        income += entry.amount;
      } else {
        expense += entry.amount;
      }
    }
    return Summary(
      income: income,
      expense: expense,
      balance: income - expense,
      entryCount: entries.length,
    );
  }
}