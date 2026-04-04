# AppFactory Public Job 设备验证运行手册

本文档说明如何使用仓库内已经提供的 AppFactory public-job 设备验证入口，对默认 Flutter public job 产物执行最小 adb 验证闭环，并收集 `device-logcat.txt`、`device-screenshot.png` 等运行证据。

## 1. 目标

- 复用真实 public-job 工作区生成链路，而不是手工拼一个临时 Flutter 示例目录。
- 在 `flutter analyze`、`flutter test`、`flutter build apk --debug` 之后，继续执行 adb 设备在线检查、安装、拉起和最小日志采集。
- 把设备验证证据稳定写入 job 的 `reports/` 目录，供 jobs 页交付表单和后续排障继续使用。

## 2. 入口

默认入口：

```bash
make verify-appfactory-public-job-device
```

跳过 builder 镜像重建的快速入口：

```bash
make verify-appfactory-public-job-device-fast
```

历史 `metrics.json` 的设备失败类别汇总入口：

```bash
make verify-appfactory-public-job-device-summary
```

设备失败告警入口：

```bash
make check-appfactory-public-job-device-alerts
```

完整设备回归循环入口：

```bash
make run-appfactory-public-job-device-regression-fast
```

固定设备池模式的完整设备回归入口：

```bash
make run-appfactory-public-job-device-regression-pool-fast
```

设备回归历史索引重建入口：

```bash
make update-appfactory-public-job-device-regression-index
```

固定设备池状态看板重建入口：

```bash
make update-appfactory-public-job-device-pool-status
```

该入口默认会同时写出 `workspace/appfactory/reports/device-failure-summary.md` 和 `workspace/appfactory/reports/device-failure-summary.json`；如果想改路径，可覆盖 `APPFACTORY_DEVICE_METRICS_ROOT`、`APPFACTORY_DEVICE_METRICS_OUTPUT` 或 `APPFACTORY_DEVICE_METRICS_JSON_OUTPUT`。

这两个入口最终都会调用 `scripts/verify-appfactory-public-job.sh`，区别只是前者会先执行 `make build-appfactory-builder`，后者直接复用现有 `APPFACTORY_BUILDER_IMAGE`。

如果要启用固定设备池，先把 [config/appfactory-device-pool.example.json](../../../config/appfactory-device-pool.example.json) 复制为 `config/appfactory-device-pool.json`，再按本机模拟器/真机 serial、adb server 和 Docker 参数调整配置。当前仓库已经落了一份本地设备池配置样例 `config/appfactory-device-pool.json`，可直接按实际设备变更 serial / label / tags。对于当前 Linux 宿主机，如果 adb server 只监听本机回环地址，设备池配置应配合 `docker_args="--network host"` 与 `adb_server_socket="tcp:127.0.0.1:5037"`，不要继续沿用默认的 `host.docker.internal` 方案。

## 3. 前置条件

- 本机可执行 `docker`。
- 已存在可用的 AppFactory builder 镜像，或者允许入口自动构建镜像。
- 至少有一个 Android 模拟器或真机可被 adb 访问。
- 如果 adb server 运行在宿主机，容器内默认使用 `ADB_SERVER_SOCKET=tcp:host.docker.internal:5037` 连接宿主机 adb server。

## 4. 常用环境变量

| 变量 | 说明 | 默认值 |
|---|---|---|
| `APPFACTORY_DEVICE_VERIFICATION_ENABLED` | 是否启用设备验证；device 入口会自动设为 `1` | `0` |
| `APPFACTORY_DEVICE_SERIAL` | 指定目标设备序列号；多设备时建议显式设置 | 空 |
| `APPFACTORY_ANDROID_APP_ID` | 指定 Android applicationId；不设时脚本会尝试从 Gradle 文件解析 | 空 |
| `APPFACTORY_DEVICE_CAPTURE_SCREENSHOT` | 是否额外采集 `device-screenshot.png` | `0` |
| `APPFACTORY_DEVICE_SUMMARIZE_METRICS` | device 入口结束后是否额外生成一次 `device-failure-summary.md` | `1` |
| `APPFACTORY_PUBLIC_JOB_DEVICE_DOCKER_ARGS` | 透传给 `docker run` 的额外参数，用于映射 USB、共享网络或挂载 adb 相关 socket | 空 |
| `ADB_SERVER_SOCKET` | adb server 地址；容器默认回连宿主机 `5037` | `tcp:host.docker.internal:5037` |
| `KEEP_GENERATED_WORKSPACE` | 是否保留生成的临时工作区，便于排障 | `0` |
| `APPFACTORY_BUILDER_IMAGE` | 指定 builder 镜像 tag | `picoclaw/appfactory-builder:local` |
| `APPFACTORY_DEVICE_METRICS_ROOT` | 设备失败汇总脚本扫描的 appfactory 根目录 | `workspace/appfactory` |
| `APPFACTORY_DEVICE_METRICS_OUTPUT` | 设备失败汇总脚本输出 Markdown 路径；summary wrapper 默认写到 `<metrics_root>/reports/device-failure-summary.md` | 空 |
| `APPFACTORY_DEVICE_METRICS_JSON_OUTPUT` | 设备失败汇总脚本输出 JSON 路径；summary wrapper 默认写到 `<metrics_root>/reports/device-failure-summary.json` | 空 |
| `APPFACTORY_DEVICE_ALERT_REFRESH_SUMMARY` | 告警脚本执行前是否先刷新 summary | `1` |
| `APPFACTORY_DEVICE_ALERT_MAX_TOTAL_COUNT` | 单个 failure category 的 `total_count` 达到该阈值即返回非零退出码 | `0` |
| `APPFACTORY_DEVICE_ALERT_MAX_RUN_COUNT` | 单个 failure category 的 `run_count` 达到该阈值即返回非零退出码 | `0` |
| `APPFACTORY_DEVICE_ALERT_FOCUS_CATEGORIES` | 逗号分隔的重点 failure category；只要出现就返回非零退出码 | 空 |
| `APPFACTORY_DEVICE_REGRESSION_ROOT` | 设备回归循环脚本的归档根目录 | `workspace/appfactory/device-regression` |
| `APPFACTORY_DEVICE_REGRESSION_MODE` | 设备回归循环模式，支持 `fast` / `full` | `fast` |
| `APPFACTORY_DEVICE_REGRESSION_VERIFY_COMMAND` | 覆盖默认 verify 命令，便于接 mock 或自定义入口 | 空 |
| `APPFACTORY_DEVICE_REGRESSION_RUN_ALERTS` | 回归循环结束后是否继续执行 alert check | `1` |
| `APPFACTORY_DEVICE_REGRESSION_FAIL_ON_ALERT` | alert 触发时是否让回归循环返回非零退出码 | `1` |
| `APPFACTORY_DEVICE_REGRESSION_KEEP_TEMP` | 回归循环归档完成后是否继续保留原始临时工作区 | `0` |
| `APPFACTORY_DEVICE_REGRESSION_INDEX_OUTPUT` | 回归历史索引 JSON 输出路径 | `<regression_root>/index.json` |
| `APPFACTORY_DEVICE_REGRESSION_LATEST_MARKDOWN` | 回归最新状态 Markdown 输出路径 | `<regression_root>/latest.md` |
| `APPFACTORY_DEVICE_POOL_ENABLED` | 是否启用固定设备池租约治理；启用后回归脚本会先 claim 再 verify，结束后 release | `0` |
| `APPFACTORY_DEVICE_POOL_CONFIG` | 固定设备池配置路径 | `config/appfactory-device-pool.json` |
| `APPFACTORY_DEVICE_POOL_STATE_ROOT` | 设备池租约与状态输出目录 | `workspace/appfactory/device-pool` |
| `APPFACTORY_DEVICE_POOL_REQUIRE_TAGS` | claim 设备时必须满足的 tags，逗号分隔；回归模式未显式指定 serial 时默认要求当前 `mode` | 空 |
| `APPFACTORY_DEVICE_POOL_PREFER_TAGS` | claim 设备时优先匹配的 tags，逗号分隔 | 空 |
| `APPFACTORY_DEVICE_POOL_LEASE_TTL_SECONDS` | 设备池 lease TTL；过期 lease 会在下一次 claim 时被回收 | `21600` |
| `APPFACTORY_DEVICE_POOL_STATUS_JSON_OUTPUT` | 固定设备池 JSON 状态输出路径 | `<state_root>/status.json` |
| `APPFACTORY_DEVICE_POOL_STATUS_MARKDOWN_OUTPUT` | 固定设备池 Markdown 状态输出路径 | `<state_root>/status.md` |

固定设备池配置样例：

```json
{
	"devices": [
		{
			"serial": "emulator-5554",
			"label": "pixel-6-api-34",
			"enabled": true,
			"tags": ["fast", "full", "emulator", "android-34"],
			"adb_server_socket": "tcp:host.docker.internal:5037",
			"docker_args": ""
		}
	]
}
```

## 5. 推荐用法

### 5.1 单设备最小验证

```bash
APPFACTORY_DEVICE_SERIAL=emulator-5554 \
make verify-appfactory-public-job-device-fast
```

适用场景：

- 宿主机已经跑着 adb server。
- 容器只需要通过 `host.docker.internal:5037` 访问 adb。

### 5.2 需要截图证据

```bash
APPFACTORY_DEVICE_SERIAL=emulator-5554 \
APPFACTORY_DEVICE_CAPTURE_SCREENSHOT=1 \
make verify-appfactory-public-job-device-fast
```

执行成功后，`reports/` 目录里除了 `device-logcat.txt` 之外，还会多一个 `device-screenshot.png`。

### 5.4 汇总历史设备失败类别

```bash
APPFACTORY_DEVICE_METRICS_ROOT=/path/to/workspace/appfactory \
APPFACTORY_DEVICE_METRICS_OUTPUT=/path/to/device-failure-summary.md \
APPFACTORY_DEVICE_METRICS_JSON_OUTPUT=/path/to/device-failure-summary.json \
make verify-appfactory-public-job-device-summary
```

如果直接使用默认路径：

```bash
make verify-appfactory-public-job-device-summary
cat workspace/appfactory/reports/device-failure-summary.md
cat workspace/appfactory/reports/device-failure-summary.json
```

适用场景：

- 已经保留了一批 `jobs/*/runs/*/metrics.json`，希望快速看最近设备失败分布。
- 还没有固定设备池或 CI，但已经需要手工做周期性回归盘点。
- 希望直接消费 `metrics.json.device_failure_categories[]`，而不是再次手工解析原始 `failure_signatures`。

### 5.5 按阈值触发设备失败告警

```bash
APPFACTORY_DEVICE_ALERT_MAX_TOTAL_COUNT=3 \
APPFACTORY_DEVICE_ALERT_MAX_RUN_COUNT=2 \
APPFACTORY_DEVICE_ALERT_FOCUS_CATEGORIES='device_check_failed:app_runtime_crash,device_check_failed:adb_device_unavailable' \
make check-appfactory-public-job-device-alerts
```

适用场景：

- 希望把当前 JSON 汇总直接接到 cron、CI 或 watcher，而不是人工盯表格。
- 需要在某个 failure category 持续出现时返回非零退出码，方便外层系统挂告警。
- 想先把告警逻辑固定下来，再接入真实固定设备池。

### 5.6 执行完整设备回归循环

```bash
APPFACTORY_DEVICE_SERIAL=emulator-5554 \
APPFACTORY_DEVICE_ALERT_MAX_TOTAL_COUNT=3 \
APPFACTORY_DEVICE_ALERT_FOCUS_CATEGORIES='device_check_failed:app_runtime_crash' \
make run-appfactory-public-job-device-regression-fast
```

该入口会顺序完成四件事：

- 执行一次 device verify。
- 把关键证据归档到 `workspace/appfactory/device-regression/runs/<timestamp>-<mode>/`。
- 刷新当前 summary / alert 输入。
- 把本次回归结果写入 `regression-result.json`，同时刷新 `index.json` 与 `latest.md`，必要时按 alert 阈值返回非零退出码。

适用场景：

- 需要一个可被 cron、CI 或值班脚本直接调用的单入口，而不是手工串 verify + summary + alert。
- 希望每次回归都保留独立归档，方便追查某一轮设备回归具体产物。
- 想在接固定设备池前，先把回归流程和退出码约定稳定下来。

### 5.7 执行固定设备池回归

```bash
cp config/appfactory-device-pool.example.json config/appfactory-device-pool.json
APPFACTORY_DEVICE_POOL_ENABLED=1 \
APPFACTORY_DEVICE_POOL_REQUIRE_TAGS=fast,emulator \
make run-appfactory-public-job-device-regression-pool-fast
```

该入口会在完整回归循环前自动执行：

- 从 `config/appfactory-device-pool.json` 中挑选满足 tags 的设备。
- 为目标 serial 创建 lease 文件，避免两轮回归同时占用同一台设备。
- 把选中的 `serial`、`label`、`adb_server_socket` 和可选 `docker_args` 注入 verify 流程。
- 回归结束后自动 release lease，并刷新 `workspace/appfactory/device-pool/status.json` 与 `status.md`。

适用场景：

- 已经有稳定模拟器或真机池，但还不想接完整 CI 编排。
- 希望本地 cron、值班机或单机 runner 能稳定串行占用设备，而不是继续靠人工口头协调。
- 需要把“哪台设备跑了哪轮回归”写进 machine-readable 结果，便于之后接看板或排障。

### 5.3 通过额外 Docker 参数暴露真机

```bash
APPFACTORY_PUBLIC_JOB_DEVICE_DOCKER_ARGS='--privileged -v /dev/bus/usb:/dev/bus/usb' \
APPFACTORY_DEVICE_SERIAL=R58NXXXXXXXX \
make verify-appfactory-public-job-device-fast
```

适用场景：

- 需要把宿主机 USB 设备直接暴露给容器。
- 当前环境不适合只通过宿主机 adb server 代理。

## 6. 成功信号与产物位置

脚本成功结束时，会输出以下关键信息：

- `workspace=`: 真实 public-job Flutter 工作区路径。
- `apk=`: 构建出的 `app-debug.apk` 路径。
- `device_logcat=`: 设备日志路径，仅在设备验证开启时存在。
- `device_screenshot=`: 截图路径，仅在截图开启且采集成功时存在。
- `device_failure_summary=`: 设备失败类别汇总路径，仅在设备验证开启且启用汇总时存在。
- `device_failure_summary_json=`: 设备失败类别 JSON 汇总路径，仅在设备验证开启且启用汇总时存在。
- `device_alert_status=`: 告警脚本返回的状态，取值 `ok` 或 `triggered`。
- `regression_result_json=`: 本次设备回归归档结果路径。

关键产物位于对应 job 临时目录下：

- `workspace/appfactory/jobs/<job_id>/workspace/build/app/outputs/flutter-apk/app-debug.apk`
- `workspace/appfactory/jobs/<job_id>/reports/device-logcat.txt`
- `workspace/appfactory/jobs/<job_id>/reports/device-screenshot.png`
- `workspace/appfactory/jobs/<job_id>/reports/device-failure-summary.md`
- `workspace/appfactory/jobs/<job_id>/reports/device-failure-summary.json`
- `workspace/appfactory/device-regression/runs/<timestamp>-<mode>/regression-result.json`
- `workspace/appfactory/device-regression/runs/<timestamp>-<mode>/verify.log`
- `workspace/appfactory/device-regression/index.json`
- `workspace/appfactory/device-regression/latest.md`
- `workspace/appfactory/device-pool/status.json`
- `workspace/appfactory/device-pool/status.md`

如果 `KEEP_GENERATED_WORKSPACE=1`，脚本还会打印 `preserved_temp_dir=`，便于后续进入工作区复查。

固定设备池模式下，`regression-result.json` 还会新增 `device` 对象，至少包含：

- `serial`
- `label`
- `claim_status`
- `owner_id`
- `lease_file`
- `claimed_at`
- `expires_at`

## 7. 失败信号

当前脚本把以下情况视为失败：

- `go test` 没有成功生成真实 public-job 工作区。
- builder 容器内任一 Flutter 步骤失败。
- 设备验证模式下，`adb wait-for-device`、安装、拉起或日志采集失败。
- `app-debug.apk` 缺失。
- 设备验证开启但 `device-logcat.txt` 为空或不存在。
- 要求截图但 `device-screenshot.png` 为空或不存在。

## 8. 排障建议

- 多设备并存时，优先显式设置 `APPFACTORY_DEVICE_SERIAL`，避免 adb 命中错误设备。
- 如果容器连不上 adb server，先在宿主机执行 `adb devices` 验证，再检查 `ADB_SERVER_SOCKET` 和 Docker 网络连通性。
- 如果需要深入排障，使用 `KEEP_GENERATED_WORKSPACE=1` 保留临时目录，然后直接查看 `reports/device-logcat.txt` 和 Flutter build 输出。
- jobs 页现在会把 `device-logcat.txt`、`device-screenshot.png`、`smoke-test-report.md`、`build-report.md`、`app-debug.apk` 作为设备验证和发布跟踪的建议证据；回归盘点和后续告警则优先读取 `metrics.json.device_failure_categories[]` 或汇总后的 `device-failure-summary.json`，再通过 `check-appfactory-public-job-device-alerts` 按阈值返回退出码；如果需要完整归档和单入口执行，则直接走 `run-appfactory-public-job-device-regression-fast`，如果需要稳定历史视图，则读取 `device-regression/index.json` 和 `device-regression/latest.md`，`device-failure-summary.md` 更适合人工浏览，不要再靠口头描述补洞。

## 9. 当前边界

当前这套入口已经支持固定设备池模式：`run-appfactory-public-job-device-regression-pool-fast` 会从 `config/appfactory-device-pool.json` 自动 claim/release 设备，并刷新 `workspace/appfactory/device-pool/status.json` / `status.md`。因此 item 5 所需的“最小设备验证闭环 + 固定设备池占用治理 + machine-readable 运维视图”已经收口；当前仍未完成的是把这套入口接进真正的 CI 调度系统，这部分属于后续持续回归机制，不再阻塞 item 5。