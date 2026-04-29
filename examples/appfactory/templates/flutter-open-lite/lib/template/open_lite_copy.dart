import '../models/record.dart';

const openLiteCopy = OpenLiteCopy();

class OpenLiteCopy {
  const OpenLiteCopy();

  String get appTitle => 'Open Lite Seed';

  String get homeSummaryTitle => '今日摘要';

  String summaryCountLabel(int count) => '$count 条记录';

  String get createPrimaryActionLabel => '新建记录';

  String get viewAllActionLabel => '查看全部';

  String get recentRecordsTitle => '最近记录';

  String recentRecordsCountLabel(int count) => '$count 条';

  String get homeEmptyTitle => '还没有记录';

  String get homeEmptyDescription => '先创建一条记录，首页摘要和列表会自动刷新。';

  String get listPageTitle => '全部记录';

  String get listFilterTitle => '状态筛选';

  String listCountLabel(int visibleCount, int totalCount) =>
      '当前展示 $visibleCount / $totalCount 条记录';

  String get filteredEmptyTitle => '当前筛选下还没有记录。';

  String get filteredEmptyDescription => '可以切回全部，或者先新增一条符合当前状态的记录。';

  String get clearFilterActionLabel => '清除筛选';

  String get listEmptyLabel => '暂无记录，请先从首页新增一条记录。';

  String get createPageTitle => '新建记录';

  String get editPageTitle => '编辑记录';

  String get titleFieldLabel => '标题';

  String get titleFieldRequiredError => '请输入标题';

  String get categoryFieldLabel => '分类';

  String get dateFieldLabel => '更新时间';

  String get noteFieldLabel => '备注';

  String get noteFieldHint => '补充交付物、状态说明或下一步动作';

  String get createSubmitLabel => '保存';

  String get editSubmitLabel => '更新';

  String get detailPageTitle => '记录详情';

  String get editActionLabel => '编辑';

  String get deleteActionLabel => '删除';

  String get deleteDialogTitle => '删除记录';

  String get deleteDialogMessage => '删除后不可恢复，确认继续吗？';

  String get detailCategoryLabel => '分类';

  String get detailStatusLabel => '状态';

  String get detailDateLabel => '更新时间';

  String get detailNoteLabel => '备注';

  String get emptyNoteLabel => '暂无备注';

  String get allFilterLabel => '全部';

  String get inboxFilterLabel => '待整理';

  String get inProgressFilterLabel => '进行中';

  String get doneFilterLabel => '已完成';

  String statusLabel(RecordStatus status) {
    switch (status) {
      case RecordStatus.inbox:
        return inboxFilterLabel;
      case RecordStatus.inProgress:
        return inProgressFilterLabel;
      case RecordStatus.done:
        return doneFilterLabel;
    }
  }
}