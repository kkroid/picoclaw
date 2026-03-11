# Jarvis — 个人 AI 管家

## 产品定位

基于 UE5 数字人（MetaHuman + OneLive 音频表情）+ picoclaw AI 引擎，构建面向个人的 AI 管家产品。

**双模态交互**：
- 语音（数字人界面）：实时对话、口语问答、指令下达
- 消息（飞书 / Telegram）：长文本推送、信息订阅、异步通知

**核心功能**：
- 实时语音对话，有跨会话持久记忆
- 股票 / 新闻 / 事件订阅，主动推送关键信息
- 晨报（语音摘要 + 消息详情）
- 提醒系统（语音设置 → 消息通知）
- 阅读助手（发链接 → AI 总结回复）
- 可扩展 MCP 接入任意业务能力

**输出路由逻辑**（agent 自行判断）：
```
语音短问答       → TTS 语音回复
语音/消息长问答  → 语音播报摘要 + 消息发详情
股票/新闻监控   → 消息推送（重要事件同时触发语音提醒）
定时任务         → 消息通知
```

---

## 本地大模型选型

### 引擎选型：llama.cpp

Ollama 本质是 llama.cpp 的包装层。All-in-One 目标下，按三阶段向 llama.cpp 直接嵌入迁移：

| 方案 | 集成方式 | 独立进程 | 分发 | 阶段 |
|---|---|---|---|---|
| Ollama | HTTP（守护进程） | 是 | 高（需安装） | 当前过渡 |
| **llama-server** | HTTP（OpenAI 兼容） | 是（单 exe） | 中（单文件） | 阶段六前期 |
| **go-llama CGo** | in-process | 否 | 低（随 DLL）| 阶段六后期 ← 终态 |

- **llama-server**：llama.cpp 内置服务器，Windows 原生 exe，与 Ollama API 完全兼容，替换时只改 `api_base` 端口，代码零改动
- **go-llama**：llama.cpp 的 Go CGo 绑定，直接嵌入 `jarvis_ai.dll`，消除独立进程，实现真正 All-in-One

### 模型推荐（GGUF 格式，RTX 4090 24GB）

**单模型方案**：`Qwen3-4B-Q4_K_M`（~2.3GB，RTX 4090 全量 GPU offload，首字 ~150ms）

语音对话和分析任务用同一个模型，150ms 首字延迟完全满足实时对话，晨报/投研等异步场景更不在意。双模型只增加复杂度，等真正遇到质量瓶颈再引入第二个模型。

```json
// config.json（llama-server 替换 Ollama 后）
{
  "model_list": [
    {"model_name": "default", "model": "openai/qwen3-4b", "api_base": "http://127.0.0.1:8080/v1", "api_key": "none"}
  ]
}
```

### Agent 路由：任务 vs 闲聊

**结论**：任务场景本地 4B 够用；闲聊/知识问答本地模型质量不足，按需切云端大模型。

**为什么任务场景够用**：文档中所有任务（股票、天气、提醒、搜索、晨报、阅读助手）本质是工具调用——模型只需识别意图、格式化 JSON 参数、拼接工具返回结果。这是 pattern matching + JSON 生成，4B 完全胜任。

**为什么闲聊不够**：闲聊/深度知识问答需要大量知识密度、长对话连贯性和复杂推理，这恰恰是小模型的短板。

**路由方案**：picoclaw 支持 per-agent 配置不同模型，在 `jarvis-voice/handler.go` 中调用前做关键词路由，0ms 额外延迟：

```go
// handler.go
func routeAgent(text string) string {
    taskKeywords := []string{
        "股票", "天气", "提醒", "设置", "查询", "搜索", "新闻",
        "总结", "链接", "今天", "明天", "多少", "涨跌", "晨报",
    }
    for _, kw := range taskKeywords {
        if strings.Contains(text, kw) {
            return "jarvis-task" // 本地 4B
        }
    }
    return "jarvis-chat" // 云端大模型
}
```

```json
// config.json 双 agent 配置（尚未固化云端选型，模型名待定）
{
  "model_list": [
    {"model_name": "local", "model": "openai/qwen3-4b",    "api_base": "http://127.0.0.1:8080/v1",      "api_key": "none"},
    {"model_name": "cloud", "model": "openai/TO_BE_DECIDED", "api_base": "https://api.TO_BE_DECIDED/v1", "api_key": "sk-xxx"}
  ],
  "agents": {
    "jarvis-task": {"model_name": "local"},
    "jarvis-chat": {"model_name": "cloud"}
  }
}
```

**云端选型候选**（尚未决定，参考）：

| 供应商 | 模型 | 需代理 | 价格参考 | 备注 |
|---|---|---|---|---|
| DeepSeek | deepseek-v3 | 否 | 极低 | 国内直连，中文强，OpenAI 兼容 |
| 通义千问 | qwen-plus | 否 | 有免费额度 | 国内直连，OpenAI 兼容 |
| Claude | haiku-3.5 | 是 | 低 | 速度快 |

> 此方案当前**不实现**，待任务场景稳定后再引入。关键词路由准确率约 85%，误判代价低（多调几次云端 API），是务实起点。规则不够时可升级为本地小分类模型（fasttext，~1MB）。

---

## 消息渠道选型

picoclaw 原生支持 17 个渠道，无需额外开发。选型原则：国内可用 + 免费 + API 完整。

| 渠道 | 需代理 | 个人注册 | 文件/图片 | 结论 |
|---|---|---|---|---|
| **飞书** | 否 | 免费个人版 | 支持 | 生产首选 |
| **Telegram** | 是 | 免费 | 支持 | 开发调试首选 |
| 企微 | 否 | 需企业认证 | 支持 | — |
| OneBot/QQ | 否 | 需第三方客户端 | 支持 | 有封号风险，不推荐 |

**结论**：飞书（生产）+ Telegram（开发）双配置并行，picoclaw 切换零成本。

飞书配置：创建自建应用 → 获取 App ID/Secret → 填入 `channels.feishu`。

---

## 服务部署方案

### 当前开发环境（过渡方案，阶段一~五）

```
Windows 主机
├── llama-server port 8080：Qwen3-4B-Q4_K_M（全功能统一模型）
└── WSL2 (ubuntu2404)
    ├── jarvis-voice   api_base: http://10.255.255.254:8080/v1
    └── picoclaw       api_base: http://10.255.255.254:8080/v1

JarvisCore（UE5，Windows）→ WSL jarvis-voice :18790
```

> 注：当前仍用 Ollama 过渡（已配置 OLLAMA_HOST=0.0.0.0）。阶段六前期替换为 llama-server，API 完全兼容，只改端口，无需修改任何业务代码。

### 中期生产环境（Docker，阶段五）

```
Linux 服务器（有 GPU）
├── llama-server 容器   GPU 推理（Qwen3-4B + Qwen2.5-7B），端口 8080/8081
├── jarvis-voice 容器   连接 llama-server
├── picoclaw 容器       连接 llama-server
└── nginx 容器          TLS 终止，反向代理
```

### 终态（All-in-One DLL，阶段六）

```
Windows / Linux（有 GPU）
└── UE5 进程（OneLiveUE）
    └── OneJarvis Plugin：
        ├── mm_vad.dll        VAD / NS / AEC / AGC
        ├── libonelive.dll    面部表情音频驱动
        └── jarvis_ai.dll     Go c-shared，AI 管家全量
            ├── picoclaw Agent  记忆 / 工具 / MCP / 消息渠道
            ├── ASR             豆包流式
            ├── TTS             豆包流式
            └── llama.cpp (CGo) in-process，无独立进程
```

---

## 仓库信息

- Fork 地址：`git@github.com:kkroid/picoclaw.git`
- 工作分支：`feature/jarvis`

---

## 当前进度（2026-03-12）

| 阶段 | 状态 | Commit |
|---|---|---|
| 一：MVP 语音对话 | ✅ 完成 | `de8755c` (picoclaw) |
| 一 Sprint 1.6：真正流式 ASR | ✅ 完成 | `de8755c` (picoclaw) |
| JarvisCore 重构 v4 + 重连退避 | ✅ 完成 | `da975de` / `3b1a6bf` |
| 二：消息渠道接入（飞书 / Telegram 双向） | 下一步 | — |
| 三：信息订阅（股票 / 新闻 / 晨报） | 待做 | — |
| 四：个人效率（提醒 / 速记 / 阅读助手） | 待做 | — |
| 五：Docker 部署 | 待做 | — |

**下一步**：阶段二——配置飞书渠道，实现语音→消息的输出路由

---

## 架构概览

```
[JarvisCore] UE5 C++ 数字人客户端（端侧 VAD，xiaozhi WebSocket 协议）
    ↕
[jarvis-voice] 语音网关（Go，cmd/jarvis-voice/）
    ├── ASR 豆包云端流式 → 文字
    ├── LLM qwen3:4b 本地（Ollama）→ 流式 token
    ├── TTS 豆包云端流式 → Opus 音频帧 → JarvisCore
    └── 长内容 → 转发 picoclaw 消息渠道
    ↕ 进程内直接调用（无 HTTP 跳转）
[picoclaw] AI 引擎（Go，picoclaw fork feature/jarvis）
    ├── 持久记忆（memory_id = device_id）
    ├── 工具：DuckDuckGo 搜索 / WebFetch / CronTool / MCP
    ├── 消息渠道：飞书 + Telegram 双向文字
    └── AgentLoop（RunStreamAgentLoop）
    ↕
[Ollama] 本地大模型
    ├── qwen3:4b    语音对话（延迟优先）
    └── qwen3.5:27b 分析摘要（质量优先）

[picoclaw-web] 管理 UI（React，端口 18800）
    └── 可视化编辑 config.json（LLM 凭证 / 模型 / 渠道 / 日志）
```

> **以上为当前（阶段一完成）架构**，多进程分布式方案。All-in-One 目标（阶段六）是将上图全量逻辑收入 `jarvis_ai.dll`，随 OneJarvis 插件分发，消除所有独立进程，与 `libonelive.dll`、`mm_vad.dll` 同等地位。

---

## PicoClaw 侵入性分析

PicoClaw 仍在快速迭代，所有修改遵循 **"只增不改"** 原则，最大限度降低合并冲突风险。

### 现有文件改动清单（最终实现）

| 文件 | 改动量 | 原因 |
|---|---|---|
| `pkg/providers/http_provider.go` | **+11 行** | `HTTPProvider` bug fix：缺少 `ChatStream()` 转发，导致 `StreamingProvider` type assertion 失败。**PR 候选**，合并后可从 fork 删除 |
| `go.sum` | +若干行 | 新增包的哈希，不可避免 |

**`helpers.go`、所有 gateway 文件：零改动。**

### 为什么能做到近零改动？

1. **`openai_compat.Provider` struct 是导出类型**（`type Provider struct`）  
   → 在同包新文件 `openai_compat/streaming.go` 里直接给它加 `ChatStream()` 方法，不动 `provider.go`

2. **`StreamingProvider` 接口** 放新文件 `pkg/providers/streaming.go`，同包，不动 `types.go`

3. **`RunStreamAgentLoop()`** 放新文件 `pkg/agent/stream.go`，不动任何现有 agent 文件

4. **`jarvis-voice` 直接托管 AgentLoop**：通过公开 API（`pkg/config.LoadConfig` + `agent.NewAgentLoop`）在进程内初始化，完全消除对 picoclaw HTTP 端点的依赖，无需在 `helpers.go` 注册任何路由

### jarvis-voice 独立 Go Module

`cmd/jarvis-voice/` 是独立的 Go module（`github.com/sipeed/picoclaw/jarvis-voice`），有自己的 `go.mod`：
- 通过 `replace github.com/sipeed/picoclaw => ../../` 引用本地 fork
- Opus CGo 依赖（`hraban/opus.v2`）隔离在此 module，picoclaw 主 module 保持 `CGO_ENABLED=0` 纯 Go

### 新增文件清单

```
pkg/providers/
    streaming.go              ← StreamingProvider 接口（新增）
pkg/providers/openai_compat/
    streaming.go              ← ChatStream() 实现（新增，同包扩展 Provider struct）
pkg/agent/
    stream.go                 ← RunStreamAgentLoop()（新增）
pkg/asr/                      ← ASR 接口 + mock + doubao provider（新增）
pkg/tts/                      ← TTS 接口 + mock + doubao provider（新增）
cmd/jarvis-voice/             ← 独立语音网关 module（新增）
    go.mod / go.sum
    main.go                   ← AgentLoop 初始化 + WebSocket server
    handler.go                ← session 生命周期 + ASR→LLM→TTS pipeline
    handler_test.go           ← lastSentenceBreak 单元测试（9 cases，全绿）
    .env.example
```

### 合并策略

- 上游更新时：`git merge upstream/main`
- 冲突风险：**极低**。现有文件只有 `http_provider.go` +11 行，其余全为新增文件，无冲突
- 若上游修复了同一 bug（`HTTPProvider.ChatStream`）：删除我们的改动即可，stream.go/streaming.go 不受影响
- 若上游重构了 `openai_compat.Provider` struct 名称：只需更新 `streaming.go` 中的 receiver 类型

---

## 目录结构

```
picoclaw/              (fork 根目录)
├── cmd/
│   ├── picoclaw/      (现有，新增 /voice/stream 路由)
│   └── jarvis-voice/  (新增，语音网关 binary)
├── pkg/
│   ├── asr/           (新增)
│   │   ├── provider.go    接口定义
│   │   ├── xunfei/        讯飞 WebSocket
│   │   └── aliyun/        阿里云 NLS WebSocket
│   ├── tts/           (新增)
│   │   ├── provider.go    接口定义
│   │   ├── xunfei/        讯飞 WebSocket
│   │   ├── huoshan/       火山引擎 WebSocket
│   │   └── http/          通用 HTTP 流式 adapter (fish-speech 等本地 AI 服务)
│   ├── admin/         (新增，阶段二)
│   ├── configdb/      (新增，阶段二；SQLite)
│   ├── providers/     (修改：新增 StreamingProvider 接口)
│   │   └── openai_compat/ (修改：实现 ChatStream)
│   └── agent/         (修改：新增 stream.go)
└── web/               (已有；React + shadcn/ui，阶段二扩展 ASR/TTS/设备页面)
```

---

## 阶段一：MVP — 语音对话跑通

**目标**：JarvisCore → 说话 → 听到 AI 回答，有记忆，YAML 配置，无 UI。
**工期估算：2.5 周**

### Sprint 1.1 — 仓库 + 流式 LLM（4 天）*关键路径* ✅

1. Fork PicoClaw，建 `feature/jarvis` 分支，新增目录骨架
2. `pkg/providers/types.go`：新增 `StreamingProvider` 接口，不破坏现有 `Chat()`

   ```go
   type StreamingProvider interface {
       LLMProvider
       ChatStream(ctx context.Context, messages []Message, tools []ToolDefinition,
           model string, options map[string]any, onToken StreamCallback) error
   }

   type StreamCallback func(StreamToken)

   type StreamToken struct {
       Type     string        // "token" | "tool_call" | "tool_call_delta" | "done"
       Content  string
       ToolCall *ToolCallDelta
   }
   ```

3. `openai_compat/provider.go` 实现 `ChatStream()`：
   - body 加 `"stream": true`
   - `bufio.Scanner` 逐行读 SSE
   - 解析 `delta.content` 和 `delta.tool_calls`
   - 覆盖 DeepSeek/Qwen/GPT/Groq/火山 等全部 OpenAI 兼容 provider

4. `anthropic/provider.go` 实现流式（anthropic Go SDK 已有 `NewStreaming()`）

5. `pkg/agent/stream.go` 新增 `RunStreamAgentLoop()`：
   - 复用 `ContextBuilder.BuildMessages()`（memory + skills + SOUL 组装，不改 context.go）
   - 调 `ChatStream()`，onToken 回调写入 SSE response writer
   - tool_call 到达时：暂停 token 流 → 执行 MCP → 注入 result → 继续流
   - 完成后调 `Sessions.AppendMessage()` 写记忆

6. `cmd/picoclaw/` 新增路由（`lang` 由 Agent 配置决定，不在请求体里传）：
   ```
   POST /voice/stream
   Content-Type: application/json
   Body: { "session_id": "device-mac", "text": "..." }
   Response: text/event-stream SSE
     data: {"type":"token","content":"你好"}
     data: {"type":"tool_call","name":"get_weather","args":{...}}
     data: {"type":"done"}
   ```

### Sprint 1.2 — ASR 插件系统（2 天）✅

7. `pkg/asr/provider.go` 接口：
   ```go
   // 音频格式全局约定：16kHz 16-bit mono PCM，ASR/TTS/Opus 编解码均以此为准，不再传递 sampleRate
   type Provider interface {
       Name() string
       // 端侧 VAD 已完成，服务端收到的是有效语音段；PCM 格式见上方约定
       Transcribe(ctx context.Context, pcm []int16) (string, error)
   }
   type Factory func(cfg map[string]any) (Provider, error)
   ```

8. `pkg/asr/xunfei/`：讯飞实时识别 WebSocket
9. `pkg/asr/aliyun/`：阿里云 NLS WebSocket
10. Provider 注册工厂：`asr.Register(name, factory)`，按 config.yaml 的 `asr.provider` 字段选择
11. 云端 provider per-connection 实例

### Sprint 1.3 — TTS 插件系统（2 天）✅

12. `pkg/tts/provider.go` 接口：
    ```go
    // 输出 PCM 格式同全局约定（16kHz 16-bit mono），调用方负责 Opus 编码后推给客户端
    type Provider interface {
        Name() string
        SynthesizeStream(ctx context.Context, text, voice string,
            onChunk func(pcm []int16)) error
    }
    type Factory func(cfg map[string]any) (Provider, error)
    ```

13. `pkg/tts/xunfei/`：讯飞 TTS WebSocket 流式
14. `pkg/tts/huoshan/`：火山引擎 TTS WebSocket 流式
15. `pkg/tts/http/`：通用 HTTP chunked streaming adapter（给 fish-speech 等本地 AI 服务用，POST text → stream PCM 回）

### Sprint 1.4 — jarvis-voice 语音网关（3 天）✅

16. WebSocket 服务器，路径 `/xiaozhi/v1/`，完整实现 xiaozhi 消息协议：
    ```
    客户端 → 服务端:  hello | {type:"listen", state:"start"|"end", audio binary}
                      | {type:"abort"}
    服务端 → 客户端:  {type:"hello"} | {type:"tts", state:"start"|"sentence_start"|"sentence_end"|"stop"}
                      | {type:"llm", emotion, text} | audio binary (Opus 帧)
    ```

17. `hello` 消息中提取 `device-id`（MAC 地址），作为 PicoClaw `session_id`（持久记忆键）；按 YAML 配置选 ASR/TTS/LLM provider
18. 接收二进制音频帧 → Opus 解码（`pion/opus`，固定 16kHz mono）→ PCM `[]int16`；端侧 VAD 已过滤，无需服务端 VAD
19. 收到 `listen.state="end"` 时：PCM → ASR → text → 直接调用 `agentLoop.RunStreamAgentLoop()`（进程内，无 HTTP 跳转）
20. `onToken` 回调内实时断句（`。？！！
.!?`） → 逐句触发 TTS
21. TTS 首帧到达 → 先推 `tts.state=sentence_start` → Opus 编码帧 → 推回 JarvisCore
22. abort 消息：`context.CancelFunc` 取消 ASR/LLM/TTS 全链路 goroutine，清空状态

### Sprint 1.5 — MVP 验收（1 天）✅

23. `cmd/jarvis-voice/.env.example` 配置样例文件 ✅
24. 单元测试：`handler_test.go` 覆盖 `lastSentenceBreak`（9 cases）+ thinking 状态机（5 cases），共 15 个测试全绿 ✅
25. 全链路延迟日志 ✅（ASR latency / llm_first_token latency / llm sentence / llm done / tts skip）
26. MVP checklist：开口说话 → ASR 识别 → LLM 流式回复（有跨会话记忆）→ TTS 流式播放 ✅

**Sprint 1.5 期间修复的关键问题：**

| 问题 | 根因 | 修复 |
|---|---|---|
| ASR 静默后 FSM 卡在 PROCESSING | ASR 空结果时服务端无回复，客户端无法复位 | 空结果/错误时发 `tts.state=stop` |
| buffer underrun（句间停顿） | 串行流水线：LLM 生句→TTS→播放，句间等待 TTS | 三阶段并发流水线（A:LLM→B:TTS→C:Opus），TTS 预合成下一句 |
| 新 ASR 被旧 LLM 阻断 | processSpeech goroutine 无取消机制 | listen.end 时先 cancel 旧流水线 |
| TTS 400 "No readable text" | LLM 输出含纯 markdown 符号/emoji 的句子 | TTS 单句失败 continue 跳过，记录 `tts skip sentence` 日志，不中止流水线 |
| Thinking 模式 `\n` 触发 TTS 空串 | `\n` 是断句符，thinking 结束后残留在 normalBuf | flushNormal 送出前 TrimSpace |
| 双连接重复播报（服务端重启场景） | 旧连接未被驱逐 | `deviceRegistry`：新连接到来时驱逐同 device_id 的旧 session |
| device_id 未传到服务端 | `CreateHelloMessage` 未将 device_id 写入 JSON | `message_handler.cpp`：hello JSON 加 `device_id` 字段 |

**阶段一交付物**：
- `picoclaw` binary（零功能改动，仅 bug fix）
- `jarvis-voice` binary（xiaozhi 协议语音网关，自托管 AgentLoop）✅
- `config.yaml` 配置文件
- 语音对话完整跑通 ✅，全链路 ASR+LLM+TTS 端到端验证通过

### Sprint 1.6 — 真正流式 ASR（事后追加）✅

**背景**：MVP 阶段 ASR 是在 `listen end`（VAD stop）后一次性把整段 PCM 发给服务端，造成识别延迟与服务端重复片段日志。重构为 `listen start` 时立即建 WebSocket 连接、音频帧逐帧推送。

**服务端 (picoclaw)**：

- `pkg/asr/provider.go` 扩展为四层接口：
  ```go
  type Provider interface { ... }                          // 已有
  type ResultCallback func(text string, final bool)         // 新增：增量回调
  type StreamingProvider interface { TranscribeStream() }   // 新增：批量PCM+增量回调
  type StreamingSession interface {                         // 新增：实时会话
      SendAudio(pcm []int16, isLast bool) error
      Wait(ctx) (string, error)
      Close() error
  }
  type RealtimeProvider interface { OpenSession() }        // 新增：VA start 建连
  ```
- `pkg/asr/doubao/provider.go`：
  - 提取 `connect()` 消除 `transcribeInternal` 和 `OpenSession` 的重复初始化代码
  - `lastText` 去重：相同片段不重复回调/日志
  - `partial`/`final` 分级日志
- `cmd/jarvis-voice/handler.go` 全面重构：
  - `listen start` → 立即 `OpenSession`，启动 `processStreamSpeech` goroutine
  - `handleAudio` → 有 `asrFeedCh` 时直接推帧（实时模式），否则缓冲（批量 fallback）
  - `listen end` → 实时模式只发结束帧；批量 fallback 走原有 `processSpeech`
  - `runPipeline` 提取：LLM→TTS→Opus 三阶段流水线，两条路共用
  - 客户端收到 `stt{state:"recognizing"}` 增量结果、`stt{state:"stop"}` 最终结果

**客户端 (JarvisCore，`refactor/v4-provider-redesign` 分支)**：

重构为 `DialogueFSM + IDialoguePipe` 架构（commit `da975de`）：

| 删除 | 替换为 |
|---|---|
| `AsrController/NlpController/TtsController` | `VoiceInput` / `TtsOutput` |
| `XiaozhiAsrProvider/NlpProvider/TtsProvider`（假拆分） | `XiaozhiDialoguePipe`（真管道） |
| `SherpaOnnxVadProvider` | `MmVadProvider`（mm-vad：NS/AEC/AGC+Silero） |
| 散落在各处的状态判断 | `DialogueFSM`（IDLE/LISTENING/PROCESSING/SPEAKING/INTERRUPTED） |

关键修复：barge-in 后 `INTERRUPTED → LISTENING` 现在正确生成 `turn_id` 并调用 `start_listening`。

---

## 阶段二：消息渠道接入

**目标**：飞书 + Telegram 双向文字，实现输出路由（短 → 语音，长 → 消息）。
**工期估算：3 天**

### Sprint 2.1 — 渠道配置与验证（1 天）

27. 飞书自建应用申请，获取 App ID / Secret / Verification Token
28. `~/.picoclaw/config.json` 配置 `channels.feishu`
29. Telegram bot 同步配置（开发调试用）
30. 验证：飞书发消息 → AI 回复文字

### Sprint 2.2 — 输出路由逻辑（2 天）

31. `cmd/jarvis-voice/handler.go` 新增 `routeOutput()` 方法：
    - 回复字数 ≤ 50 字 → TTS 语音
    - 回复字数 > 50 字 → 语音播短摘要 + picoclaw MessageTool 发飞书详情
    - 含 URL / 代码 / 表格 → 直接发消息，不走语音
32. agent system prompt 加入路由提示，输出 `[route:voice]` / `[route:message]` 标记
33. 验证：语音问"帮我解释量化交易" → 语音说摘要 + 飞书收完整回复

**阶段二交付物**：语音 + 飞书消息双通道打通，长内容自动路由到消息。

---

## 阶段三：信息订阅

**目标**：主动推送，把 Jarvis 从"问答机器人"变成"信息助理"。
**工期估算：1.5 周**

### Sprint 3.1 — 工具层（3 天）

34. 开启 picoclaw 内置工具：
    - `tools.web.duckduckgo.enabled = true`（免费，无需 API key）
    - `tools.web.brave` 可选（更准确，需 API key）
35. 接入 akshare MCP server（Python，提供 A 股 / 港股 / 美股数据）：
    ```json
    "mcp": {"servers": [{"name": "akshare", "command": "python", "args": ["-m", "akshare_mcp"]}]}
    ```
36. 接入 RSS MCP server（新闻源订阅）
37. 验证：语音问"今天 A 股大盘怎样" → AI 调 akshare 工具回答

### Sprint 3.2 — 订阅与晨报（4 天）

38. `workspace/skills/stock-monitor/` 新增股票监控 skill：
    - 关注列表存 `workspace/memory/watchlist.json`
    - 语音指令 "帮我关注 XX 股票" → 写入关注列表
    - CronTool 每日 9:30 检查涨跌 + 重大公告，达阈值推飞书
39. `workspace/skills/morning-brief/` 新增晨报 skill：
    - CronTool `0 7 * * *` 触发
    - 抓取：天气 / 热点新闻 3 条 / 关注股票涨跌
    - 输出：语音播 30 秒摘要 + 飞书发完整晨报
40. 热点订阅：用户指定关键词，每日推送 3 条摘要到飞书

**阶段三交付物**：晨报自动送达，股票异动主动告警，关键词热点订阅。

---

## 阶段四：个人效率工具

**目标**：把语音 + 消息的组合用到极致，提升日常效率。
**工期估算：1 周**

41. **提醒系统**：语音 "明天9点提醒我开会" → CronTool → 飞书通知
42. **阅读助手**：飞书发链接 → AI 抓取正文并总结 → 回复 "3句话摘要 + 2个关键观点"
43. **语音速记**：语音 "帮我记一下..." → AI 结构化整理 → 飞书归档（支持分类：想法/任务/备忘）
44. **价格监控**：发网页链接 + 目标价格 → CronTool 定时检查 → 达到时飞书通知
45. **投研简报**：每个关注股票每日合并公告 + 新闻 + 评级，生成一句话简报

**阶段四交付物**：提醒 / 速记 / 阅读助手 / 价格监控全功能可用。

---

## 阶段五：Docker 部署

**目标**：`docker-compose up` 一键在 Linux 服务器启动全套服务。
**工期估算：3 天**

46. 多阶段 Dockerfile（jarvis-voice + picoclaw）：
    - builder stage：`CGO_ENABLED=0` 纯 Go 静态编译（jarvis-voice 含 Opus CGo，需 gcc）
    - runtime stage：distroless 极简镜像
47. `docker-compose.yml`：ollama + jarvis-voice + picoclaw 三服务，共享 workspace volume
48. 环境变量支持：`OLLAMA_BASE_URL`、`JARVIS_VOICE_PORT`、`FEISHU_APP_ID` 等
49. 健康检查 endpoint（`/healthz`）+ 自动重启（`restart: unless-stopped`）
50. 部署文档：含 GPU 直通配置（`nvidia-container-toolkit`）

**阶段五交付物**：`docker-compose up -d` 一键部署，脱离开发机独立运行。

---

## 阶段六：All-in-One DLL 封装

**目标**：消除所有独立进程，picoclaw 编译为 `jarvis_ai.dll` 直接嵌入 UE5 插件。最终形态：UE 启动即可与 Jarvis 对话，零外部服务依赖，与 `libonelive.dll`、`mm_vad.dll` 同等地位。
**工期估算：2 周**

### Sprint 6.1 — 可行性验证（3 天）

51. 验证 Go `buildmode=c-shared` 与 UE5 的兼容性：
    - 构建最小 `hello.dll`，从 UE5 C++ 代码加载并调用，确认 Go 运行时初始化无冲突
    - 验证 Go GC 在 DLL 中的行为（DLL 不卸载时 GC 正常，UE 全生命周期持有即可）
    - CGo 工具链选型：MinGW-w64（MSYS2）或 LLVM/Clang，固化进构建脚本
52. 设计最小 C API（`include/jarvis_ai.h`）：
    ```c
    // 全异步回调模型，不阻塞 UE 游戏线程
    typedef void (*JarvisTokenCb)(const char* token, int is_final, void* user_data);
    typedef void (*JarvisAudioCb)(const int16_t* pcm, int samples, void* user_data);

    int  jarvis_init(const char* config_json);
    void jarvis_shutdown();
    int  jarvis_chat(const char* session_id, const char* text,
                     JarvisTokenCb on_token, JarvisAudioCb on_audio, void* user_data);
    void jarvis_abort(const char* session_id);
    ```
    - `on_audio` 直接回调 PCM，省去 Opus 编解码往返开销
53. `cmd/jarvis-ai-dll/` 入口，`go build -buildmode=c-shared -o jarvis_ai.dll`，验证 .dll + .h 正常生成

### Sprint 6.2 — llama.cpp 嵌入（5 天）

54. 集成 go-llama（llama.cpp Go CGo 绑定），替换 HTTP LLM 调用：
    - `pkg/providers/llamacpp/` 新 provider，实现 `ChatStream()` via CGo
    - 模型路径、GPU layers（默认 `-ngl 99` 全量 offload）、上下文长度从 config 读取
55. 下载并验证模型文件：
    - `models/qwen3-4b-q4_k_m.gguf`（~2.3GB）
    - RTX 4090 CUDA 推理速度验证，记录 TPS 基准数据
56. llama.cpp context 管理：单实例，多 session 共享，context 满时 rolling window 截断

### Sprint 6.3 — UE5 集成（4 天）

57. `OneJarvis.Build.cs` 仿 libonelive 方式加载 `jarvis_ai.dll`，`ThirdParty/JarvisAI/` 目录结构
58. `UOneJarvisSubsystem` 新增 DLL 直调模式（`EDllMode`）：
    - 跳过 WebSocket server，直接调 `jarvis_chat()`
    - `JarvisTokenCb` 回调 → 触发 `OnLLMToken` Delegate（切回 GameThread）
    - `JarvisAudioCb` 回调 → 直接推入 `StreamAudioPlayerCore`，无 Opus 解码
59. 打包验证：UE Editor 加载，开口说话，全链路在单进程内跑通
60. 性能对比记录：`dll 直调` vs `WebSocket + Ollama` 端到端首字延迟，记录结果到文档

**阶段六交付物**：`jarvis_ai.dll` 随 OneJarvis 插件分发，UE 项目零外部依赖启动完整 AI 对话。

---

## 关键接口定义汇总

### 流式 LLM

```go
// pkg/providers/types.go
type StreamingProvider interface {
    LLMProvider
    ChatStream(ctx context.Context, messages []Message, tools []ToolDefinition,
        model string, options map[string]any, onToken StreamCallback) error
}
type StreamCallback func(StreamToken)
type StreamToken struct {
    Type     string        // "token" | "tool_call" | "done" | "error"
    Content  string
    ToolCall *ToolCallDelta
}
```

### 音频格式约定

> 全局固定：**16kHz 16-bit mono PCM**。ASR 输入、TTS 输出、Opus 编解码均以此为准。所有接口不再传递 sampleRate，消除格式不一致风险。

### ASR（扩展为四层接口，Sprint 1.6 后）

```go
// pkg/asr/provider.go
type Provider interface {
    Name() string
    Transcribe(ctx context.Context, pcm []int16) (string, error)
}

type ResultCallback func(text string, final bool)

type StreamingProvider interface {
    Provider
    TranscribeStream(ctx context.Context, pcm []int16, cb ResultCallback) error
}

var ErrSessionClosed = errors.New("asr: session closed")

type StreamingSession interface {
    SendAudio(pcm []int16, isLast bool) error
    Wait(ctx context.Context) (string, error)
    Close() error
}

// RealtimeProvider：listen start 时建连，音频帧实时推送
type RealtimeProvider interface {
    Provider
    OpenSession(ctx context.Context, cb ResultCallback) (StreamingSession, error)
}
```

### TTS

```go
// pkg/tts/provider.go
type Provider interface {
    Name() string
    SynthesizeStream(ctx context.Context, text, voice string,
        onChunk func(pcm []int16)) error
}
```

---

## 工期汇总

| 阶段 | 内容 | 估算工期 | 状态 |
|---|---|---|---|
| 一 | MVP：语音对话跑通 | 2.5 周 | ✅ 完成 |
| 二 | 消息渠道接入（飞书/Telegram + 输出路由） | 3 天 | 下一步 |
| 三 | 信息订阅（股票监控 / 新闻 / 晨报） | 1.5 周 | 待做 |
| 四 | 个人效率（提醒 / 速记 / 阅读助手 / 价格监控） | 1 周 | 待做 |
| 五 | Docker 部署（中期生产环境） | 3 天 | 待做 |
| 六 | All-in-One DLL 封装（jarvis_ai.dll + go-llama）| 2 周 | 待做 |
| **合计（剩余）** | | **约 5.5 周** | |

---

## 待确认/优化事项

- [x] xiaozhi 协议消息格式 — 已对照 JarvisCore 源码确认并实现
- [x] JarvisCore 重连退避策略 — 线性 1~10s，已提交 `3b1a6bf`
- [ ] **Ollama 宿主机 IP 固定**：WSL2 每次重启后 `10.255.255.254` 是否稳定？若不稳定考虑写入 `/etc/hosts` 或用 `host.docker.internal`
- [ ] **飞书渠道权限**：个人版飞书 webhook 是否支持机器人主动推送，需实测权限范围
- [ ] **语音 + 消息共享记忆**：确保飞书和语音端使用同一 `memory_id`（device_id），上下文连贯
- [ ] **输出路由边界**：50 字分界是否合理，或改为由 agent 输出 `[route:xxx]` 标记来控制
- [ ] **闲聊路由云端选型**：DeepSeek V3 vs 通义 qwen-plus，确定后填入 config.json `cloud` model_list；当前保持纯本地，任务稳定后再引入
- [ ] **akshare MCP**：Python 依赖较重，考虑用 Go 实现轻量版或独立 Docker 容器隔离
- [ ] **TLS 支持**：内网开发可暂用 HTTP，服务器上线后需 WSS/HTTPS
- [ ] JarvisCore `refactor/v4-provider-redesign` 分支何时合入 master？合入前需联调验证 barge-in + AEC 路径
- [ ] `deps/` 目录（libhv/opus 共 38MB）是否纳入 git 管理，或改用 CMake FetchContent
- [ ] **go-llama 绑定选型**：`go-llama.cpp` vs `llama-go` vs 自写 CGo wrapper，确认维护活跃度后固化
- [ ] **Go DLL CGo 工具链**：MinGW-w64（MSYS2）vs LLVM/Clang，确认与 UE5 MSVC 编译环境无冲突
- [ ] **llama-server 分发方式**：纳入项目 `ThirdParty/` 还是文档引导手动下载；确定后写启动脚本（替换 Ollama）
- [ ] **jarvis_ai.dll API 版本管理**：C API 一旦固化，UE 侧与 Go 侧需同步版本号，考虑加 `jarvis_api_version()` 接口防止版本漂移
