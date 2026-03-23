# xiaozhi 协议（picoclaw 修改版，完整说明）

本文档描述 PicoClaw 当前内置 `pkg/channels/xiaozhi` 通道实际实现的 WebSocket 协议，而不是上游 `xiaozhi-esp32-server` 的原样复刻。它是一个“兼容 xiaozhi v3 子集 + PicoClaw 扩展字段/消息”的文本 + 语音双模态协议。

当前主线运行形态以 picoclaw 内置 channel 为准，协议路径固定为 `/xiaozhi/v1/`。

## 1. 协议范围

这个实现解决的是如下链路：

- 文本输入 → LLM → 文本输出
- 语音输入 → ASR → LLM → 文本输出
- 语音输入 → ASR → LLM → 文本输出 + 可选 TTS 输出

当前默认语义：

- 文本输入：只输出文本，不触发 TTS。
- 语音输入：始终输出文本；当回复适合口语播报时，会额外下发语音。
- 略长但仍适合口语的回复，服务端会自动裁一版简短播报摘要用于 TTS，同时保留完整文本输出。
- 结构化结果、清单、代码块、链接、工具结果：默认只保留文本承载，不继续整段播报。
- 工具结果默认留在 xiaozhi 自身文本面板，不再回退到“最近活跃外部文本渠道”。

## 2. 与上游 xiaozhi 的主要差异

与上游协议相比，PicoClaw 当前增加或改变了这些语义：

- 新增客户端文本输入消息：`text`
- 新增服务端推理通知消息：`llm`
- 扩展 `hello.owner_id`：显式设备 owner 绑定
- 扩展 `listen.memory_id` / `text.memory_id`：显式 session key 覆盖
- 扩展 `hello.session_id`：连接级 ID，由服务端生成
- 扩展 `hello.owner_id`：服务端解析出的有效 owner
- 语音输出不再默认总是发生，而是由服务端根据回复内容决定
- owner / device / session 的解析规则由 PicoClaw 控制，不再只依赖 `device_id`

当前未实现或保留但未启用的内容：

- OTA / 配网类扩展
- `ping` / `pong` 心跳往返，当前实现不会回复 `pong`
- `listen.mode` 当前保留但未参与服务端逻辑
- 协议层未定义独立客户端鉴权字段；`channels.xiaozhi.token` 是服务端配置，不是线上的协议字段

## 3. 连接与传输

### 3.1 连接端点

WebSocket 端点：`ws://<host>:<gateway_port>/xiaozhi/v1/`

xiaozhi 作为 picoclaw 内置 channel 挂载在主进程共享 HTTP/WebSocket server 上，复用 `gateway.host:gateway.port`，不需要单独再开一个 xiaozhi 专用端口。

这意味着：

- `channels.xiaozhi.enabled=true` 时，xiaozhi 随主进程一起初始化和启动
- 若同一实例还启用了其他 HTTP/WebSocket channel，它们和 xiaozhi 共用同一个 gateway 端口，仅通过 path 区分
- 像 `maixcam` 这种原生 TCP channel 仍保留独立端口

### 3.2 传输编码

- 文本消息：JSON 文本帧
- 音频消息：二进制帧，内容格式由协商结果决定

关键点：

- 客户端 → 服务端的二进制帧，必须匹配服务端在 `hello.asr_params` 中声明的格式
- 服务端 → 客户端的二进制帧，必须按 `hello.tts_params` 初始化播放器/解封装器/解码器
- 服务端不会在协议层做“偷偷兼容”的隐式转码；格式协商一旦完成，后续帧必须严格遵守协商结果

### 3.3 默认 provider 组合下的协商结果

以下是当前常见 provider 组合的默认音频格式：

| provider | 方向 | 默认格式 |
|------|------|------|
| `doubao` ASR | 上行（客户端 → 服务端） | `ogg / opus / 16000 Hz / mono / frame_duration=60` |
| `funasr` ASR | 上行（客户端 → 服务端） | `pcm / raw / 16000 Hz / mono / frame_duration=60` |
| `doubao` TTS | 下行（服务端 → 客户端） | `ogg / opus / 16000 Hz / mono / frame_duration=60` |
| `fishspeech` TTS | 下行（服务端 → 客户端） | `pcm / raw / 44100 Hz / mono / frame_duration=60` |

若 ASR/TTS provider 改变，最终仍以服务端返回的 `asr_params` / `tts_params` 为准。
若服务端当前无法为某个 ASR provider 提供可直接透传的上行格式，握手会被直接拒绝并打印日志，不会先答应再在 ASR 阶段失败。

## 4. 服务端配置中会影响协议行为的字段

这些配置不会直接出现在线上协议里，但会影响服务端返回的字段和会话行为：

```json
{
  "channels": {
    "xiaozhi": {
      "enabled": true,
      "default_owner_id": "kkroid",
      "session_scope": "per-owner-device",
      "appid": "YOUR_DOUBAO_APP_ID",
      "token": "YOUR_DOUBAO_ACCESS_TOKEN",
      "asr_provider": "doubao",
      "asr_cluster": "bigmodel_transcribe",
      "asr_resource_id": "YOUR_DOUBAO_ASR_RESOURCE_ID",
      "tts_provider": "doubao",
      "tts_cluster": "volcano_tts",
      "tts_voice": "zh_female_wanwanxiaohe_moon_bigtts"
    }
  }
}
```

关键配置语义：

- `default_owner_id`：匿名语音设备的默认 owner
- `owner_id`：`default_owner_id` 的兼容别名，优先级更低
- `session_scope`：短期上下文隔离粒度，支持 `per-owner-device` 和 `per-owner`
- `appid` / `token`：ASR/TTS 共用凭证兜底
- `asr_*` / `tts_*`：分别覆盖 ASR/TTS 专用 provider 配置
- `asr_resource_id`：按控制台实际开通资源填写。ASR 1.0 小时版用 `volc.bigasr.sauc.duration`，ASR 2.0 小时版用 `volc.seedasr.sauc.duration`

## 5. 协议对象模型

当前实现里有三类 ID，语义必须区分：

- 连接级 `hello.session_id`
  - 由服务端在握手时生成
  - 每次 WebSocket 重连都会变化
  - 主要用于日志和连接关联

- 轮次级 `listen.session_id` / `text.session_id`
  - 由客户端为每轮问答生成
  - PicoClaw 内部把它当作当前 turn 的 ID

- 短期上下文 key `session_key`
  - 不直接在线上传输
  - 由服务端根据 owner / device / `memory_id` 推导
  - 决定 AgentLoop 的短期对话上下文归属

## 6. 客户端 → 服务端消息

### 6.1 `hello` 握手

连接建立后，客户端必须先发送 `hello`。在收到服务端 `hello` 之前，不应发送文本 turn 或音频 turn。

示例：

```json
{
  "type": "hello",
  "version": 3,
  "transport": "websocket",
  "device_id": "aa:bb:cc:dd:ee:ff",
  "owner_id": "kkroid"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | 是 | 固定为 `"hello"` |
| `version` | int | 是 | 协议版本，当前实现为 `3` |
| `transport` | string | 是 | 固定为 `"websocket"` |
| `device_id` | string | 否 | 设备唯一标识；建议稳定且可重复使用 |
| `owner_id` | string | 否 | 显式设备 owner 绑定；首次绑定后会持久化到设备状态文件 |

说明：

- 当前实现中，客户端 `hello` 不再携带上行音频能力参数
- 上行格式完全以服务端返回的 `hello.asr_params` 为准，属于服务端单向声明
- 最终协商结果以服务端返回的 `hello.asr_params` 为准
- 若服务端无法提供可直接透传的上行格式，会在握手阶段直接拒绝连接并打印日志
- 若客户端继续发送与 `asr_params` 不一致或明显非法的音频帧，服务端会直接拒绝连接并打印日志，不做隐式转码
- 当 `asr_params.format=ogg` 且 `asr_params.codec=opus` 时，客户端必须直接发送 Ogg Opus 字节流；picoclaw 不会在中间层做解码、重编码或容器转换
- 同一 `device_id` 的新连接到来时，旧连接会被立即踢掉，采用 last-write-wins 语义

### 6.2 `text` 文本输入

示例：

```json
{
  "type": "text",
  "text": "你好，今天有什么安排？",
  "session_id": "turn-uuid",
  "memory_id": "custom-session-key"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | 是 | 固定为 `"text"` |
| `text` | string | 是 | 本轮文本输入内容 |
| `session_id` | string | 否 | 本轮 turn ID；留空时服务端自动生成 |
| `memory_id` | string | 否 | 显式 session key 覆盖值 |

实现语义：

- 文本 turn 只走 LLM 文本输出，不触发 TTS
- 文本 turn 结束时，服务端一定会下发 `{"type":"llm","state":"stop"}` 作为收尾信号
- 文本 turn 与语音 turn 共享同一套 owner / device / session 解析规则

### 6.3 `listen` 语音轮次控制

示例：

```json
{
  "type": "listen",
  "state": "start",
  "session_id": "turn-uuid",
  "memory_id": "custom-session-key"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | 是 | 固定为 `"listen"` |
| `state` | string | 是 | `"start"` / `"end"` / `"stop"` |
| `mode` | string | 否 | 保留字段，当前实现未使用 |
| `session_id` | string | 否 | 本轮 turn ID；建议客户端生成 UUID |
| `memory_id` | string | 否 | 显式 session key 覆盖值 |

当前实现中的 `state` 语义：

- `start`
  - 开始一轮新的语音输入
  - 服务端会准备 turn 上下文，并在实时 ASR provider 下立即打开识别会话

- `end`
  - 表示客户端结束送音
  - 服务端会把当前缓冲音频作为本轮语音输入送入 ASR → LLM → TTS 流程

- `stop`
  - 当前实现中与 `end` 等价，也会结束送音并推动后续处理
  - 它不是“硬取消当前轮次”的语义
  - 如果客户端要强制中断当前轮次，请使用 `abort`

也就是说，当前实现中真正的“打断”动作不是 `listen.stop`，而是 `abort`。

### 6.4 `abort` 打断当前轮次

示例：

```json
{
  "type": "abort",
  "reason": "wake_word_detected"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | 是 | 固定为 `"abort"` |
| `reason` | string | 否 | 预留原因字段，当前服务端不做分支处理 |

实现语义：

- 关闭当前 ASR session
- 取消当前 LLM / TTS pipeline
- 服务端立即下发 `{"type":"tts","state":"abort"}`
- 客户端应把 `tts.abort` 视为当前轮次被打断后的 authoritative end signal，不应再等待 `tts.stop` 或 `llm.stop`

### 6.5 二进制帧：上行音频

`listen.start` 之后，客户端持续发送二进制音频帧，直到 `listen.end` 或 `listen.stop`。

格式要求：

- 必须匹配服务端 `hello.asr_params`
- 默认 `doubao` ASR 下，上行格式为 `ogg / opus / 16000 Hz / mono`
- `funasr` 等 PCM ASR 下，上行格式为 `pcm / raw / 16000 Hz / mono`

当前实现的容错规则：

- 空音频帧会被直接丢弃
- 当协商格式为 `pcm/raw` 时，非 16-bit 对齐帧会被直接拒绝，服务端打印日志并关闭连接
- 当协商格式为 `ogg/opus` 时，服务端按字节流透传，不会在中间层解包或解码
- 若服务端当前无法为某个 ASR provider 提供可直接透传的上行格式，握手会被直接拒绝

### 6.6 `ping`

协议注释中保留了 `ping`，但当前实现不会回复 `pong`，也没有心跳状态机逻辑。客户端不要依赖 `ping/pong` 来判断连通性。

## 7. 服务端 → 客户端消息

### 7.1 `hello` 握手响应

示例：

```json
{
  "type": "hello",
  "version": 3,
  "transport": "websocket",
  "session_id": "conn-uuid",
  "owner_id": "kkroid",
  "asr_params": {
    "format": "ogg",
    "codec": "opus",
    "sample_rate": 16000,
    "channels": 1,
    "frame_duration": 60
  },
  "tts_params": {
    "format": "ogg",
    "codec": "opus",
    "sample_rate": 16000,
    "channels": 1,
    "frame_duration": 60
  }
}
```

| 字段 | 说明 |
|------|------|
| `session_id` | 连接级 ID，由服务端在握手时生成 |
| `owner_id` | 当前连接解析出的有效 owner |
| `asr_params` | 服务端要求客户端使用的上行音频格式 |
| `tts_params` | 服务端将要下发的下行音频格式 |

说明：

- `hello.session_id` 是连接级 ID，不是 turn ID
- 当前默认 `doubao + doubao` 组合下，`asr_params` 是 `ogg / opus / 16000 / mono / frame_duration=60`，`tts_params` 也是 `ogg / opus / 16000 / mono / frame_duration=60`
- 当 provider 改变时，`tts_params` 也可能变成 `pcm / raw`，例如 `fishspeech`

### 7.2 `stt` 识别结果

示例：

```json
{ "type": "stt", "text": "你好", "state": "recognizing" }
{ "type": "stt", "text": "你好世界。", "state": "stop" }
```

| 字段 | 说明 |
|------|------|
| `text` | 当前 ASR 文本 |
| `state=recognizing` | 流式中间结果 |
| `state=stop` | 最终识别结果 |

说明：

- 在实时 ASR provider 下，客户端可能收到多次 `recognizing`
- 在批量 ASR provider 下，客户端可能只收到一次最终 `stop`

### 7.3 `llm` 推理通知

`llm` 是 PicoClaw 扩展消息，不属于上游 xiaozhi 标准协议。

当前有四种形态：

```json
{ "type": "llm", "text": "你好！" }
{ "type": "llm", "state": "thinking_start" }
{ "type": "llm", "state": "thinking_end", "duration_ms": 532 }
{ "type": "llm", "state": "stop" }
```

| 形态 | 说明 |
|------|------|
| `text` | LLM 已经成句的文本片段 |
| `state=thinking_start` | 检测到 `<think>`，进入 thinking 阶段 |
| `state=thinking_end` | 检测到 `</think>`，thinking 结束，携带耗时 |
| `state=stop` | 当前文本输出轮次结束 |

补充说明：

- 当前实现没有 `emotion` 字段，客户端不要依赖它
- `llm.text` 是按句子边界切分后的文本片段，不是 token 级流
- `llm.stop` 表示本轮文本流已经结束；即使后面还会继续下发 TTS，它也可能先到
- 语音 turn 里真正的音频播放收尾仍以 `tts.stop` 或 `tts.abort` 为准

### 7.4 `tts` 播放状态机

示例：

```json
{ "type": "tts", "state": "start" }
{ "type": "tts", "state": "sentence_start", "text": "你好！有什么可以帮你的？" }
{ "type": "tts", "state": "sentence_end" }
{ "type": "tts", "state": "stop" }
{ "type": "tts", "state": "abort" }
```

| `state` | 说明 |
|------|------|
| `start` | 本轮 TTS 播放开始 |
| `sentence_start` | 当前句开始；`text` 为本句内容 |
| `sentence_end` | 当前句结束 |
| `stop` | 本轮 TTS 播放结束 |
| `abort` | 本轮被显式打断 |

实现语义：

- `tts` 只在本轮决定输出语音，或者收到显式 `abort` 时才出现
- 文本 turn 默认不会出现 `tts.start/stop`
- 语音 turn 如果被判定为“只输出文本”，当前实现仍可能补发一个兼容 `tts.stop` 作为轮次收尾
- 某些错误或空识别场景下，服务端可能仅发送 `tts.stop` 作为收尾

### 7.5 二进制帧：下行音频

服务端音频二进制帧总是出现在 `tts.sentence_start` 和 `tts.sentence_end` 之间。

格式要求：

- 由 `hello.tts_params` 决定
- `doubao` TTS：服务端直接透传豆包返回的 Ogg Opus 字节流，不会在中间层解包、解码或重编码
- `fishspeech` TTS：直接下发原始 PCM chunk
- WebSocket 二进制消息边界只表示服务端当前收到的一段音频字节块；客户端必须按顺序拼接并按 `tts_params` 解析，不能假设每个消息都恰好等于一个裸 Opus packet 或一个完整 Ogg page

客户端必须按 `tts_params` 初始化播放器/解封装器/解码器，不能把下行格式写死为裸 Opus。

## 8. 完整时序

### 8.1 文本 turn

```text
Client                              Server
  |                                   |
  | WS connect                        |
  |---------------------------------->| 
  | hello                             |
  |---------------------------------->| 
  |                                   | hello
  |<----------------------------------|
  | text                              |
  |---------------------------------->| 
  |                                   | llm(text)
  |<----------------------------------|
  |                                   | llm(text)
  |<----------------------------------|
  |                                   | llm(stop)
  |<----------------------------------|
```

### 8.2 语音 turn，最终包含 TTS

```text
Client                              Server
  |                                   |
  | listen(start)                     |
  |---------------------------------->| 
  | [binary audio frames]             |
  |---------------------------------->| 
  | listen(end)                       |
  |---------------------------------->| 
  |                                   | stt(recognizing...)
  |<----------------------------------|
  |                                   | stt(stop)
  |<----------------------------------|
  |                                   | llm(text)
  |<----------------------------------|
  |                                   | llm(stop)
  |<----------------------------------|
  |                                   | tts(start)
  |<----------------------------------|
  |                                   | tts(sentence_start)
  |<----------------------------------|
  |                                   | [binary audio frames]
  |<----------------------------------|
  |                                   | tts(sentence_end)
  |<----------------------------------|
  |                                   | tts(stop)
  |<----------------------------------|
```

### 8.3 语音 turn，最终只输出文本

```text
Client                              Server
  |                                   |
  | listen(start/end) + 音频           |
  |---------------------------------->| 
  |                                   | stt(stop)
  |<----------------------------------|
  |                                   | llm(text)
  |<----------------------------------|
  |                                   | llm(text)
  |<----------------------------------|
  |                                   | llm(stop)
  |<----------------------------------|
  |                                   | tts(stop)  (compat)
  |<----------------------------------|
```

### 8.4 显式打断

```text
Client                              Server
  |                                   |
  | abort                             |
  |---------------------------------->| 
  |                                   | tts(abort)
  |<----------------------------------|
```

## 9. 语音输出选择规则

当前是否下发 TTS，不由客户端决定，而由服务端根据 LLM 最终回复内容决定。

当前规则：

- 文本输入：固定只输出文本
- 语音输入：固定输出文本；是否额外播报语音取决于内容是否适合口语播报
- 对于略长但不结构化的回答，服务端会优先选取前 1 到 2 句、压缩成较短播报摘要后再做 TTS
- 对于编号清单或 markdown 列表这类“可转述的结构化回答”，服务端会尽量提炼成 1 到 2 句口语摘要后播报
- 以下内容会被判定为只输出文本：
  - 代码块
  - 表格
  - 链接
  - 工具结果

补充说明：

- 如果 owner 当天有待播摘要，服务端可能把待播摘要作为本轮 TTS 的第一句先播，再播本轮回复
- 这种待播摘要是服务端行为，不需要额外协议字段

## 10. owner / device / session 解析规则

### 10.1 有效 owner 解析优先级

服务端为当前连接解析 `owner_id` 的优先级是：

1. `hello.owner_id` 显式绑定
2. 已持久化设备绑定 `workspace/state/devices/<device_id>.json`
3. `channels.xiaozhi.default_owner_id`

解析后的 `owner_id` 会在服务端 `hello.owner_id` 中回给客户端。

### 10.2 短期 `session_key` 推导规则

服务端使用以下优先级推导短期上下文 key：

1. `listen.memory_id` 或 `text.memory_id`
2. 若存在 owner，则按 `session_scope` 生成
3. 若无 owner 但有 `device_id`，回退到设备级 key
4. 若连 `device_id` 都没有，回退到连接级 key

具体规则：

- `session_scope = per-owner-device`
  - 有 owner 且有 device：`xiaozhi:owner:<owner>:device:<device>`
  - 只有 owner：`xiaozhi:owner:<owner>`

- `session_scope = per-owner`
  - 有 owner：`xiaozhi:owner:<owner>`

- 若没有 owner
  - 有 device：`xiaozhi:device:<device>`
  - 否则：`xiaozhi:conn:<conn_id>`

owner 级长期记忆 key 固定为：

- `xiaozhi:owner:<owner>`

### 10.3 持久化路径

当前实现会在 workspace 内维护这些状态文件：

- `workspace/state/devices/<device_id>.json`
  - 保存 `device_id -> owner_id` 绑定

- `workspace/memory/owners/<owner_id>/profile.json`
  - 保存 owner 的已绑定设备、最近语音活动

- `workspace/memory/owners/<owner_id>/voice_context.json`
  - 保存 owner 级语音摘要与最近对话快照

- `workspace/memory/queues/<owner_id>/voice_pending.json`
  - 保存待播语音摘要队列

## 11. 音频规格建议

虽然最终仍以协商结果为准，但当前默认 `doubao` 组合下推荐客户端按如下参数实现：

| 参数 | 默认值 |
|------|------|
| 上行容器/编码 | Ogg / Opus |
| 下行容器/编码 | Ogg / Opus |
| 采样率 | 16000 Hz |
| 声道 | 1 |
| 帧时长 | 60 ms |
| 应用模式 | 上行按 `asr_params` 采集/编码/封装，下行按 `tts_params` 解封装/解码/播放 |

客户端必须始终以服务端返回的 `asr_params` / `tts_params` 为准，不能把上行或下行格式写死。

## 12. 客户端实现建议

如果你要按当前 PicoClaw 实现来写客户端，建议遵循下面的行为：

1. 建连后立即发送 `hello`
2. 收到服务端 `hello` 后，用 `asr_params` 初始化采集/编码/封装链路，用 `tts_params` 初始化播放器/解封装器/解码器
3. 文本输入使用 `text`
4. 语音输入使用 `listen.start` → 二进制音频帧 → `listen.end`
5. 不要依赖 `listen.stop` 做硬取消；需要中断时发 `abort`
6. 当收到 `llm.stop`、`tts.stop` 或 `tts.abort` 时，把本轮视为结束
7. 不要依赖 `ping/pong`
8. 不要依赖不存在的 `emotion` 字段

## 13. 与上游仓库的关系

本模块实现了与 [xiaozhi-esp32-server](https://github.com/xinnan-tech/xiaozhi-esp32-server) 兼容的 v3 协议子集，并把后端替换为 PicoClaw 的 AgentLoop、ASR provider、TTS provider 和 owner / memory 体系。

如果客户端目标是“接 PicoClaw 内置 xiaozhi”，请以本文档为准；不要默认以上游 `xiaozhi-esp32-server` 的线下行为作为权威实现。
