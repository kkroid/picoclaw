# 实施计划

- Job ID: job-weight-tracker-sample-001
- PRD ID: prd-weight-tracker-sample-001

## 阶段拆分

- 阶段 1：冻结体重记录实体、体重概览摘要模型和本地仓储边界。
- 阶段 2：搭建概览、历史集合、实体变更、结果检查四类承载单元并接通导航。
- 阶段 3：补齐新增、编辑、删除和结果检查主流程，为后续真实 validate 链路预留稳定输入包。

## 任务包

### 创建领域记录模型

- 目标：根据 domain model JSON 创建最终的领域记录实体，替代默认 generic record 语义。
- 分类：domain
- 目标路径：lib/models/record.dart
- 完成标准：领域记录字段已明确；导出的模型类型可被控制器和页面直接消费

### 创建首页摘要模型

- 目标：根据领域记录模型创建首页摘要模型，让首页指标直接承接领域语义。
- 分类：summary
- 依赖任务：task-create-record-model
- 目标路径：lib/models/dashboard_summary.dart
- 完成标准：摘要字段已明确；摘要模型可由领域记录推导

### 创建本地仓储实现

- 目标：固定本地持久化边界，为后续控制器和页面提供稳定的记录读写入口。
- 分类：storage
- 依赖任务：task-create-record-model
- 目标路径：lib/repositories/record_repository.dart
- 完成标准：本地持久化实现明确；记录读写入口稳定

### 创建概览承载控制器

- 目标：围绕领域记录与摘要模型创建概览承载控制器，承接概览入口与最近记录读取。
- 分类：summary
- 依赖任务：task-create-repository, task-create-summary-model
- 目标路径：lib/controllers/home_controller.dart
- 完成标准：概览承载控制器可读取摘要与最近记录；概览摘要字段与领域语义一致

### 创建集合浏览控制器

- 目标：围绕领域记录提供集合读取、排序和结果检查跳转所需的控制器接口。
- 分类：flow
- 依赖任务：task-create-repository, task-create-record-model
- 目标路径：lib/controllers/record_list_controller.dart
- 完成标准：集合浏览控制器可提供领域记录集合；集合浏览入口可浏览领域记录；结果检查入口与集合数据保持一致

### 创建领域文案投影

- 目标：基于参考模板创建 open_lite_copy.dart，使页面标题、标签和空态文案直接映射到领域语义。
- 分类：content
- 依赖任务：task-create-record-model
- 目标路径：lib/template/open_lite_copy.dart
- 完成标准：页面标题和表单标签体现领域命名；默认 seed 文案不再残留

### 创建 Android 启动器文案

- 目标：在 strings.xml 中创建 Android 启动器名称与品牌文案，确保不再残留默认 seed branding。
- 分类：content
- 依赖任务：task-create-copy
- 目标路径：android/app/src/main/res/values/strings.xml
- 完成标准：Android 启动器名称已切换到领域文案；默认 seed branding 不再残留

### 创建 Android 构建文案入口

- 目标：在 build.gradle.kts 中保留 Android branding 的单点覆盖入口，避免品牌改动分散到其他文件。
- 分类：content
- 依赖任务：task-create-android-branding
- 目标路径：android/app/build.gradle.kts
- 完成标准：Android branding 单点覆盖入口存在；applicationId 或 launcher branding 的改动路径明确

### 绑定概览承载单元

- 目标：根据概览承载控制器和领域文案绑定概览承载单元，承接领域摘要与最近记录入口。
- 分类：screen
- 依赖任务：task-create-home-controller, task-create-copy
- 目标路径：lib/views/home_page.dart
- 完成标准：概览承载单元可渲染领域摘要；承载单元标题与领域语义一致

### 绑定集合浏览承载单元

- 目标：根据集合浏览控制器和领域文案绑定集合浏览承载单元，提供记录浏览与结果定位入口。
- 分类：screen
- 依赖任务：task-create-list-controller, task-create-copy
- 目标路径：lib/views/record_list_page.dart
- 完成标准：集合浏览承载单元可浏览领域记录；结果检查入口与集合数据保持一致

### 绑定实体变更承载单元

- 目标：根据实体变更控制器和领域文案绑定实体变更承载单元，承接领域字段输入。
- 分类：screen
- 依赖任务：task-create-copy
- 目标路径：lib/views/record_form_page.dart
- 完成标准：实体变更承载单元字段与领域记录一致；保存动作已接线到控制器

### 绑定结果检查承载单元

- 目标：根据领域记录模型和文案绑定结果检查承载单元，承接单条记录展示、编辑和删除入口。
- 分类：screen
- 依赖任务：task-create-record-model, task-create-copy
- 目标路径：lib/views/record_detail_page.dart
- 完成标准：结果检查承载单元可展示单条领域记录；编辑和删除入口已预留

### 接线应用入口与承载单元路由

- 目标：把已创建的控制器、承载单元和仓储接线到应用入口，形成稳定的中性交互骨架。
- 分类：screen
- 依赖任务：task-bind-overview-surface, task-bind-collection-surface, task-bind-mutation-surface, task-bind-inspection-surface, task-create-home-controller, task-create-list-controller, task-create-form-controller, task-create-repository
- 目标路径：lib/main.dart
- 完成标准：应用入口完成接线；概览、集合浏览、实体变更、结果检查承载单元导航可达

### 创建最小 Widget 测试

- 目标：为当前领域承载单元和主流程创建最小 widget 测试，确保后续 analyze/test 收口有稳定入口。
- 分类：flow
- 依赖任务：task-bind-app-entry
- 目标路径：test/widget_test.dart
- 完成标准：test/widget_test.dart 已同步当前领域文案；测试入口可覆盖主承载单元或主流程

## 验收检查

- 安装 Flutter 依赖：stage=baseline，commands=flutter pub get
- 阻断 open-lite 默认 counter 模板：stage=cheap，commands=grep -q . lib/main.dart lib/models/record.dart lib/models/dashboard_summary.dart lib/views/home_page.dart lib/controllers/home_controller.dart lib/views/record_form_page.dart lib/controllers/record_form_controller.dart lib/views/record_list_page.dart lib/controllers/record_list_controller.dart lib/views/record_detail_page.dart lib/repositories/record_repository.dart && ! grep -E 'Flutter Demo Home Page|You have pushed the button this many times|Counter increments smoke test|_counter|_incrementCounter|MyHomePage' lib/main.dart test/widget_test.dart >/dev/null 2>&1
- 确认 open-lite CRUD 页面与控制器已接线：stage=cheap，commands=grep -E 'TextEditingController|TextFormField|DropdownButtonFormField|showDatePicker|RecordStatus|categoryController' lib/views/record_form_page.dart lib/controllers/record_form_controller.dart >/dev/null 2>&1 && grep -E 'HomePage' lib/main.dart >/dev/null 2>&1 && grep -E 'RecordListPage' lib/views/record_list_page.dart >/dev/null 2>&1 && grep -E 'RecordDetailPage' lib/main.dart lib/views/record_detail_page.dart lib/views/record_list_page.dart >/dev/null 2>&1 && grep -E 'deleteRecord' lib/main.dart lib/views/record_detail_page.dart lib/views/record_list_page.dart lib/controllers/home_controller.dart >/dev/null 2>&1
- 确认 open-lite 本地持久化已接线：stage=cheap，commands=grep -E 'hive_flutter|Hive' pubspec.yaml lib/repositories/record_repository.dart lib/main.dart >/dev/null 2>&1 && ! grep -E 'shared_preferences|SharedPreferences' pubspec.yaml lib/repositories/record_repository.dart lib/main.dart >/dev/null 2>&1
- 阻断 open-lite 默认 seed branding：stage=cheap，commands=! grep -ER 'Open Lite Seed|open_lite_seed' lib android/app/src/main/AndroidManifest.xml android/app/src/main/res/values/strings.xml >/dev/null 2>&1
- 确认领域字段与文案已进入 open-lite 工作区：stage=cheap，commands=grep -ER '体重|weight|recorded_at|latest_weight|trend' lib test android/app/src/main/res/values/strings.xml >/dev/null 2>&1
- 执行静态检查：stage=cheap，commands=flutter analyze
- 执行模板测试：stage=cheap，commands=flutter test
- 构建 Debug APK：stage=milestone，commands=flutter build apk --debug --no-pub
