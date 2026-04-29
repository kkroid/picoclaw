import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:flutter_open_lite/main.dart';
import 'package:flutter_open_lite/repositories/record_repository.dart';

void main() {
  testWidgets('open lite flow supports create, filter, read, update and delete', (WidgetTester tester) async {
    await tester.pumpWidget(
      FlutterOpenLiteApp(repository: InMemoryRecordRepository()),
    );
    await tester.pumpAndSettle();

    expect(find.text('今日摘要'), findsOneWidget);
    expect(find.text('最近记录'), findsOneWidget);

    await tester.tap(find.text('新建记录'));
    await tester.pumpAndSettle();

    await tester.enterText(find.byKey(const Key('title-field')), '补充维度说明');
    await tester.enterText(find.byKey(const Key('note-field')), '把待办、打卡和库存都映射到同一记录模型');
    await tester.tap(find.text('保存'));
    await tester.pumpAndSettle();

    expect(find.text('补充维度说明'), findsOneWidget);
    expect(find.textContaining('例行事项'), findsOneWidget);

    await tester.tap(find.text('补充维度说明').first);
    await tester.pumpAndSettle();

    expect(find.text('记录详情'), findsOneWidget);
    expect(find.text('把待办、打卡和库存都映射到同一记录模型'), findsOneWidget);

    await tester.tap(find.text('编辑'));
    await tester.pumpAndSettle();

    expect(find.text('编辑记录'), findsOneWidget);
    await tester.enterText(find.byKey(const Key('title-field')), '补充实体映射规则');
    await tester.enterText(find.byKey(const Key('note-field')), '补上编辑闭环，方便后续 PRD 二开');
    await tester.tap(find.text('已完成'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('更新'));
    await tester.pumpAndSettle();

    expect(find.text('补充实体映射规则'), findsOneWidget);
    expect(find.text('补上编辑闭环，方便后续 PRD 二开'), findsOneWidget);
    expect(find.text('已完成'), findsWidgets);

    await tester.pageBack();
    await tester.pumpAndSettle();

    await tester.tap(find.text('查看全部'));
    await tester.pumpAndSettle();

    expect(find.text('状态筛选'), findsOneWidget);
    await tester.tap(find.byKey(const Key('record-filter-done')));
    await tester.pumpAndSettle();

    expect(find.text('当前展示 1 / 1 条记录'), findsOneWidget);
    expect(find.text('补充实体映射规则'), findsOneWidget);

    await tester.tap(find.text('补充实体映射规则'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('删除'));
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(FilledButton, '删除'));
    await tester.pumpAndSettle();

    expect(find.text('补充实体映射规则'), findsNothing);
    expect(find.text('暂无记录，请先从首页新增一条记录。'), findsOneWidget);
  });
}
