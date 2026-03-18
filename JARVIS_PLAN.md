# Jarvis — 个人 AI 管家

## 产品定位

基于 UE5 数字人（MetaHuman + OneLive 音频表情）+ picoclaw AI 引擎，构建面向个人的 AI 管家产品。

Jarvis 面向个人用户，但底层 picoclaw 以服务端形态运行，必须按多用户 / 多工作区边界建模；个人体验通过租户内的 owner 归并实现，而不是把整个服务实例当成单用户进程。

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

**输出路由逻辑**（工具维度，不依赖响应长度/内容判断）：
```
工具调用完成后，按工具所属分组决定输出目标：
  channel-tool（股票/新闻/晨报/提醒/阅读助手...）
      语音触发 → TTS 播提示语（"已发送到频道"）+ 频道发完整结果
      频道触发 → 频道回复完整结果
  voice-tool（天气/时间/简单查询...）
      语音触发 → TTS 直接播报
      频道触发 → 频道回复
  无工具调用（纯对话）
      哪个频道进来就从哪个频道回复
```

---

## 大模型方案

### 当前及所有阶段：云端 OpenAI 兼容 API

不引入本地 LLM，所有阶段统一使用云端 API。

**当前配置**：
- Provider：chatanywhere（OpenAI 兼容代理）
- API Base：`https://api.chatanywhere.tech/v1`
- 默认模型：`gpt-4o-mini`（工具调用稳定，性价比高）
- 备用模型：`gpt-4o`（更复杂的分析任务）

**切换备用方案**（只改 config.json，零代码改动）：
- DeepSeek V3 / R1（国内直连，低延迟，极低价格）
- 通义 qwen-plus（有免费额度）
- OpenAI 官方 API

### 取消本地 LLM 路线

原计划的 Ollama → llama-server → go-llama CGo 迁移路线全部取消。

| 取消项 | 原因 |
|---|---|
| Ollama | picoclaw 作为服务器组件，云端 API 网络直达，本地推理无意义 |
| llama-server | 同上 |
| go-llama CGo 嵌入 DLL | AllInOne DLL 定位为轻量语音客户端，不嵌入推理引擎 |

### 多模型路由（后期备忘，当前不实现）

语音频道与消息频道若需不同模型（延迟 vs 质量），使用 picoclaw per-agent 配置，handler 一行判断，无需架构改动。候选：语音 → gpt-4o-mini，消息分析 → DeepSeek-R1。

---

## 消息频道选型

picoclaw 原生支持 17 个频道，无需额外开发。选型原则：国内可用 + 免费 + API 完整。

| 频道 | 需代理 | 个人注册 | 文件/图片 | 结论 |
|---|---|---|---|---|
| **飞书** | 否 | 免费个人版 | 支持 | 生产首选 |
| **Telegram** | 是 | 免费 | 支持 | 开发调试首选 |
| 企微 | 否 | 需企业认证 | 支持 | — |
| OneBot/QQ | 否 | 需第三方客户端 | 支持 | 有封号风险，不推荐 |

**结论**：飞书（生产）+ Telegram（开发）双配置并行，picoclaw 切换零成本。

飞书配置：创建自建应用 → 获取 App ID/Secret → 填入 `channels.feishu`。

---

## 服务部署方案

### 当前开发环境（阶段一~四：WSL）

所有服务运行在同一台 Windows 开发机的 WSL2 (ubuntu2404) 中，通过 Docker Compose 管理：

```
Windows 主机
└── WSL2 (ubuntu2404)
    └── Docker Compose（docker/docker-compose.jarvis.yml）
        ├── picoclaw          :18800  picoclaw gateway + Web UI（sipeed/picoclaw:launcher）
        └── picoclaw-voice    :8765   xiaozhi WebSocket 语音网关（自建镜像，doubao ASR/TTS）
            └── 进程内直接托管 picoclaw AgentLoop（Go 库调用）

共享 Volume：/home/kkroid/.picoclaw → /root/.picoclaw（config.json / workspace / 记忆）
JarvisCore（UE5，Windows）→ WSL picoclaw-voice :8765（局域网直连）
LLM：gpt-4o-mini（chatanywhere，HTTPS 出公网）
```

**启动命令**：
```bash
cd ~/github/picoclaw
docker compose -f docker/docker-compose.jarvis.yml up -d
```

**消息频道（飞书/Telegram）此阶段不配置**：webhook 需要公网 IP，迁移公有云后开启。

### 生产环境（公有云，阶段五起）

```
公有云 Linux 服务器（有公网 IP）
├── picoclaw-voice 容器   :18790  语音网关（支持远程连接）
├── picoclaw 容器          :18800  AI 引擎 + Web UI
│   ├── 消息频道：飞书 + Telegram（webhook 公网可达）
│   └── skills/  Python 脚本（服务器固定 conda 环境）
└── nginx 容器             TLS 终止，反向代理，域名证书

JarvisCore（任意位置）→ 公网域名:18790（TLS）
LLM：gpt-4o-mini（HTTPS，服务器直连）
```

### 终态（AllInOne 瘦客户端，阶段六）

```
用户设备（任意位置，有 UE5）
└── UE5 进程（OneLiveUE）
    └── OneJarvis Plugin：
        ├── mm_vad.dll          VAD / NS / AEC / AGC（本地）
        ├── libonelive.dll      面部表情音频驱动（本地）
        └── jarvis_client.dll   Go c-shared，纯 WebSocket 语音客户端
            ├── xiaozhi 协议 WebSocket（连接服务器）
            └── Opus 编解码

公有云 / 家庭 NAS（服务器常驻）
├── picoclaw-voice  语音网关
└── picoclaw        AI 引擎 + skills + 消息频道
```

`jarvis_client.dll` 极度轻量：只做本地音频处理 + WebSocket 通信，不内嵌任何 AI 推理，与 `libonelive.dll`、`mm_vad.dll` 同等地位。

---

## 仓库信息

- Fork 地址：`git@github.com:kkroid/picoclaw.git`
- 工作分支：`feature/jarvis`
- 正式分叉基线：`350f5a2`（已 push 的 PR 快照，适合作为长期 fork 起点）
- 当前工作树：中途重构态，只作为演进中的开发分支，不作为正式 fork 起点

---

## 当前进度（2026-03-12）

| 阶段 | 状态 | 备注 |
|---|---|---|
| 一：MVP 语音对话 | ✅ 完成 | `de8755c` (picoclaw) |
| 一 Sprint 1.6：真正流式 ASR | ✅ 完成 | `de8755c` (picoclaw) |
| JarvisCore 重构 v4 + 重连退避 | ✅ 完成 | `da975de` / `3b1a6bf` |
| 切换云端 LLM（gpt-4o-mini） | ✅ 完成 | config.json，不再使用 Ollama |
| stock-announcement skill | ✅ 完成 | `~/.picoclaw/workspace/skills/` |
| 工程化：目录重命名 + Docker 部署 | ✅ 完成 | jarvis-voice→picoclaw-voice，docker-compose.jarvis.yml |
| 二：消息频道接入（飞书 / Telegram） | ⏸ 待公有云 | webhook 需公网 IP |
| 三：信息订阅（股票 / 新闻 / 晨报） | 下一步 | WSL 阶段继续扩充 skills |
| 四：个人效率（提醒 / 速记 / 阅读助手） | 待做 | — |
| 五：Docker 部署（公有云迁移） | 待做 | — |

**下一步**：阶段三 skill 开发——晨报 / stock-monitor / read-article，在 WSL 验证，待迁移公有云后开启消息频道。

---

## 当前结论（2026-03-18）

### fork 结论

- 长期 fork 可行，而且值得做；Jarvis 的产品目标已经明显超出上游通用 bot 的定位。
- 正式 fork 起点应使用已 push 的 `350f5a2`，该版本基本以新增文件为主，侵入小，后续维护成本最低。
- 当前工作树是语音网关并入 channel / gateway / config 的中途重构态，不适合作为正式分叉起点。

### 服务端边界

- Jarvis 客户端可以是个人助手，但 picoclaw 服务端不是“全局单用户进程”。
- 推荐把 `workspace` 作为租户隔离边界；不同最终用户的长期记忆、skills、`USER.md`、`IDENTITY.md`、`SOUL.md` 不应混放在同一个 workspace。
- `owner_id` 只能在租户内使用，不能作为整个服务实例的全局唯一用户标识。

### 推荐领域模型

```text
tenant/workspace
    └── owner
            ├── platform identity
            ├── device
            ├── session
            └── turn / connection
```

- `owner`：长期画像、长期记忆、订阅、偏好的归属主体。
- `platform identity`：各渠道外部身份，如 `telegram:123`、`feishu:ou_xxx`；通过 canonical id + `identity_links` 归并到 owner。
- `device`：终端能力、在线状态、音频链路属性；不是长期记忆主键。
- `session`：一段对话线程；文本私聊可按 owner 归并，语音建议按 owner + device 保持短期上下文隔离。
- `turn / connection`：纯运行时 tracing 和链路控制字段，不进入长期记忆主键。

### 记忆与会话结论

- 长期记忆主键应为 `tenant_id + owner_id`，而不是 `device_id`。
- 会话记录主键应为 `session_key`；文本私聊优先复用上游 `dm_scope + identity_links`，语音则保留 `owner_id + device_id` 的短期会话隔离。
- `device_id` 用于设备注册、重连、旧连接驱逐；`conn_id`、`turn_id` 仅用于日志、超时和打断控制。
- 跨频道统一的是 owner 级长期记忆，不是把所有设备和所有连接强行压成一个实时 session。

### 上游可直接复用的能力

- 身份与路由：canonical sender id、`identity_links`、`dm_scope`。
- 记忆与压缩：JSONL session store、摘要压缩、强制压缩兜底。
- skills：workspace / global / builtin 多级加载、本地 skill 优先、按需读取 `SKILL.md`。
- workspace 分层：`USER.md`、`IDENTITY.md`、`SOUL.md`、`memory/`、`skills/`。

### 当前明确不再采用的做法

- 不再把 `device_id` 定义为持久记忆键。
- 不再把“全局 `owner_id`”理解为整个服务实例唯一用户。
- 不再把多设备实时对话强行并入同一个短期 session。

---

## 架构概览

```
[JarvisCore] UE5 C++ 数字人客户端（端侧 VAD，xiaozhi WebSocket 协议）
    ↕ WebSocket（xiaozhi 协议，:18790）
[picoclaw-voice] 语音网关（Go，cmd/picoclaw-voice/）
    ├── ASR 豆包云端流式 → 文字
    ├── TTS 豆包云端流式 → Opus 音频帧 → JarvisCore
    └── AgentLoop（进程内直接调用 picoclaw 库，无 HTTP 跳转）
    ↕ Go 库调用（picoclaw as library）
[picoclaw] AI 引擎（Go，picoclaw fork feature/jarvis）
    ├── workspace = tenant 边界（不同用户群体隔离）
    ├── 长期记忆（tenant_id + owner_id）
    ├── 会话记录（session_key；文本可 per-owner，语音建议 per-owner-device）
    ├── 工具：WebFetch / exec / CronTool / MCP
    ├── Skills：stock-announcement / ...（Python 脚本，conda 环境）
    └── AgentLoop（RunStreamAgentLoop）
    ↕ HTTPS
[云端 LLM] gpt-4o-mini（chatanywhere / OpenAI 兼容）

[picoclaw-web] 管理 UI（React，端口 18800）
    └── 可视化编辑 config.json（LLM 凭证 / 模型 / 频道 / 日志）

── 公有云迁移后新增 ──────────────────────────────
[picoclaw] 消息频道：飞书 + Telegram 双向（webhook 需公网 IP）
```

> **以上为当前（阶段一完成）架构**。AllInOne 目标（阶段六）是将 UE 侧收入 `jarvis_client.dll`（纯 WebSocket 客户端），picoclaw-voice + picoclaw 保持服务器常驻，与 `libonelive.dll`、`mm_vad.dll` 同等地位。

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

4. **`picoclaw-voice` 直接托管 AgentLoop**：通过公开 API（`pkg/config.LoadConfig` + `agent.NewAgentLoop`）在进程内初始化，完全消除对 picoclaw HTTP 端点的依赖，无需在 `helpers.go` 注册任何路由

### picoclaw-voice 独立 Go Module

`cmd/picoclaw-voice/` 是独立的 Go module（`github.com/sipeed/picoclaw/picoclaw-voice`），有自己的 `go.mod`：
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
cmd/picoclaw-voice/           ← 独立语音网关 module（新增）
    go.mod / go.sum
    main.go                   ← AgentLoop 初始化 + WebSocket server
    handler.go                ← session 生命周期 + ASR→TTS pipeline
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
│   ├── picoclaw/      (现有)
│   └── picoclaw-voice/  (新增，语音网关 binary)
├── pkg/
│   ├── asr/           (新增)
│   │   ├── provider.go    接口定义（四层：Provider/StreamingProvider/StreamingSession/RealtimeProvider）
│   │   └── doubao/        豆包实时 ASR WebSocket（已实现）
│   ├── tts/           (新增)
│   │   ├── provider.go    接口定义
│   │   └── doubao/        豆包 TTS WebSocket 流式（已实现）
│   ├── providers/     (修改：新增 StreamingProvider 接口)
│   │   └── openai_compat/ (修改：实现 ChatStream)
│   └── agent/         (修改：新增 stream.go)
└── web/               (已有；React + shadcn/ui)
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
     Body: { "session_id": "voice:kkroid:macbook", "text": "..." }
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

8. `pkg/asr/doubao/`：豆包实时 ASR WebSocket（已实现）
9. Provider 注册工厂：`asr.Register(name, factory)`，按 config 的 `asr.provider` 字段选择

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

10. `pkg/tts/doubao/`：豆包 TTS WebSocket 流式（已实现）

### Sprint 1.4 — picoclaw-voice 语音网关（3 天）✅

16. WebSocket 服务器，路径 `/xiaozhi/v1/`，完整实现 xiaozhi 消息协议：
    ```
    客户端 → 服务端:  hello | {type:"listen", state:"start"|"end", audio binary}
                      | {type:"abort"}
    服务端 → 客户端:  {type:"hello"} | {type:"tts", state:"start"|"sentence_start"|"sentence_end"|"stop"}
                      | {type:"llm", emotion, text} | audio binary (Opus 帧)
    ```

17. `hello` 消息中提取 `device-id`（MAC 地址），作为首版语音 session 标识；后续在服务端模型中仅用于设备维度，不再作为长期记忆主键；按 YAML 配置选 ASR/TTS/LLM provider
18. 接收二进制音频帧 → Opus 解码（`pion/opus`，固定 16kHz mono）→ PCM `[]int16`；端侧 VAD 已过滤，无需服务端 VAD
19. 收到 `listen.state="end"` 时：PCM → ASR → text → 直接调用 `agentLoop.RunStreamAgentLoop()`（进程内，无 HTTP 跳转）
20. `onToken` 回调内实时断句（`。？！！
.!?`） → 逐句触发 TTS
21. TTS 首帧到达 → 先推 `tts.state=sentence_start` → Opus 编码帧 → 推回 JarvisCore
22. abort 消息：`context.CancelFunc` 取消 ASR/LLM/TTS 全链路 goroutine，清空状态

### Sprint 1.5 — MVP 验收（1 天）✅

23. `cmd/picoclaw-voice/.env.example` 配置样例文件 ✅
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
- `picoclaw-voice` binary（xiaozhi 协议语音网关，自托管 AgentLoop）✅
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
- `cmd/picoclaw-voice/handler.go` 全面重构：
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

## 阶段二：消息频道接入

> **前置条件：服务迁移至公有云后执行。** 飞书/Telegram webhook 需要公网 IP，WSL 开发环境无法接收外部 webhook 推送。

**目标**：飞书 + Telegram 双向文字，实现输出路由（短 → 语音，长 → 消息）。
**工期估算：3 天**

### Sprint 2.1 — 频道配置与验证（1 天）

27. 飞书自建应用申请，获取 App ID / Secret / Verification Token
28. `~/.picoclaw/config.json` 配置 `channels.feishu`
29. Telegram bot 同步配置（开发调试用）
30. **跨频道身份与记忆统一**：在单个 workspace 内配置 `owner_id` 默认值 + `identity_links`；文本私聊按 owner 归并 session，语音保留 `owner_id + device_id` 的短期 session，并将可沉淀的长期信息回写到 owner 级 memory，确保语音和消息频道共享长期记忆但不污染实时上下文
31. 验证：频道发消息 → AI 回复（含历史记忆）

### Sprint 2.2 — 工具维度输出路由（2 天）

32. `cmd/picoclaw-voice/handler.go` 维护两个工具列表，在 tool_call 完成后路由（不影响流式 TTS）：
    ```go
    // 结果发频道；语音触发时 TTS 播提示语
    var channelTools = map[string]string{
        "akshare":          "数据已发送到频道",
        "stock_monitor":    "监控结果已发送到频道",
        "rss_fetch":        "新闻已发送到频道",
        "morning_brief":    "晨报已发送到频道",
        "web_fetch":        "内容已发送到频道",
        "schedule_reminder":"提醒已设置，到时频道通知",
        "read_article":     "总结已发送到频道",
    }
    // 结果直接 TTS 播报；频道触发时频道回复
    var voiceTools = []string{"get_weather", "get_time", "calculator"}
    // 不在列表 → 来源频道回复（语音→TTS，消息→消息）
    ```
33. 验证：语音问"今天天气" → TTS 直接播报（voice-tool）
34. 验证：语音问"苹果股价" → TTS 播"数据已发送到频道" + 频道收完整数据（channel-tool）
35. 验证：频道发"总结 https://..." → 频道收总结（不触发 TTS）

**阶段二交付物**：双频道打通，owner 级长期记忆统一，工具维度输出路由可用。

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
    - 关注列表存 `workspace/memory/owners/<owner_id>/watchlist.json`
    - 语音指令 "帮我关注 XX 股票" → 写入关注列表
    - CronTool 每日 9:30 检查涨跌 + 重大公告，达阈值推飞书
39. `workspace/skills/morning-brief/` 新增晨报 skill：
    - CronTool `0 7 * * *` 触发
    - 抓取：天气 / 热点新闻 3 条 / 关注股票涨跌
    - **主动推送只走频道**（CronTool 触发时无活跃语音 session）；当天首次开口时 picoclaw-voice 检查该 owner 的待播队列，播报 30 秒摘要
40. 热点订阅：用户指定关键词，按 owner 维度存储，每日推送 3 条摘要到飞书

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

## 本地知识库（待决策，不排期）

**目标**：为 Jarvis 增加私有知识问答能力，语音/消息均可触发检索，返回基于本地文档的精准回答。

**当前不决策原因**：阶段一~四的核心功能尚未完成；知识库方向取决于实际使用场景（技术文档 vs 私人笔记 vs 企业内部文档）。

### 方案 A — Qdrant MCP（推荐，契合架构）

picoclaw 原生支持 MCP，接入 `mcp-server-qdrant` 无需任何代码改动：

```json
"mcp": {
  "servers": {
    "knowledge": {
      "type": "stdio",
      "command": "uvx",
      "args": ["mcp-server-qdrant"],
      "env": {
        "QDRANT_URL": "http://localhost:6333",
        "OPENAI_API_KEY": "sk-...",
        "COLLECTION_NAME": "jarvis-kb"
      }
    }
  }
}
```

- Qdrant 本地 Docker 运行，向量存储不上云
- Embedding 复用已有 gpt-4o-mini key（OpenAI text-embedding-3-small）
- AI 自动决策何时调 `qdrant_find` 工具，无需手动触发
- 适合：技术文档、代码注释、研究报告等结构化知识

### 方案 B — chromadb skill（最简单，完全离线）

一个 exec skill，无需 MCP，无网络依赖：

```
skills/knowledge-base/
  scripts/
    ingest.py   ← 导入文档（PDF/TXT/MD），sentence-transformers 本地 embedding
    query.py    ← 向量检索，返回相关片段
  SKILL.md
```

- `sentence-transformers` 本地 embedding 模型（~300MB，首次下载后离线可用）
- chromadb 持久化到本地目录，文档不出本机
- 适合：私人笔记、公司内部文档、不希望内容经过任何 API 的场景

### 方案 C — AnythingLLM（开箱即用 GUI）

独立服务，提供 Web UI 管理文档，通过 REST API 或 MCP 接入 picoclaw：

- 支持 PDF/Word/网页/Notion 等多种格式导入
- 内置向量数据库，有可视化管理界面
- 适合：文档量大、需要非技术人员维护知识库、多人共享的场景

### 选型建议（决策时参考）

| 场景 | 推荐方案 |
|---|---|
| 技术文档、代码知识库 | 方案 A（Qdrant MCP，与架构最一致） |
| 私人笔记、不希望内容上云 | 方案 B（chromadb skill，完全离线） |
| 大量异构文档、需要 GUI 管理 | 方案 C（AnythingLLM） |

公有云迁移后，方案 B 的向量库保留本地，通过 MCP 远程访问（参考"本地私有 skill"章节），三个方案均可与云端架构兼容。

---

## 阶段五：Docker 部署

**目标**：`docker-compose up` 一键在 Linux 服务器启动全套服务。
**工期估算：3 天**

46. 多阶段 Dockerfile（picoclaw-voice + picoclaw）：
    - builder stage：`CGO_ENABLED=0` 纯 Go 静态编译（picoclaw-voice 含 Opus CGo，需 gcc）
    - runtime stage：distroless 极简镜像
47. `docker-compose.yml`：picoclaw-voice + picoclaw 两服务，共享 workspace volume
48. 环境变量支持：`API_BASE_URL`、`API_KEY`、`PICOCLAW_VOICE_PORT`、`FEISHU_APP_ID` 等
49. 健康检查 endpoint（`/healthz`）+ 自动重启（`restart: unless-stopped`）
50. 部署文档：含 nginx TLS 配置、域名证书（Let's Encrypt）

**阶段五交付物**：`docker-compose up -d` 一键部署，脱离开发机独立运行。

---

## 阶段六：AllInOne 瘦客户端 DLL

**目标**：消除 UE 对独立进程的依赖，将 UE 侧收入 `jarvis_client.dll`（纯 WebSocket 语音客户端），picoclaw-voice + picoclaw 保持服务器常驻。最终形态：UE 启动即连接 Jarvis 服务器，与 `libonelive.dll`、`mm_vad.dll` 同等地位。
**工期估算：1 周**（不嵌入 LLM，只做 WebSocket 客户端）

### Sprint 6.1 — C API 设计与 DLL 构建（2 天）

51. 验证 Go `buildmode=c-shared` 与 UE5 的兼容性：
    - 构建最小 `hello.dll`，从 UE5 C++ 代码加载并调用，确认 Go 运行时初始化无冲突
    - CGo 工具链选型：MinGW-w64（MSYS2）或 LLVM/Clang，固化进构建脚本
52. 设计最小 C API（`include/jarvis_client.h`）：
    ```c
    // 全异步回调，不阻塞 UE 游戏线程
    typedef void (*JarvisAudioCb)(const int16_t* pcm, int samples, void* user_data);
    typedef void (*JarvisStateCb)(const char* state, void* user_data);

    int  jarvis_connect(const char* server_url, const char* device_id);
    void jarvis_disconnect();
    void jarvis_send_audio(const int16_t* pcm, int samples, int is_last);
    void jarvis_abort();
    void jarvis_set_callbacks(JarvisAudioCb on_audio, JarvisStateCb on_state, void* user_data);
    ```
53. `cmd/jarvis-client-dll/` 入口，`go build -buildmode=c-shared -o jarvis_client.dll`，验证生成

### Sprint 6.2 — UE5 集成（3 天）

54. `OneJarvis.Build.cs` 仿 libonelive 方式加载 `jarvis_client.dll`，`ThirdParty/JarvisClient/` 目录
55. `UOneJarvisSubsystem` 新增 DLL 模式（`EClientMode`）：
    - 跳过直连 WebSocket server，调用 `jarvis_connect(server_url, device_id)`
    - `JarvisAudioCb` 回调 → 直接推入 `StreamAudioPlayerCore`，无 Opus 解码
56. 打包验证：UE Editor 加载，开口说话，全链路客户端-服务器架构跑通

**阶段六交付物**：`jarvis_client.dll` 随 OneJarvis 插件分发，UE 项目只需配置服务器地址，无本地 AI 依赖。

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

## 工期汇总

| 阶段 | 内容 | 估算工期 | 状态 |
|---|---|---|---|
| 一 | MVP：语音对话跑通 | 2.5 周 | ✅ 完成 |
| 二 | 消息频道接入（飞书/Telegram + 输出路由） | 3 天 | ⏸ 待公有云 |
| 三 | 信息订阅（股票监控 / 新闻 / 晨报） | 1.5 周 | 下一步 |
| 四 | 个人效率（提醒 / 速记 / 阅读助手 / 价格监控） | 1 周 | 待做 |
| 五 | Docker 部署（公有云迁移） | 3 天 | 待做 |
| 六 | AllInOne 瘦客户端 DLL（jarvis_client.dll） | 1 周 | 待做 |
| **合计（剩余）** | | **约 5 周** | |

---

## 待确认/优化事项

- [x] xiaozhi 协议消息格式 — 已对照 JarvisCore 源码确认并实现
- [x] JarvisCore 重连退避策略 — 线性 1~10s，已提交 `3b1a6bf`
- [x] 本地 LLM 方案 — 取消，统一使用云端 API（gpt-4o-mini via chatanywhere）
- [x] **服务端边界**：Jarvis 是个人客户端，picoclaw 是多用户服务端；推荐以 workspace 作为租户边界
- [ ] **公有云迁移时机**：阶段三~四 skill 开发完成后迁移，开启消息频道开发
- [ ] **飞书频道权限**：个人版飞书 webhook 是否支持机器人主动推送，需公有云后实测
- [ ] **跨频道记忆**：在 workspace 内按 `owner_id + identity_links` 统一长期记忆；语音短期 session 保留 `owner_id + device_id`
- [ ] **多模型路由（后期）**：语音 → gpt-4o-mini（延迟优先），消息分析 → DeepSeek-R1（质量优先）
