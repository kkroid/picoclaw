import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:bookkeeping_lite/main.dart';
import 'package:bookkeeping_lite/repositories/entry_repository.dart';

void main() {
  testWidgets('bookkeeping flow saves an entry', (WidgetTester tester) async {
    await tester.pumpWidget(
      BookkeepingApp(repository: InMemoryEntryRepository()),
    );
    await tester.pumpAndSettle();

    expect(find.text('本月结余'), findsOneWidget);
    expect(find.text('最近账单'), findsOneWidget);

    await tester.tap(find.text('记一笔'));
    await tester.pumpAndSettle();

    await tester.enterText(find.byKey(const Key('amount-field')), '48.5');
    await tester.enterText(find.byKey(const Key('note-field')), '早餐');
    await tester.tap(find.text('保存'));
    await tester.pumpAndSettle();

    expect(find.text('早餐'), findsOneWidget);
    expect(find.text('48.50'), findsOneWidget);
  });
}
