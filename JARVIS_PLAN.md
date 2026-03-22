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

**输出路由逻辑**（工具维度，不依赖响应长度/内容判断）：
```
工具调用完成后，按工具所属分组决定输出目标：
  channel-tool（股票/新闻/晨报/提醒/阅读助手...）
      语音触发 → TTS 播提示语（"已发送到渠道"）+ 渠道发完整结果
      渠道触发 → 渠道回复完整结果
  voice-tool（天气/时间/简单查询...）
      语音触发 → TTS 直接播报
      渠道触发 → 渠道回复
  无工具调用（纯对话）
      哪个渠道进来就从哪个渠道回复
```

---

## 本地大模型选型

### 当前阶段：Ollama + qwen3:4b，不引入任何路由

**功能实现阶段的唯一优先级是流程跑通**，模型好坏、延迟高低都是后话。先用现有 Ollama 跑完所有功能（工具调用、记忆、渠道、订阅），全部验收后再做模型优化。

**三个关键判断，全部不需要额外逻辑**：

| 问题 | 答案 | 依据 |
|---|---|---|
| 是否调用工具 | LLM 自己决定 | function calling 机制，qwen3:4b 原生支持 |
| 调用哪个工具 | LLM 自己决定 | 根据工具描述和意图匹配，4B 够用 |
| 走哪个通道 | 消息来源决定 | WebSocket 进来 → 语音；飞书 webhook 进来 → 消息，handler 层一行代码 |

**输出路由规则**（工具维度，在 AgentLoop tool_call 完成后执行，与流式 TTS 不冲突）：

维护两个工具列表（配置文件可覆盖）：
```go
// 结果发渠道，语音触发时播提示语
var channelTools = []string{
    "akshare", "stock_monitor", "rss_fetch", "morning_brief",
    "web_fetch", "schedule_reminder", "read_article",
}
// 结果直接 TTS 播报
var voiceTools = []string{
    "get_weather", "get_time", "calculator",
}
// 不在列表 → 走调用来源渠道（语音→TTS，消息→消息回复）
```

**与流式 TTS 的兼容**：工具调用发生在流水线的工具执行阶段（非 token streaming 阶段），tool result 注入后 LLM 再次流式输出最终答案，此时再做路由判断，不影响流式体验。

### 引擎迁移路线（功能完成后再做）

Ollama 是 llama.cpp 的包装层，All-in-One DLL 终态需要去掉这层包装：

| 阶段 | 方案 | 说明 |
|---|---|---|
| 当前（功能阶段） | Ollama | 现有，不动 |
| 阶段六前期 | llama-server | llama.cpp 内置服务器，OpenAI 兼容，只改 `api_base` 端口 |
| 阶段六后期（终态） | go-llama CGo | in-process，嵌入 `jarvis_ai.dll`，无独立进程 |

**模型**：`qwen3:4b`（Ollama）→ `Qwen3-4B-Q4_K_M.gguf`（llama 阶段），~2.3GB，RTX 4090 全量 offload，首字 ~150ms。

### 多模型路由（后期备忘，当前不实现）

当出现质量瓶颈时，按 channel 切分：语音 → 本地 4B（延迟优先），消息 → 云端（质量优先）。云端候选：DeepSeek V3（国内直连，极低价格），通义 qwen-plus（有免费额度）。切换时 picoclaw per-agent 配置，handler 一行判断，无需架构改动。

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

### 当前开发环境（阶段一~五）

```
Windows 主机
├── Ollama（GPU 推理，OLLAMA_HOST=0.0.0.0，模型 qwen3:4b）
└── WSL2 (ubuntu2404)
    └── picoclaw
        ├── gateway + xiaozhi channel   api_base: http://10.255.255.254:11434/v1
        └── channels.feishu / telegram  api_base: http://10.255.255.254:11434/v1

JarvisCore（UE5，Windows）→ WSL picoclaw `/xiaozhi/v1/`
```

> 阶段六前期替换为 llama-server（llama.cpp 内置，OpenAI 兼容），只改 `api_base` 端口，业务代码零改动。

### 中期生产环境（Docker，阶段五）

```
Linux 服务器（有 GPU）
├── ollama 容器        GPU 推理，挂载模型目录
├── picoclaw 容器
│   ├── gateway + xiaozhi channel
│   ├── 飞书 / Telegram / MCP / Cron
│   └── 连接 ollama
└── nginx 容器         TLS 终止，反向代理
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

## 当前进度（2026-03-20）

| 阶段 | 状态 | 说明 |
|---|---|---|
| 一：MVP 语音对话 | ✅ 完成 | xiaozhi 协议已在主进程内跑通 ASR → LLM → TTS |
| 一 Sprint 1.6：真正流式 ASR | ✅ 完成 | `listen.start` 建连，音频逐帧推送 |
| 一补充：xiaozhi 内嵌化 | ✅ 完成 | 独立语音网关已并入 `pkg/channels/xiaozhi` |
| JarvisCore 重构 v4 + 重连退避 | ✅ 完成 | `da975de` / `3b1a6bf` |
| 二：消息渠道接入（飞书 / Telegram 双向） | 下一步 | 重点是 owner 统一与路由策略验收 |
| 三：信息订阅（股票 / 新闻 / 晨报） | 待做 | — |
| 四：个人效率（提醒 / 速记 / 阅读助手） | 待做 | — |
| 五：Docker 部署 | 待做 | 单容器主进程承载 xiaozhi + 消息渠道 |

**下一步**：阶段二——配置飞书渠道，完成语音 owner 与消息渠道 owner 的统一验收

---

## 架构概览

```
[JarvisCore] UE5 C++ 数字人客户端（端侧 VAD，xiaozhi WebSocket 协议）
    ↕
[picoclaw gateway] 主进程（Go）
    ├── pkg/channels/xiaozhi
    │   ├── owner / device / session 解析
    │   ├── ASR provider（doubao / funasr）
    │   ├── AgentLoop.RunStreamAgentLoopWithKeys
    │   ├── TTS provider（doubao / fishspeech）
    │   └── voice_pending 首次开口摘要播报
    ├── 工具：DuckDuckGo 搜索 / WebFetch / CronTool / MCP / Message
    ├── 消息渠道：飞书 + Telegram 双向文字
    └── workspace memory / state / queues
    ↕
[Ollama] 本地大模型
    ├── qwen3:4b    语音对话（延迟优先）
    └── qwen3.5:27b 分析摘要（质量优先）

[picoclaw-web] 管理 UI（React，端口 18800）
    └── 可视化编辑 config.json（LLM 凭证 / 模型 / 渠道 / 日志）
```

> 当前主线已经没有独立 `jarvis-voice` 或 `picoclaw-voice` 进程，语音入口就是 picoclaw 主 gateway 内置的 xiaozhi channel。All-in-One 目标（阶段六）仍是将上述逻辑进一步收入 `jarvis_ai.dll`。

---

## PicoClaw 侵入性分析

PicoClaw 仍在快速迭代，但当前语音能力已经从“外挂语音网关”演进为“主进程内嵌 channel”。因此这里不再强调早期的 **"只增不改"**，而改为以下实际原则：

1. **能放子包就放子包**：xiaozhi 相关实现集中在 `pkg/channels/xiaozhi/`
2. **必须改核心时只改编排层**：仅在 `manager` / `gateway` / `config` / `agent` / `tools` 等接缝层打孔
3. **先把协议和状态模型做对，再谈兼容旧实现**
4. **所有关键链路都补测试**：尤其是 channel handler、provider 协议和 owner/session 状态逻辑

### 现有文件改动清单（最终实现）

| 分类 | 文件 | 原因 |
|---|---|---|
| 新增 | `pkg/channels/xiaozhi/*` | xiaozhi 协议、owner/device/session、文本/语音流水线 |
| 新增 | `pkg/memory/voice_pending.go` | 渠道结果转语音待播摘要 |
| 新增 | `pkg/agent/stream.go` / 测试 | 流式 AgentLoop 与 owner memory 双键 |
| 修改 | `cmd/picoclaw/internal/gateway/helpers.go` | 注册 xiaozhi、注入 AgentLoop、接入待播写入器 |
| 修改 | `pkg/channels/manager.go` | 初始化 xiaozhi channel |
| 修改 | `pkg/config/config.go` | 新增 `channels.xiaozhi` 与 `session.identity_links` |
| 修改 | `pkg/bus/bus.go` | 出站钩子机制（OnOutbound / ClearOutboundHooks），集中处理渠道输出镜像 |
| 修改 | `pkg/agent/loop.go` / `pkg/tools/message.go` | 语音待播与渠道输出接缝（pendingWriter 已移至 bus 钩子） |
| 修改 | ~~`pkg/devices/service.go`~~ / ~~`pkg/heartbeat/service.go`~~ / ~~`pkg/tools/cron.go`~~ | 已移除散射 pendingWriter 注入，全部收归 bus 出站钩子 |
| 移除 | `cmd/picoclaw-voice/*` / `docker/docker-compose.voice.yml` / `pkg/tts/ogg.go` | 废弃独立语音网关与旧解包路径 |

### 当前架构的核心判断

1. **语音入口必须是 channel，而不是旁路进程**  
    否则 owner / memory / tools / channels / workspace 状态都会重复实现一套。

2. **owner 才是跨渠道统一记忆主键，device 只是语音侧的连接附属物**

3. **音频格式必须显式化，不再依赖“默认就是 Opus/PCM”这种隐式约定**

4. **长内容转渠道、首次开口播摘要应该是统一的 owner 级行为，不应该散落在客户端或独立进程里**

### 当前改动边界

当前主线只保留一个 `picoclaw` 主程序：

- xiaozhi 语音能力在 `pkg/channels/xiaozhi/` 内聚实现
- gateway 只负责注册 channel、注入 AgentLoop 和共享服务
- 没有额外的独立语音 gateway module，也不再维护第二套独立启动脚本

### 新增文件清单

```
pkg/providers/
    streaming.go              ← StreamingProvider 接口（新增）
pkg/providers/openai_compat/
    streaming.go              ← ChatStream() 实现（新增，同包扩展 Provider struct）
pkg/agent/
    stream.go                 ← RunStreamAgentLoopWithKeys()（新增）
pkg/asr/                      ← ASR 接口 + doubao / funasr provider（新增）
pkg/tts/                      ← TTS 接口 + doubao / fishspeech provider（新增）
pkg/channels/xiaozhi/         ← 主进程内置语音 channel（新增）
pkg/memory/voice_pending.go   ← owner 级待播摘要队列（新增）
```

### 合并策略

- 上游更新时：`git merge upstream/main`
- 冲突风险：**极低**。现有文件只有 `http_provider.go` +11 行，其余全为新增文件，无冲突
- 若上游修复了同一 bug（`HTTPProvider.ChatStream`）：删除我们的改动即可，stream.go/streaming.go 不受影响
- 若上游重构了 `openai_compat.Provider` struct 名称：只需更新 `streaming.go` 中的 receiver 类型

### 与上游的分歧：StreamingProvider.ChatStream 回调语义

**背景**：2026-03-20 合并 upstream/main 时发现，上游在 `#1101`（Telegram stream LLM responses）中将 `StreamingProvider` 接口的回调从增量 delta 改为累积文本：

```go
// 上游：回调收到累积文本（为 Telegram editMessage 设计）
ChatStream(..., onChunk func(accumulated string))

// 我们的硬改：回调直接传递 SSE 原始 delta
ChatStream(..., onChunk func(delta string))
```

**原因**：大模型 SSE 原始输出就是增量 delta（`{"delta":{"content":"你"}}`），上游在 `parseStreamResponse` 中先用 `strings.Builder` 追加成累积文本再传给 `onChunk`，这对语音 TTS 场景（需要逐 token 实时合成）是不必要的弯路。

**我们的改动（共 3 处，均有 `[KKROID FORK]` 注释标记）**：

| 文件 | 改动 | 说明 |
|------|------|------|
| `pkg/providers/openai_compat/provider.go` | `onChunk(choice.Delta.Content)` 替代 `onChunk(textContent.String())` | 从源头传 delta |
| `pkg/agent/loop.go` | Telegram streamer 回调中自行用 `strings.Builder` 累积 | 消费者按需累积 |
| `pkg/agent/stream.go` | `onToken(delta)` 直接透传 | 语音场景无需转换 |

**合并上游时的注意事项**：如果上游修改了 `ChatStream` 或 `parseStreamResponse`，需要在合并后确认上述 3 处 `[KKROID FORK]` 标记的改动仍然存在。搜索关键字：`grep -rn "KKROID FORK" pkg/`。

**建议与上游讨论的方向**：
1. 回调改为传 delta，累积由调用方按需自己做（Telegram 侧自行维护 Builder）
2. 或回调同时传 delta 和 accumulated：`onChunk func(delta, accumulated string)`

---

## 目录结构

```
picoclaw/              (fork 根目录)
├── cmd/
│   ├── picoclaw/      (主 gateway / CLI)
│   └── picoclaw-launcher-tui/
├── pkg/
│   ├── agent/         (流式 agent loop + owner memory 双键)
│   ├── asr/           (doubao / funasr)
│   ├── channels/
│   │   └── xiaozhi/   (内置语音 channel)
│   ├── memory/        (voice_pending 等 owner 级状态)
│   ├── providers/     (StreamingProvider + openai_compat streaming)
│   └── tts/           (doubao / fishspeech)
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

### Sprint 1.4 — xiaozhi 内置语音通道（3 天）✅

16. WebSocket 服务器，路径 `/xiaozhi/v1/`，完整实现 xiaozhi 消息协议：
    ```
    客户端 → 服务端:  hello | {type:"listen", state:"start"|"end", audio binary}
                      | {type:"abort"}
    服务端 → 客户端:  {type:"hello"} | {type:"tts", state:"start"|"sentence_start"|"sentence_end"|"stop"}
                      | {type:"llm", emotion, text} | audio binary (Opus 帧)
    ```

17. `hello` 消息中提取 `device-id` / `owner_id`，按当前 owner / device / session 规则建立语音上下文
18. 接收二进制音频帧，严格按 `hello.asr_params` 声明的格式透传给 ASR provider；端侧 VAD 已过滤，无需服务端 VAD
19. 收到 `listen.state="end"` 时：音频 → ASR → text → 直接调用 `agentLoop.RunStreamAgentLoopWithKeys()`（进程内，无 HTTP 跳转）
20. `onToken` 回调内实时断句（`。？！！
.!?`） → 逐句触发 TTS
21. TTS 首帧到达 → 先推 `tts.state=sentence_start` → 按 `hello.tts_params` 约定的格式推回 JarvisCore
22. abort 消息：`context.CancelFunc` 取消 ASR/LLM/TTS 全链路 goroutine，清空状态

### Sprint 1.5 — MVP 验收（1 天）✅

23. xiaozhi 协议文档与配置样例已沉淀到主仓库配置与文档体系 ✅
24. 单元测试：`pkg/channels/xiaozhi/*_test.go` 覆盖 `lastSentenceBreak`、thinking 状态机、文本/语音输出策略、owner/session/pending 状态 ✅
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
- `picoclaw` binary（主 gateway + 内置 xiaozhi channel）✅
- `config.yaml` / `config.json` 配置文件
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
- `pkg/channels/xiaozhi/handler.go` 全面重构：
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
30. **跨渠道记忆统一**：picoclaw config 设置全局 `owner_id`（如 "kkroid"），所有渠道 agent 调用强制使用同一 session_id，语音端同步改为 owner_id（不再用 device_id），确保语音和消息渠道共享同一记忆
31. 验证：渠道发消息 → AI 回复（含历史记忆）

### Sprint 2.2 — 工具维度输出路由（2 天）

32. `pkg/channels/xiaozhi/handler.go` 维护两个工具列表，在 tool_call 完成后路由（不影响流式 TTS）：
    ```go
    // 结果发渠道；语音触发时 TTS 播提示语
    var channelTools = map[string]string{
        "akshare":          "数据已发送到渠道",
        "stock_monitor":    "监控结果已发送到渠道",
        "rss_fetch":        "新闻已发送到渠道",
        "morning_brief":    "晨报已发送到渠道",
        "web_fetch":        "内容已发送到渠道",
        "schedule_reminder":"提醒已设置，到时渠道通知",
        "read_article":     "总结已发送到渠道",
    }
    // 结果直接 TTS 播报；渠道触发时渠道回复
    var voiceTools = []string{"get_weather", "get_time", "calculator"}
    // 不在列表 → 来源渠道回复（语音→TTS，消息→消息）
    ```
33. 验证：语音问"今天天气" → TTS 直接播报（voice-tool）
34. 验证：语音问"苹果股价" → TTS 播"数据已发送到渠道" + 渠道收完整数据（channel-tool）
35. 验证：渠道发"总结 https://..." → 渠道收总结（不触发 TTS）

**阶段二交付物**：双渠道打通，跨渠道记忆统一，工具维度输出路由可用。

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
    - **主动推送只走渠道**（CronTool 触发时无活跃语音 session）；当天首次开口时 xiaozhi channel 检查待播队列，播报 30 秒摘要
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

46. 多阶段 Dockerfile（picoclaw 单服务）：
    - builder stage：主构建只编译 `picoclaw`
    - runtime stage：distroless 极简镜像
47. `docker-compose.yml`：ollama + picoclaw + nginx 三服务，共享 workspace volume
48. 环境变量支持：`OLLAMA_BASE_URL`、`PICOCLAW_CHANNELS_XIAOZHI_*`、`FEISHU_APP_ID` 等
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

> 当前不再使用“全局固定 PCM”这一旧约定，而是显式声明 `Format + Codec + SampleRate + Channels`，并以服务端 `hello.asr_params` / `hello.tts_params` 为准。

### ASR（扩展为四层接口，Sprint 1.6 后）

```go
// pkg/asr/provider.go
type Provider interface {
    Name() string
    Transcribe(ctx context.Context, frames [][]byte) (string, error)
}

type ResultCallback func(text string, final bool)

type StreamingProvider interface {
    Provider
    TranscribeStream(ctx context.Context, frames [][]byte, cb ResultCallback) error
}

var ErrSessionClosed = errors.New("asr: session closed")

type StreamingSession interface {
    SendAudio(frame []byte, isLast bool) error
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
    SynthesizeFrames(ctx context.Context, text, voice string,
        onFrame func(data []byte)) error
}
```

---

## 工期汇总

| 阶段 | 内容 | 估算工期 | 状态 |
|---|---|---|---|
| 一 | MVP：语音对话跑通 + xiaozhi 内嵌化 | 2.5 周 | ✅ 完成 |
| 二 | 消息渠道接入（飞书/Telegram + 输出路由） | 3 天 | 下一步 |
| 三 | 信息订阅（股票监控 / 新闻 / 晨报） | 1.5 周 | 待做 |
| 四 | 个人效率（提醒 / 速记 / 阅读助手 / 价格监控） | 1 周 | 待做 |
| 五 | Docker 部署（单主进程 + xiaozhi） | 3 天 | 待做 |
| 六 | All-in-One DLL 封装（jarvis_ai.dll + go-llama）| 2 周 | 待做 |
| **合计（剩余）** | | **约 5.5 周** | |

---

## 待确认/优化事项

- [x] xiaozhi 协议消息格式 — 已按当前主线实现并更新文档
- [x] JarvisCore 重连退避策略 — 线性 1~10s，已完成
- [x] 语音链路内嵌化 — 独立 `jarvis-voice` / `picoclaw-voice` 已淘汰
- [x] owner / device / session / 待播摘要模型 — 已落到 workspace 状态文件
- [ ] **飞书渠道权限**：个人版飞书 webhook 是否支持机器人主动推送，需实测
- [ ] **owner 映射验收**：`session.identity_links` 在真实语音 + 飞书 + Telegram 环境下继续压测
- [ ] **双通道阈值校准**：voice-tool / channel-tool 列表按真实体验补充
- [ ] **Docker 单服务部署**：补生产 compose 与部署文档
- [ ] **Ollama 宿主机 IP 固定**：WSL2 每次重启后 `10.255.255.254` 是否稳定；如不稳定则切 `llama-server` 或写入 hosts
- [ ] **多模型路由（后期）**：功能全部验收后再决定是否引入云端，候选 DeepSeek V3 / 通义 qwen-plus
