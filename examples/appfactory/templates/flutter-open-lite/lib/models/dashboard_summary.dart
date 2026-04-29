import 'record.dart';

class DashboardSummary {
  const DashboardSummary({
    required this.totalCount,
    required this.inboxCount,
    required this.inProgressCount,
    required this.doneCount,
  });

  final int totalCount;
  final int inboxCount;
  final int inProgressCount;
  final int doneCount;

  factory DashboardSummary.fromRecords(List<AppRecord> records) {
    var inboxCount = 0;
    var inProgressCount = 0;
    var doneCount = 0;
    for (final record in records) {
      switch (record.status) {
        case RecordStatus.inbox:
          inboxCount += 1;
          break;
        case RecordStatus.inProgress:
          inProgressCount += 1;
          break;
        case RecordStatus.done:
          doneCount += 1;
          break;
      }
    }
    return DashboardSummary(
      totalCount: records.length,
      inboxCount: inboxCount,
      inProgressCount: inProgressCount,
      doneCount: doneCount,
    );
  }
}