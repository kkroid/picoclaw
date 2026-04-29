# 🐛 疑难解答

> 返回 [README](../../README.zh.md)

## "model ... not found in model_list" 或 OpenRouter "free is not a valid model ID"

**症状：** 你看到以下任一错误：

- `Error creating provider: model "openrouter/free" not found in model_list`
- OpenRouter 返回 400：`"free is not a valid model ID"`

**原因：** `model_list` 条目中的 `model` 字段是发送给 API 的内容。对于 OpenRouter，你必须使用**完整的**模型 ID，而不是简写。

- **错误：** `"model": "free"` → OpenRouter 收到 `free` 并拒绝。
- **正确：** `"model": "openrouter/free"` → OpenRouter 收到 `openrouter/free`（自动免费层路由）。

**修复方法：** 在 `~/.picoclaw/config.json`（或你的配置路径）中：

1. **agents.defaults.model_name** 必须匹配 `model_list` 中的某个 `model_name`（例如 `"openrouter-free"`）。
2. 该条目的 **model** 必须是有效的 OpenRouter 模型 ID，例如：
   - `"openrouter/free"` – 自动免费层
   - `"google/gemini-2.0-flash-exp:free"`
   - `"meta-llama/llama-3.1-8b-instruct:free"`

示例片段：

```json
{
  "agents": {
    "defaults": {
      "model_name": "openrouter-free"
    }
  },
  "model_list": [
    {
      "model_name": "openrouter-free",
      "model": "openrouter/free",
      "api_key": "sk-or-v1-YOUR_OPENROUTER_KEY",
      "api_base": "https://openrouter.ai/api/v1"
    }
  ]
}
```

在 [OpenRouter Keys](https://openrouter.ai/keys) 获取你的密钥。

## AppFactory Flutter APK 构建报 AAPT2 `Permission denied`

**症状：** `check-flutter-build-apk` 或手工执行 `flutter build apk --debug --no-pub` 失败，日志里出现类似报错：

- `AAPT2 ... Daemon startup failed`
- `Cannot run program ".../aapt2": error=13, Permission denied`

**原因：** Gradle 会把 `aapt2` 解压到 AppFactory 的 `.runtime/gradle-user-home` 缓存目录下。某些环境里这个二进制会被解压成 `0644`，导致 AAPT2 守护进程无法启动，Flutter 最后还可能误报成 “Gradle does not have execution permission”。

**当前行为：** 运行器在执行 `check-flutter-build-apk` 时，如果日志命中上述 `aapt2` 权限错误，会自动给缓存里的 `aapt2` 补执行位并重跑一次同一步，无需手工清缓存。

**手工排查：** 如果你在运行器外单独复现，也可以先检查并修复该文件权限：

```sh
find ~/.picoclaw/workspace/appfactory/.runtime/gradle-user-home -type f -path '*/transformed/aapt2-*-linux/aapt2' -exec chmod 755 {} +
```

如果修复后仍失败，再继续查看同一份构建日志中 `:app:processDebugResources` / `:app:processReleaseResources` 附近的 AAPT2 原始错误，而不要只看 Flutter 最后的通用提示。

## AppFactory `/jobs` 回归结果看不懂

**症状：** `scripts/run-appfactory-jobs-regression.sh` 已经跑到 terminal failure，但旧的 poll 行或原始事件摘要很难直接看出“到底卡在哪个前线、先看哪个文件”。

**当前行为：** 回归脚本现在会在结束时打印一段 `== readable summary ==`，并把同样的结构化结论写到 `workspace/appfactory/jobs-ui-regression/latest.json` 和 `workspace/appfactory/jobs-ui-regression/latest.md`。同时，每次验证的完整控制台输出也会自动落盘到本次 `run_dir` 下的 `console.log`，并同步覆盖 `workspace/appfactory/jobs-ui-regression/latest.log`。

推荐按下面顺序读：

- `result`：先确认 run 是成功还是失败，以及总耗时。
- `frontier`：当前最具体的失败边界。脚本会优先选 patch apply / patch generation / run_failed 这类真正贴近问题的事件，而不是泛化的 orchestrator 结束事件。
- `focus_path`：第一优先级要检查的文件或路径集合。
- `failure`：压缩后的原始失败摘要，方便快速扫读。
- `explanation`：对常见失败签名给出的可读解释。
- `last_success`：失败前最后一个已确认成功的关键事件，用来判断前线到底推进到了哪里。
- `builder_log` 与 `event_log`：如果 summary 还不够，再继续打开这两个产物深挖。
- `console_log`：本次验证从 compile 到 terminal summary 的完整控制台输出，适合回看 poll 节奏和脚本行为。
- `latest_console_log`：当前最新一次验证的固定日志入口，不想先找 `run_dir` 时优先看它。

**实用阅读顺序：** 先看 `frontier` 和 `focus_path`，再看 `explanation`。如果还想知道脚本当时到底打印了什么，就直接打开 `console_log` 或 `latest_console_log`；只有这些仍不足以定性时，再去看 `builder_log` 和完整事件流。

## AppFactory `/jobs` strict 回归在 preflight 就报 `gateway_start_allowed=false`

**症状：** `scripts/run-appfactory-jobs-regression.sh` 还没进入 compile/create，就直接在 `== preflight runtime mode ==` 之后失败；`latest.md` 或 readable summary 里能看到类似：

- `runtime_mode=builder_runtime_blocked`
- `failure_signature=gateway_start_blocked`
- `gateway_start_reason=default model "..." has no credentials configured`

**当前行为：** strict 回归现在把 `/api/gateway/status.gateway_start_allowed=false` 当成 hard gate。只要 gateway 已经明确说“当前默认 builder model 不能启动”，脚本就不会再继续 compile/create/start，因为后面的 `/jobs` 结论一定会被环境漂移污染。

**排查顺序：**

- 先看 `config_path` 和 `uses_user_home_config`，确认 launcher 读的是不是你预期的那份配置。
- 再看 `builder_runtime_default_model` 和 `gateway_start_reason`，确认当前默认模型是谁、缺的是凭据、endpoint 还是其他运行前置条件。
- 只有在 `gateway_start_allowed=true` 之后，再去看 completion probe、patch generation waiting 或 builder-runtime 代码前线；否则继续跑 strict `/jobs` 只会得到噪声。

## AppFactory builder runtime 返回空 patch body

**症状：** builder log 里能看到某次 patch 生成的模型头，但正文是空的，或者 patch generation 之后立刻报 `unexpected end of JSON input` 这类 parse 错误。

**当前行为：** 运行时现在把“空 patch body”视为该 model alias 的一次软失败，而不是立刻把整轮请求判死。如果当前 route 还配置了 fallback alias，会先自动尝试下一个 alias；如果所有 alias 最终都只返回空 body，运行时仍会把这份空响应继续送进 schema repair，这样单模型路由和 fallback 全部耗尽的场景仍然可以基于当前 workspace 状态重试，而不是停在最原始的 parse failure。

**排查方式：** 如果 run 仍然停在这里，先打开 builder log，确认是只有第一个 alias 为空，还是所有 alias 都返回了空 body / 截断 body。现在“只有主 alias 为空”的情况，前线应该继续推进到 fallback alias；如果所有 alias 都为空，下一道有意义的前线应该是 schema repair，而不是最初那次 patch attempt。

## AppFactory inventory relation-rich controller 漂移回 `uuid` 或占位空壳

**症状：** 针对 `examples/appfactory/relation-rich/inventory-sheet-line-item/requirement.md` 的 fresh `/jobs` 已经越过 model / repository，但 controller 生成结果里仍出现明显漂移，例如：

- `lib/controllers/record_form_controller.dart` 里重新引入 `package:uuid/uuid.dart` 或 `const Uuid()`
- `lib/controllers/home_controller.dart` 里出现 `_waresides`、project 领域 API 或空壳 getter
- `lib/controllers/record_list_controller.dart` 里出现 `_applyFilters()`、`_filterlyType`、`InventoryListControllerFixed`、重复 controller class 等占位内容

**当前行为：** builder runtime 现在把 `inventory-sheet-line-item` 识别为独立的 relation-rich profile，并会把 `lib/controllers/home_controller.dart`、`lib/controllers/record_form_controller.dart`、`lib/controllers/record_list_controller.dart` 统一 canonicalize 到 inventory 领域专属 contract，确保它们和 `inventory_sheet.dart`、`line_item.dart`、`sku.dart`、`warehouse.dart`、`dashboard_summary.dart` 以及 `record_repository.dart` 的当前语义口径一致。

**排查方式：** 如果 controller 阶段仍然看到上述漂移，先确认当前 `/jobs` 是否跑在最新重建的 launcher 上。最可靠的判断信号不是原始模型输出，而是 live workspace 里最终落地的 controller 文件内容。重新构建 `build/picoclaw-launcher`，在隔离端口重启 launcher 后再发起 fresh `/jobs`；如果修复已生效，最终 workspace 里的 controller 文件不应再包含 `uuid`、project/tag API、`_waresides`、`_applyFilters()` 或重复的 fallback controller class。
