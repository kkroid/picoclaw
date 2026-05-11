package prepare

import (
	"strings"
	"time"

	appruns "github.com/sipeed/oneappfactory/pkg/appfactory/runs"
)

const (
	protocolSurfaceConnection    = "surface-connection"
	protocolSurfaceHome          = "surface-home"
	protocolSurfaceProjectDrawer = "surface-project-drawer"
	protocolSurfaceConversation  = "surface-conversation"
	protocolSurfaceFiles         = "surface-files"
	protocolSurfaceSettings      = "surface-settings"
)

func isProtocolClientRequirement(lower, raw string) bool {
	protocolSignals := []string{
		"/api/v1",
		"websocket",
		"web socket",
		"web_socket_channel",
		"rest endpoint",
		"rest api",
		"get /",
		"post /",
		"put /",
		"delete /",
		"ws://",
		"host 和 port",
		"host、port",
		"远端服务",
		"服务端协议",
		"局域网 http",
		"局域网连接",
	}
	for _, signal := range protocolSignals {
		if strings.Contains(lower, signal) || strings.Contains(raw, signal) {
			return true
		}
	}
	return false
}

func compileProtocolClientSpec(request Request, requirementText string) domainSpec {
	now := time.Now().UTC()
	if request.Now != nil {
		now = request.Now()
	}
	title := strings.TrimSpace(request.TitleHint)
	if title == "" {
		title = protocolClientTitle(requirementText)
	}
	jobID := fallbackGeneratedID(request.JobID, "job-protocol-client-mvp", request.RequirementSource, now)
	prdID := fallbackGeneratedID(request.PRDID, "prd-protocol-client-mvp", request.RequirementSource, now)
	templateID := strings.TrimSpace(request.TemplateID)
	preferredTemplateIDs := []string{"flutter-open-lite"}
	if templateID != "" {
		preferredTemplateIDs = []string{templateID}
	}
	realBuild := request.RealBuild || strings.TrimSpace(request.ExecutorImage) != ""
	executorImage := strings.TrimSpace(request.ExecutorImage)
	if realBuild && executorImage == "" {
		executorImage = "oneappfactory/builder:local"
	}
	flutterProfile := appruns.NewFlutterAndroidProfile()
	commandProfile := flutterProfile.CommandProfile
	if !realBuild {
		commandProfile = appruns.CommandProfile{ProfileName: "prepare-p0", AllowedStages: []string{"baseline", "cheap"}, AllowedCommands: []string{"echo", "grep"}, DeniedCommands: []string{"rm", "sudo"}, MaxSingleCommandSeconds: 60, MaxParallelCommands: 1, NetworkPolicy: "disabled", WritableRoots: []string{"lib", "assets", "."}, EnvAllowlist: []string{"PATH", "HOME"}}
	}
	entities := protocolClientEntities(requirementText)
	capabilities := protocolClientCapabilityFlags(requirementText)
	acceptanceCriteria := protocolClientAcceptanceCriteria()
	return domainSpec{
		Kind:                "protocol-client",
		Slug:                "protocol-client-mvp",
		Title:               title,
		TemplateID:          templateID,
		ExecutorImage:       executorImage,
		RequirementText:     requirementText,
		PRDID:               prdID,
		JobID:               jobID,
		SourceSummary:       sourceSummary(request.RequirementSource),
		Summary:             "基于输入协议生成一个 Flutter 协议客户端 MVP，通过局域网 REST 与 WebSocket 连接远端服务，完成连接配置、资源浏览、主交互和项目文件读写预览。",
		ProblemStatement:    "当前生成链路必须从协议输入中保留 endpoint、实时事件、连接配置和负向能力边界，不能再降级成离线 generic shell。",
		TargetUsers:         protocolClientUsers(),
		CoreScenarios:       protocolClientScenarios(),
		Goals:               protocolClientGoals(),
		NonGoals:            protocolClientNonGoals(),
		FeatureList:         protocolClientFeatures(),
		SurfaceList:         protocolClientSurfaces(),
		UserFlows:           protocolClientUserFlows(),
		DataEntities:        entities,
		ComplexityLevel:     "L4-protocol-client-contract",
		CapabilityFlags:     capabilities,
		BehaviorRules:       protocolClientBehaviorRules(),
		PersistenceContract: &PersistenceContract{Mode: "client-settings-shared-preferences", RepositoryPath: "lib/services/settings_store.dart", EntityRefs: []string{"entity-connection-settings"}, CapabilityRefs: []string{"connection-settings", "settings-store"}, Required: true},
		ProtocolContract:    protocolClientProtocolContract(),
		RealtimeContract:    protocolClientRealtimeContract(),
		RuntimeContract:     protocolClientRuntimeContract(),
		TemplateConstraints: TemplateConstraints{Stack: "flutter", AndroidRequired: true, RequiredCapabilities: []string{"navigation", "rest-api", "websocket-streaming", "connection-settings", "provider-state", "markdown-rendering", "settings-store"}, PreferredTemplateIDs: preferredTemplateIDs},
		AcceptanceCriteria:  acceptanceCriteria,
		ManualReviewPoints:  protocolClientManualReviewPoints(),
		KnownUnknowns:       []KnownUnknown{{Question: "iOS build 是否在本轮纳入强 gate", Impact: "medium", Owner: "product"}, {Question: "后续是否扩展扫码、文件搜索和项目生命周期管理", Impact: "medium", Owner: "product"}},
		TaskBundle:          protocolClientTaskBundle(),
		AcceptanceChecks:    protocolClientAcceptanceChecks(realBuild),
		GoalSummary:         "生成真实协议客户端 Flutter MVP：保留 REST endpoint、WebSocket 事件、连接配置、Provider 状态、Markdown 消息、文件浏览保存和 fake protocol 验收，不允许退回 generic record/local-hive。",
		TemplateFitReasons: []string{
			"当前可复用 Flutter seed 作为工程骨架，但协议客户端能力由 planning artifacts 和 deterministic emitter 补齐。",
			"协议客户端分支按 REST/WebSocket/endpoint 信号进入，不按 App 名称走专用模板。",
		},
		TemplateFitGaps: []string{
			"flutter-open-lite 原始 slot 是 generic record 语义，protocol-client 必须使用独立 slot map 和 task bundle 覆盖。",
			"真实服务端不作为默认验收前提，必须通过 fake REST/WS harness 验证主流程。",
		},
		ImplementationPhases: []string{
			"阶段 1：冻结协议实体、endpoint、WebSocket、连接设置和负向能力合同。",
			"阶段 2：生成模型、ApiClient、WsClient、settings store、Provider tree 和 app bootstrap。",
			"阶段 3：生成连接、主页、对话、文件和设置 surface，并接入 fake protocol widget tests。",
			"阶段 4：通过 analyze、test、release APK build 与 protocol semantic checks 收口。",
		},
		ManualConstraints: []string{
			"不得继承 generic local 的默认离线、业务 Hive 数据库、无远端 API、无实时通信约束。",
			"不得按 OnePilot 名称写专用生成分支；必须由 REST endpoint、WebSocket 和协议实体驱动。",
			"MVP 不生成项目新增/删除/启动/停止、文件搜索或扫码入口；文本文件保存仅覆盖当前文件内容，不做冲突检测。",
			"不要求真实服务端在线；使用 fake REST/WS harness 证明协议行为。",
		},
		HumanNotes:              []map[string]string{{"note_id": "note-protocol-client", "scope": "engineering", "summary": "该输入包用于生成协议客户端，而不是 generic open-lite 离线记录壳。"}, {"note_id": "note-negative-capabilities", "scope": "product", "summary": "MVP 明确排除扫码、项目生命周期管理和文件搜索；允许文本文件保存。"}},
		RequirementHighlights:   protocolClientRequirementHighlights(requirementText),
		SupportingAssumptions:   []string{"远端服务通过局域网 REST 与 WebSocket 暴露。", "本地仅保存连接设置、可选 token 和最近选择，不保存业务离线数据库。", "无真实服务端时 App 仍能进入连接配置或错误空状态。"},
		PreferredAllowedPaths:   protocolClientAllowedPaths(),
		PreferredProtectedPaths: protocolClientProtectedPaths(),
		KnowledgePack:           append([]appruns.ProfileSkill(nil), flutterProfile.KnowledgePack...),
		CommandProfile:          commandProfile,
		ContextFiles:            appruns.ContextFiles{PRDMarkdownPath: prdMarkdownFileName, PRDJSONPath: prdJSONFileName, TemplateFitReportPath: fitReportFileName, ImplementationPlanPath: planFileName, ManualConstraintsPath: constraintsFileName, SupportingFiles: []string{requirementFileName, prdApprovalFileName, templateApprovalFileName, planningContextFileName, domainModelFileName, templateSlotMapFileName, taskAllocationFileName, acceptancePlanFileName}},
	}
}

func protocolClientTitle(requirementText string) string {
	if strings.Contains(strings.ToLower(requirementText), "onepilot") {
		return "OnePilot 手机 App MVP"
	}
	return "协议客户端 App MVP"
}

func protocolClientEntities(requirementText string) []DataEntity {
	_ = requirementText
	return []DataEntity{
		{EntityID: "entity-project", Name: "Project", Source: "remote-rest", Fields: []DataField{{Name: "id", Type: "string", Role: "identifier", Required: true}, {Name: "name", Type: "string", Role: "primary_text", Required: true}, {Name: "workspace", Type: "string", Required: false}, {Name: "os", Type: "string", Required: false}, {Name: "status", Type: "enum[running,stopped,error]", Role: "status", Required: true}, {Name: "port", Type: "int", Required: false}, {Name: "pid", Type: "int", Required: false}, {Name: "last_active", Type: "datetime", Role: "date", Required: false}}},
		{EntityID: "entity-conversation", Name: "Conversation", Source: "remote-rest", Fields: []DataField{{Name: "id", Type: "string", Role: "identifier", Required: true}, {Name: "title", Type: "string", Role: "primary_text", Required: true}, {Name: "model", Type: "string", Required: false}, {Name: "mode", Type: "string", Required: false}, {Name: "source", Type: "enum[thread,task]", Required: true}, {Name: "status", Type: "enum[active,running,completed,failed,interrupted]", Role: "status", Required: true}, {Name: "created_at", Type: "datetime", Required: false}, {Name: "updated_at", Type: "datetime", Role: "date", Required: false}, {Name: "last_message_preview", Type: "string", Required: false}}},
		{EntityID: "entity-message", Name: "Message", Source: "remote-rest-websocket", Fields: []DataField{{Name: "role", Type: "enum[user,assistant,system]", Required: true}, {Name: "content", Type: "string", Role: "primary_text", Required: true}, {Name: "timestamp", Type: "datetime", Role: "date", Required: false}, {Name: "usage", Type: "object", Required: false}}},
		{EntityID: "entity-message-segment", Name: "MessageSegment", Source: "remote-websocket", Fields: []DataField{{Name: "kind", Type: "enum[agent_message,reasoning,tool_call,tool_result]", Role: "status", Required: true}, {Name: "content", Type: "string", Role: "primary_text", Required: true}, {Name: "turn_id", Type: "string", Required: false}}},
		{EntityID: "entity-file-node", Name: "FileNode", Source: "remote-rest", Fields: []DataField{{Name: "path", Type: "string", Role: "identifier", Required: true}, {Name: "name", Type: "string", Role: "primary_text", Required: true}, {Name: "type", Type: "enum[file,directory,image]", Role: "status", Required: true}, {Name: "size", Type: "int", Required: false}, {Name: "modified", Type: "datetime", Required: false}}},
		{EntityID: "entity-connection-settings", Name: "ConnectionSettings", Source: "local-shared-preferences", Fields: []DataField{{Name: "host", Type: "string", Required: true}, {Name: "port", Type: "int", Required: true}, {Name: "token", Type: "string", Required: false}, {Name: "last_project_id", Type: "string", Required: false}, {Name: "last_conversation_id", Type: "string", Required: false}, {Name: "theme_mode", Type: "string", Required: false}}},
	}
}

func protocolClientCapabilityFlags(requirementText string) []string {
	_ = requirementText
	return []string{"rest-api", "websocket-streaming", "connection-settings", "provider-state", "project-switching", "conversation-list", "conversation-create", "streaming-chat", "markdown-rendering", "reasoning-block", "tool-call-status", "emergency-stop", "file-browser", "read-only-file-preview", "file-edit-save", "settings-store", "fake-protocol-tests", "android-lan-http"}
}

func protocolClientUsers() []UserProfile {
	return []UserProfile{{UserID: "user-mobile-operator", Label: "移动端操作者", Summary: "希望在手机上连接局域网内的 PC 服务，查看项目、对话、AI 回复和文件。", PainPoints: []string{"离开电脑后无法快速查看 AI 任务状态", "需要在网络异常时知道如何重试"}}, {UserID: "user-validator", Label: "内部验证人员", Summary: "需要确认生成物是真实协议客户端，而不是可编译的 generic shell。", PainPoints: []string{"build 通过不能证明协议行为正确", "缺 endpoint 或负向能力错误很难靠肉眼发现"}}}
}

func protocolClientScenarios() []Scenario {
	return []Scenario{{ScenarioID: "scenario-connect", Title: "配置并验证 PC 连接", Summary: "用户输入 host、port 和可选 token，保存后通过 system info 验证连接。", PrimaryUserRefs: []string{"user-mobile-operator"}}, {ScenarioID: "scenario-project-conversations", Title: "浏览项目与对话", Summary: "用户查看项目列表、切换项目，并浏览 thread/task 混排对话列表。", PrimaryUserRefs: []string{"user-mobile-operator"}}, {ScenarioID: "scenario-streaming-chat", Title: "查看和发送流式对话", Summary: "用户进入对话详情，加载历史消息、订阅 WebSocket delta、发送消息并可急停。", PrimaryUserRefs: []string{"user-mobile-operator"}}, {ScenarioID: "scenario-files", Title: "浏览并保存项目文件", Summary: "用户浏览目录树、读取/保存文本文件并预览图片。", PrimaryUserRefs: []string{"user-mobile-operator"}}}
}

func protocolClientGoals() []string {
	return []string{"从 REST/WebSocket 协议输入生成结构化协议客户端 artifacts。", "生成可 analyze/test/build 的 Flutter MVP，不保留 counter demo 或 generic record 语义。", "通过 fake protocol harness 验证连接、项目、对话、流式事件、急停和文件浏览主流程。"}
}

func protocolClientNonGoals() []string {
	return []string{"不做二维码扫码或完整配对认证。", "不做项目新增、启动、停止、删除。", "不做文件搜索、冲突检测或二进制编辑。", "不做业务离线数据库、推送、分享、登录、支付。"}
}

func protocolClientFeatures() []Feature {
	return []Feature{{FeatureID: "feature-connection", Title: "连接配置与健康检查", Summary: "输入 host/port/token 并调用 system info 验证。", Priority: "p0", Required: true, RelatedSurfaceRefs: []string{protocolSurfaceConnection, protocolSurfaceSettings}, AcceptanceRefs: []string{"ac-connection"}}, {FeatureID: "feature-projects", Title: "项目列表与切换", Summary: "展示远端项目列表和状态，切换后重连 WebSocket。", Priority: "p0", Required: true, RelatedSurfaceRefs: []string{protocolSurfaceHome, protocolSurfaceProjectDrawer}, AcceptanceRefs: []string{"ac-projects"}}, {FeatureID: "feature-conversations", Title: "对话列表与创建", Summary: "拉取 thread/task 混排列表，支持创建普通 thread。", Priority: "p0", Required: true, RelatedSurfaceRefs: []string{protocolSurfaceConversation}, AcceptanceRefs: []string{"ac-conversations"}}, {FeatureID: "feature-streaming-chat", Title: "流式对话详情", Summary: "加载历史、订阅 delta、发送 turns、渲染 reasoning/tool_call 并支持急停。", Priority: "p0", Required: true, RelatedSurfaceRefs: []string{protocolSurfaceConversation}, AcceptanceRefs: []string{"ac-streaming-chat"}}, {FeatureID: "feature-files", Title: "文件浏览与文本保存", Summary: "浏览目录树，读取文本和图片，编辑并保存文本文件。", Priority: "p0", Required: true, RelatedSurfaceRefs: []string{protocolSurfaceFiles}, AcceptanceRefs: []string{"ac-files"}}, {FeatureID: "feature-settings-recovery", Title: "设置与状态恢复", Summary: "保存主题、连接地址、上次项目和对话，并在错误时展示可恢复空状态。", Priority: "p1", Required: true, RelatedSurfaceRefs: []string{protocolSurfaceSettings, protocolSurfaceConnection}, AcceptanceRefs: []string{"ac-settings-recovery"}}}
}

func protocolClientSurfaces() []InteractionSurface {
	return []InteractionSurface{{SurfaceID: protocolSurfaceConnection, Label: "连接配置页", Purpose: "承载 host/port/token 输入、保存和 system info 验证。", PrimaryFeatureRefs: []string{"feature-connection"}}, {SurfaceID: protocolSurfaceHome, Label: "主页 Shell", Purpose: "承载顶部连接状态、项目入口和对话/文件底部 Tab。", PrimaryFeatureRefs: []string{"feature-projects", "feature-conversations", "feature-files"}}, {SurfaceID: protocolSurfaceProjectDrawer, Label: "项目抽屉", Purpose: "承载项目列表、状态展示和项目切换。", PrimaryFeatureRefs: []string{"feature-projects"}}, {SurfaceID: protocolSurfaceConversation, Label: "对话 Surface", Purpose: "承载对话列表、创建、详情、发送、流式事件和急停。", PrimaryFeatureRefs: []string{"feature-conversations", "feature-streaming-chat"}}, {SurfaceID: protocolSurfaceFiles, Label: "文件 Surface", Purpose: "承载目录树、面包屑、文本编辑保存和图片缩放预览。", PrimaryFeatureRefs: []string{"feature-files"}}, {SurfaceID: protocolSurfaceSettings, Label: "设置 Surface", Purpose: "承载主题、连接地址、关于页和本地恢复设置。", PrimaryFeatureRefs: []string{"feature-settings-recovery", "feature-connection"}}}
}

func protocolClientUserFlows() []UserFlow {
	return []UserFlow{{FlowID: "flow-connect", Title: "连接并进入主页", Steps: []FlowStep{{StepID: "flow-connect-open", SurfaceRef: protocolSurfaceConnection, Actor: "user", Title: "输入 host、port 和可选 token", ExpectedResult: "连接设置保存到 SharedPreferences"}, {StepID: "flow-connect-check", SurfaceRef: protocolSurfaceConnection, Actor: "app", Title: "调用 GET /system/info", ExpectedResult: "连接成功后进入主页，失败展示错误"}}}, {FlowID: "flow-project-switch", Title: "切换项目并刷新对话", Steps: []FlowStep{{StepID: "flow-projects", SurfaceRef: protocolSurfaceProjectDrawer, Actor: "app", Title: "调用 GET /projects", ExpectedResult: "项目列表和状态可见"}, {StepID: "flow-switch", SurfaceRef: protocolSurfaceProjectDrawer, Actor: "user", Title: "选择项目", ExpectedResult: "清空当前对话和文件预览，保存 last_project_id，关闭旧 WebSocket，并以新 project_id 重连刷新对话与文件树"}}}, {FlowID: "flow-chat", Title: "对话详情流式交互", Steps: []FlowStep{{StepID: "flow-conversation-list", SurfaceRef: protocolSurfaceConversation, Actor: "app", Title: "调用 GET /conversations", ExpectedResult: "thread/task 混排列表可见"}, {StepID: "flow-conversation-detail", SurfaceRef: protocolSurfaceConversation, Actor: "app", Title: "调用 GET /conversations/{id} 并发送 subscribe", ExpectedResult: "历史消息和后续 delta 合并显示，退出时发送 unsubscribe"}, {StepID: "flow-send-turn", SurfaceRef: protocolSurfaceConversation, Actor: "user", Title: "发送消息并调用 POST /conversations/{id}/turns", ExpectedResult: "用户气泡立即显示，AI delta 流式追加；没有当前对话时自动创建普通 thread"}, {StepID: "flow-stop", SurfaceRef: protocolSurfaceConversation, Actor: "user", Title: "运行中点击急停并确认", ExpectedResult: "确认后调用 POST /conversations/{id}/emergency-stop"}}}, {FlowID: "flow-files", Title: "浏览并保存文件", Steps: []FlowStep{{StepID: "flow-file-tree", SurfaceRef: protocolSurfaceFiles, Actor: "app", Title: "调用 GET /files/tree", ExpectedResult: "目录和文件条目可见"}, {StepID: "flow-file-read", SurfaceRef: protocolSurfaceFiles, Actor: "user", Title: "打开文本或图片文件", ExpectedResult: "文本可编辑或图片可缩放预览"}, {StepID: "flow-file-save", SurfaceRef: protocolSurfaceFiles, Actor: "user", Title: "编辑文本文件并保存", ExpectedResult: "调用 PUT /files/write 保存当前文件内容"}}}}
}

func protocolClientBehaviorRules() []DomainBehaviorRule {
	return []DomainBehaviorRule{{RuleID: "behavior-connect", Kind: "connect", Description: "连接设置保存后必须通过 system info 验证。", CapabilityRefs: []string{"connection-settings", "rest-api"}, SurfaceRefs: []string{protocolSurfaceConnection}, EntityRefs: []string{"entity-connection-settings"}, AcceptanceRefs: []string{"ac-connection"}, Required: true}, {RuleID: "behavior-rest-projects", Kind: "read-remote", Description: "项目与对话数据来自 REST，不写入业务本地库；切换项目时刷新对话、文件树并重连 WebSocket。", CapabilityRefs: []string{"rest-api", "project-switching", "conversation-list"}, SurfaceRefs: []string{protocolSurfaceHome, protocolSurfaceProjectDrawer, protocolSurfaceConversation, protocolSurfaceFiles}, EntityRefs: []string{"entity-project", "entity-conversation", "entity-file-node"}, AcceptanceRefs: []string{"ac-projects", "ac-conversations", "ac-files"}, Required: true}, {RuleID: "behavior-stream", Kind: "stream", Description: "对话详情必须订阅 WebSocket delta，退出时 unsubscribe，并处理 turn_completed usage 与 replay_truncated。", CapabilityRefs: []string{"websocket-streaming", "streaming-chat"}, SurfaceRefs: []string{protocolSurfaceConversation}, EntityRefs: []string{"entity-message", "entity-message-segment"}, AcceptanceRefs: []string{"ac-streaming-chat"}, Required: true}, {RuleID: "behavior-files-save", Kind: "write-remote", Description: "文件能力支持文本读取、编辑保存和图片预览；禁止搜索、冲突检测和二进制编辑。", CapabilityRefs: []string{"file-browser", "read-only-file-preview", "file-edit-save"}, SurfaceRefs: []string{protocolSurfaceFiles}, EntityRefs: []string{"entity-file-node"}, AcceptanceRefs: []string{"ac-files", "check-protocol-negative-capabilities"}, Required: true}, {RuleID: "behavior-settings", Kind: "persist-settings", Description: "本地只保存连接设置和最近选择。", CapabilityRefs: []string{"settings-store"}, SurfaceRefs: []string{protocolSurfaceSettings, protocolSurfaceConnection}, EntityRefs: []string{"entity-connection-settings"}, AcceptanceRefs: []string{"ac-settings-recovery"}, Required: true}}
}

func protocolClientProtocolContract() *ProtocolContract {
	return &ProtocolContract{BasePath: "/api/v1", ProjectContext: ProjectContextSpec{Required: true, QueryKey: "project_id", HeaderKey: "X-Project-Id", EntityRefs: []string{"entity-project"}}, Auth: AuthContract{Mode: "optional-token", HeaderKey: "Authorization", Required: false}, Endpoints: []EndpointContract{{EndpointID: "endpoint-system-info", Method: "GET", Path: "/system/info", Purpose: "验证服务健康与版本。", CapabilityRefs: []string{"connection-settings"}, Required: true}, {EndpointID: "endpoint-projects-list", Method: "GET", Path: "/projects", Purpose: "获取项目列表。", EntityRefs: []string{"entity-project"}, CapabilityRefs: []string{"project-switching"}, Required: true}, {EndpointID: "endpoint-project-detail", Method: "GET", Path: "/projects/{id}", Purpose: "获取项目详情。", EntityRefs: []string{"entity-project"}, CapabilityRefs: []string{"project-switching"}, Required: true}, {EndpointID: "endpoint-conversations-list", Method: "GET", Path: "/conversations", Purpose: "获取对话列表。", EntityRefs: []string{"entity-conversation"}, CapabilityRefs: []string{"conversation-list"}, Required: true}, {EndpointID: "endpoint-conversations-create", Method: "POST", Path: "/conversations", Purpose: "创建普通对话。", EntityRefs: []string{"entity-conversation"}, CapabilityRefs: []string{"conversation-create"}, RequestBody: "title/source", Required: true}, {EndpointID: "endpoint-conversation-detail", Method: "GET", Path: "/conversations/{id}", Purpose: "读取对话历史。", EntityRefs: []string{"entity-conversation", "entity-message"}, CapabilityRefs: []string{"streaming-chat"}, Required: true}, {EndpointID: "endpoint-send-turn", Method: "POST", Path: "/conversations/{id}/turns", Purpose: "发送用户消息。", EntityRefs: []string{"entity-message"}, CapabilityRefs: []string{"streaming-chat"}, RequestBody: "message", Required: true}, {EndpointID: "endpoint-emergency-stop", Method: "POST", Path: "/conversations/{id}/emergency-stop", Purpose: "对话级急停。", CapabilityRefs: []string{"emergency-stop"}, Required: true}, {EndpointID: "endpoint-files-tree", Method: "GET", Path: "/files/tree", Purpose: "读取目录树。", EntityRefs: []string{"entity-file-node"}, CapabilityRefs: []string{"file-browser"}, Required: true}, {EndpointID: "endpoint-files-read", Method: "GET", Path: "/files/read", Purpose: "读取文本或图片。", EntityRefs: []string{"entity-file-node"}, CapabilityRefs: []string{"read-only-file-preview"}, Required: true}, {EndpointID: "endpoint-files-write", Method: "PUT", Path: "/files/write", Purpose: "保存文本文件内容。", EntityRefs: []string{"entity-file-node"}, CapabilityRefs: []string{"file-edit-save"}, RequestBody: "text/plain", Required: true}, {EndpointID: "endpoint-projects-create", Method: "POST", Path: "/projects", Excluded: true}, {EndpointID: "endpoint-project-start", Method: "POST", Path: "/projects/{id}/start", Excluded: true}, {EndpointID: "endpoint-project-stop", Method: "POST", Path: "/projects/{id}/stop", Excluded: true}, {EndpointID: "endpoint-project-delete", Method: "DELETE", Path: "/projects/{id}", Excluded: true}, {EndpointID: "endpoint-files-search", Method: "GET", Path: "/files/search", Excluded: true}, {EndpointID: "endpoint-system-qrcode", Method: "GET", Path: "/system/qrcode", Excluded: true}}}
}

func protocolClientRealtimeContract() *RealtimeContract {
	return &RealtimeContract{Transport: "websocket", URLPattern: "ws://{host}:{port}/ws?project_id={project_id}", Subscribe: RealtimeClientMessage{Type: "subscribe", Required: []string{"conversation_id"}}, Unsubscribe: RealtimeClientMessage{Type: "unsubscribe", Required: []string{"conversation_id"}}, ReconnectPolicy: "exponential-backoff-and-rest-refresh", ReplayPolicy: "replay_truncated triggers GET /conversations/{id}", ServerEvents: []RealtimeServerEvent{{EventType: "connected", Purpose: "连接确认。", CapabilityRefs: []string{"websocket-streaming"}, EntityRefs: []string{"entity-project"}}, {EventType: "delta", Purpose: "AI 回复增量。", CapabilityRefs: []string{"streaming-chat", "markdown-rendering", "reasoning-block", "tool-call-status"}, EntityRefs: []string{"entity-message-segment"}}, {EventType: "turn_completed", Purpose: "标记 turn 完成并保存 usage。", CapabilityRefs: []string{"streaming-chat"}, EntityRefs: []string{"entity-message"}}, {EventType: "conversation_update", Purpose: "更新对话状态。", CapabilityRefs: []string{"streaming-chat", "emergency-stop"}, EntityRefs: []string{"entity-conversation"}}, {EventType: "instance_status", Purpose: "更新连接或引擎状态。", CapabilityRefs: []string{"connection-settings"}, EntityRefs: []string{"entity-project"}}, {EventType: "replay_truncated", Purpose: "触发 REST 全量刷新。", CapabilityRefs: []string{"websocket-streaming", "streaming-chat"}, EntityRefs: []string{"entity-conversation"}}}}
}

func protocolClientRuntimeContract() *RuntimeDependencyContract {
	return &RuntimeDependencyContract{PackageName: "flutter_open_lite", AppEntry: "lib/main.dart", RouteStrategy: "MaterialApp + Provider tree + page-local Navigator routes", StateManagement: "provider", SettingsStore: "SharedPreferences", Dependencies: []DependencyContract{{Name: "http", Version: "^1.2.0", Purpose: "REST client"}, {Name: "web_socket_channel", Version: "^2.4.0", Purpose: "WebSocket stream"}, {Name: "flutter_markdown", Version: "^0.7.0", Purpose: "Markdown chat bubbles"}, {Name: "shared_preferences", Version: "^2.2.0", Purpose: "connection settings"}, {Name: "provider", Version: "^6.1.0", Purpose: "state management"}, {Name: "cupertino_icons", Version: "^1.0.8", Purpose: "iOS-style icons"}}, PlatformConfig: []PlatformConfig{{Platform: "android", Paths: []string{"android/app/build.gradle.kts", "android/app/src/main/AndroidManifest.xml", "android/app/src/main/kotlin/com/appfactory/onepilot/MainActivity.kt"}, Purpose: "INTERNET permission, cleartext LAN HTTP, matching OnePilot MainActivity package, and arm64-v8a-only APK packaging."}, {Platform: "ios", Paths: []string{"ios/Runner/Info.plist"}, Purpose: "Target iPhone 13 and newer 64-bit devices; keep local network/ATS follow-up visible while iOS build is not current gate."}}, FakeTestHarness: []string{"fake REST responses", "fake WebSocket stream", "counter demo blocker", "negative capability source scan", "android arm64-v8a source scan"}}
}

func protocolClientAcceptanceCriteria() []AcceptanceCriterion {
	return []AcceptanceCriterion{{CriterionID: "ac-connection", Label: "连接配置可验证", Category: "functional", Required: true, Description: "host/port/token 保存后调用 GET /system/info，失败时显示错误空状态。"}, {CriterionID: "ac-projects", Label: "项目列表可展示并切换", Category: "functional", Required: true, Description: "调用 GET /projects 和 GET /projects/{id}，切换 project_id 后刷新对话、文件树和 WebSocket。"}, {CriterionID: "ac-conversations", Label: "对话列表和创建可用", Category: "functional", Required: true, Description: "调用 GET /conversations 和 POST /conversations，thread/task 混排展示；无当前对话时发送可自动创建普通 thread。"}, {CriterionID: "ac-streaming-chat", Label: "流式对话可用", Category: "functional", Required: true, Description: "详情加载历史、subscribe/unsubscribe、delta 聚合、turn_completed usage、replay_truncated 刷新、发送消息和确认急停。"}, {CriterionID: "ac-files", Label: "文件浏览和文本保存可用", Category: "functional", Required: true, Description: "调用 GET /files/tree、GET /files/read 和 PUT /files/write，支持文本编辑保存、复制、图片预览和大文件提示。"}, {CriterionID: "ac-settings-recovery", Label: "设置和恢复可用", Category: "functional", Required: true, Description: "主题、连接地址、last project/conversation 使用 SharedPreferences 保存并恢复。"}, {CriterionID: "ac-platform-targets", Label: "平台目标正确", Category: "platform", Required: true, Description: "Android 仅打包 arm64-v8a；iOS 目标记录为 iPhone 13 及以上 64 位设备，本轮不要求 iOS build。"}, {CriterionID: "ac-runtime-bootstrap", Label: "App 启动和依赖接线正确", Category: "smoke", Required: true, Description: "main/app/provider/routes/pubspec/platform config 实际接线，不保留 counter demo。"}, {CriterionID: "ac-fake-protocol", Label: "Fake protocol 验收可跑通", Category: "test", Required: true, Description: "无真实服务端时，fake REST/WS widget tests 覆盖主流程。"}}
}

func protocolClientManualReviewPoints() []ManualReviewPoint {
	return []ManualReviewPoint{{PointID: "mrp-protocol-contract", Summary: "确认 endpoint 与 WebSocket 事件来自输入协议", Reason: "避免按 App 名称写死专用分支。", Owner: "engineering"}, {PointID: "mrp-negative-capabilities", Summary: "确认 MVP 排除能力没有生成入口", Reason: "项目生命周期、文件搜索和扫码入口不得出现在源码或 UI；文本文件保存允许出现。", Owner: "product"}}
}

func protocolClientTaskBundle() []appruns.TaskBundleItem {
	return []appruns.TaskBundleItem{
		protocolClientTask("task-protocol-models", "生成协议领域模型", appruns.TaskCategoryDomain, []string{"lib/models/protocol_models.dart"}, nil, []string{"ac-protocol-models", "ac-streaming-chat", "ac-files"}, []string{"entity-project", "entity-conversation", "entity-message", "entity-message-segment", "entity-file-node", "entity-connection-settings"}),
		protocolClientTask("task-protocol-services", "生成 REST/WS/settings 基础层", appruns.TaskCategoryStorage, []string{"lib/services/api_client.dart", "lib/services/ws_client.dart", "lib/services/settings_store.dart"}, []string{"task-protocol-models"}, []string{"ac-connection", "ac-streaming-chat", "ac-settings-recovery"}, []string{"entity-connection-settings", "entity-project", "entity-conversation", "entity-file-node"}),
		protocolClientTask("task-protocol-state", "生成 Provider 状态层", appruns.TaskCategoryFlow, []string{"lib/providers/connection_provider.dart", "lib/providers/project_provider.dart", "lib/providers/conversation_provider.dart", "lib/providers/file_provider.dart"}, []string{"task-protocol-services"}, []string{"ac-projects", "ac-conversations", "ac-streaming-chat", "ac-files"}, []string{"entity-project", "entity-conversation", "entity-message", "entity-file-node"}),
		protocolClientTask("task-protocol-app-entry", "生成 App bootstrap、依赖和平台网络配置", appruns.TaskCategoryScreen, []string{"pubspec.yaml", "lib/main.dart", "lib/app.dart", "android/app/build.gradle.kts", "android/app/src/main/AndroidManifest.xml", "android/app/src/main/kotlin/com/appfactory/onepilot/MainActivity.kt", "android/app/src/main/res/values/strings.xml"}, []string{"task-protocol-state"}, []string{"ac-runtime-bootstrap", "ac-platform-targets", "ac-connection"}, []string{"entity-connection-settings"}),
		protocolClientTask("task-protocol-surfaces", "生成连接、主页、对话、文件和设置 UI", appruns.TaskCategoryScreen, []string{"lib/screens/home.dart", "lib/screens/conversations/list_page.dart", "lib/screens/conversations/detail_page.dart", "lib/screens/files/browser_page.dart", "lib/screens/files/file_preview_page.dart", "lib/screens/settings/connection_page.dart", "lib/widgets/project_drawer.dart", "lib/widgets/chat_bubble.dart", "lib/widgets/thinking_block.dart", "lib/widgets/connection_indicator.dart", "lib/widgets/file_tree_tile.dart"}, []string{"task-protocol-app-entry"}, []string{"ac-projects", "ac-conversations", "ac-streaming-chat", "ac-files", "ac-settings-recovery"}, []string{"entity-project", "entity-conversation", "entity-message", "entity-message-segment", "entity-file-node"}),
		protocolClientTask("task-protocol-tests", "生成 fake protocol widget tests", appruns.TaskCategoryValidation, []string{"test/widget_test.dart"}, []string{"task-protocol-surfaces"}, []string{"ac-fake-protocol", "check-protocol-endpoints", "check-protocol-negative-capabilities"}, []string{"entity-project", "entity-conversation", "entity-file-node"}),
		protocolClientTask("task-protocol-cleanup-generic-seed", "清理 generic record seed 残留", appruns.TaskCategoryValidation, protocolClientLegacyOpenLitePaths(), []string{"task-protocol-tests"}, []string{"check-protocol-no-generic-shell"}, nil),
	}
}

func protocolClientTask(taskID, title string, category appruns.TaskCategory, targetPaths, dependencies, acceptanceRefs, entityRefs []string) appruns.TaskBundleItem {
	return appruns.TaskBundleItem{TaskID: taskID, Title: title, Category: category, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, RiskLevel: appruns.TaskRiskLevelHigh, Objective: title + "，并严格按 protocol-client artifacts 生成，不退回 generic local shell。", Priority: "p0", RelatedRequirements: append([]string(nil), acceptanceRefs...), Dependencies: append([]string(nil), dependencies...), TargetPaths: append([]string(nil), targetPaths...), CompletionCriteria: []string{title + "已完成", "不包含 generic record/local-hive/counter demo 语义"}, OutputExpectations: []string{"目标文件可被 analyze/test/build 消费", "fake protocol tests 可验证主流程"}, RiskNotes: []string{"不要按 OnePilot 名称写专用分支，必须按协议合同生成。"}, AllocationTransition: &appruns.TaskAllocationTransition{AllocationID: taskID, SemanticIntentRefs: append([]string(nil), acceptanceRefs...), BindingRefs: protocolClientBindingRefsForPaths(targetPaths), SurfaceRefs: protocolClientSurfaceRefsForTask(taskID), EntityRefs: append([]string(nil), entityRefs...), OwnedPaths: append([]string(nil), targetPaths...), BlockedBy: append([]string(nil), dependencies...), SuccessEvidence: []string{title + "已生成"}}}
}

func protocolClientBindingRefsForPaths(paths []string) []string {
	refs := make([]string, 0, len(paths))
	for _, path := range paths {
		switch {
		case strings.Contains(path, "/models/"):
			refs = append(refs, "protocol-models")
		case strings.Contains(path, "/services/"):
			refs = append(refs, "protocol-services")
		case strings.Contains(path, "/providers/"):
			refs = append(refs, "protocol-state")
		case strings.Contains(path, "/screens/") || strings.Contains(path, "/widgets/"):
			refs = append(refs, "protocol-surfaces")
		case strings.HasPrefix(path, "test/"):
			refs = append(refs, "protocol-tests")
		case path == "lib/main.dart" || path == "lib/app.dart" || path == "pubspec.yaml" || strings.HasPrefix(path, "android/"):
			refs = append(refs, "protocol-app-entry")
		}
	}
	return uniqueStrings(refs)
}

func protocolClientSurfaceRefsForTask(taskID string) []string {
	switch taskID {
	case "task-protocol-models", "task-protocol-services", "task-protocol-state", "task-protocol-app-entry", "task-protocol-tests", "task-protocol-cleanup-generic-seed":
		return []string{protocolSurfaceConnection, protocolSurfaceHome, protocolSurfaceProjectDrawer, protocolSurfaceConversation, protocolSurfaceFiles, protocolSurfaceSettings}
	case "task-protocol-surfaces":
		return []string{protocolSurfaceConnection, protocolSurfaceHome, protocolSurfaceProjectDrawer, protocolSurfaceConversation, protocolSurfaceFiles, protocolSurfaceSettings}
	default:
		return nil
	}
}

func protocolClientAcceptanceChecks(realBuild bool) []appruns.AcceptanceCheck {
	checks := []appruns.AcceptanceCheck{
		{CheckID: "check-protocol-endpoints", Label: "确认 required REST/WS 协议已生成", Stage: appruns.StageCheap, Required: true, Commands: []string{protocolRequiredEndpointCheckCommand()}, SuccessCriteria: "源码中包含 OnePilot MVP 必需 REST endpoints、WebSocket subscribe/delta 和 ApiClient/WsClient。", TimeoutSeconds: 30},
		{CheckID: "check-protocol-negative-capabilities", Label: "阻断 MVP 排除能力", Stage: appruns.StageCheap, Required: true, Commands: []string{protocolNegativeCapabilityCheckCommand()}, SuccessCriteria: "源码不包含项目生命周期、文件搜索或扫码入口；允许文本保存。", TimeoutSeconds: 30},
		{CheckID: "check-protocol-runtime-bootstrap", Label: "确认 protocol-client 入口和依赖接线", Stage: appruns.StageCheap, Required: true, Commands: []string{protocolRuntimeBootstrapCheckCommand()}, SuccessCriteria: "main/app/provider/routes/pubspec/platform config 已接线且不保留 counter demo。", TimeoutSeconds: 30},
		{CheckID: "check-protocol-platform-targets", Label: "确认平台目标约束", Stage: appruns.StageCheap, Required: true, Commands: []string{protocolPlatformTargetsCheckCommand()}, SuccessCriteria: "Android build.gradle.kts 仅声明 arm64-v8a ABI，iOS 目标保留 iPhone 13+ 64 位要求。", TimeoutSeconds: 30},
		{CheckID: "check-protocol-no-generic-shell", Label: "阻断 generic record/local-hive 壳", Stage: appruns.StageCheap, Required: true, Commands: []string{protocolNoGenericShellCheckCommand()}, SuccessCriteria: "源码和依赖不再把通用记录、record_repository 或 Hive 作为业务主线。", TimeoutSeconds: 30},
	}
	if !realBuild {
		return append([]appruns.AcceptanceCheck{{CheckID: "check-protocol-context-ready", Label: "确认 protocol-client 输入包", Stage: appruns.StageBaseline, Required: true, Commands: []string{"echo protocol-client-context-ready"}, SuccessCriteria: "protocol-client artifacts 已生成。", TimeoutSeconds: 30}}, checks...)
	}
	flutterProfile := appruns.NewFlutterAndroidProfile()
	result := make([]appruns.AcceptanceCheck, 0, len(flutterProfile.CommandChecks)+len(checks)+1)
	if len(flutterProfile.CommandChecks) > 0 {
		result = append(result, flutterProfile.CommandChecks[0])
	}
	result = append(result, checks...)
	if len(flutterProfile.CommandChecks) > 1 {
		result = append(result, flutterProfile.CommandChecks[1:]...)
	}
	return result
}

func protocolRequiredEndpointCheckCommand() string {
	return "grep -ER 'class .*ApiClient|GET /system/info|/system/info|getSystemInfo|/projects|getProjects|/conversations|getConversations|createConversation|sendTurn|emergencyStop|/files/tree|getFileTree|/files/read|readFile|/files/write|writeFile' lib test >/dev/null 2>&1 && grep -ER 'class .*WsClient|subscribe|unsubscribe|delta|replay_truncated|conversation_update|turn_completed|instance_status' lib test >/dev/null 2>&1"
}

func protocolNegativeCapabilityCheckCommand() string {
	return "! grep -ER 'startProject|stopProject|deleteProject|createProject|searchFiles|qrcode|QrCode|scan' lib test >/dev/null 2>&1"
}

func protocolRuntimeBootstrapCheckCommand() string {
	return "grep -E 'http:|web_socket_channel:|flutter_markdown:|shared_preferences:|provider:' pubspec.yaml >/dev/null 2>&1 && grep -ER 'OnePilotApp|MultiProvider|ConnectionProvider|ProjectProvider|ConversationProvider|FileProvider' lib/main.dart lib/app.dart lib/providers >/dev/null 2>&1 && grep -q 'package com.appfactory.onepilot' android/app/src/main/kotlin/com/appfactory/onepilot/MainActivity.kt && ! grep -ER 'Flutter Demo Home Page|You have pushed the button this many times|Counter increments smoke test|_counter|_incrementCounter|MyHomePage' lib test >/dev/null 2>&1"
}

func protocolPlatformTargetsCheckCommand() string {
	return "grep -E 'abiFilters.*arm64-v8a|arm64-v8a.*abiFilters' android/app/build.gradle.kts >/dev/null 2>&1 && ! grep -E 'abiFilters.*(armeabi-v7a|x86|x86_64)|(armeabi-v7a|x86|x86_64).*abiFilters' android/app/build.gradle.kts >/dev/null 2>&1"
}

func protocolNoGenericShellCheckCommand() string {
	return "! grep -ER '通用记录|HiveRecordRepository|record_repository|hive_flutter|Hive' lib pubspec.yaml test >/dev/null 2>&1"
}

func protocolClientRequirementHighlights(requirementText string) []string {
	return extractRequirementHighlights(requirementText, []string{"REST endpoint、WebSocket 和远端服务信号必须进入 protocol-client 分支。", "MVP 使用手动 host/port/token，不做扫码、项目生命周期或文件搜索，允许文本文件保存。", "Android 产物只支持 arm64-v8a；iOS 目标为 iPhone 13 及以上 64 位设备。", "Flutter 工程必须使用 http、web_socket_channel、flutter_markdown、shared_preferences、provider。"})
}

func protocolClientAllowedPaths() []string {
	return []string{"lib/**", "assets/**", "pubspec.yaml", "test/**", "android/app/build.gradle.kts", "android/app/src/main/AndroidManifest.xml", "android/app/src/main/kotlin/com/appfactory/onepilot/MainActivity.kt", "android/app/src/main/res/values/strings.xml"}
}

func protocolClientProtectedPaths() []string {
	return []string{"android/gradle/**", "android/local.properties", "ios/**", "linux/**", "macos/**", "windows/**"}
}

func protocolClientLegacyOpenLitePaths() []string {
	return []string{"lib/models/record.dart", "lib/models/dashboard_summary.dart", "lib/repositories/record_repository.dart", "lib/controllers/home_controller.dart", "lib/controllers/record_list_controller.dart", "lib/controllers/record_form_controller.dart", "lib/views/home_page.dart", "lib/views/record_list_page.dart", "lib/views/record_detail_page.dart", "lib/views/record_form_page.dart", "lib/template/open_lite_copy.dart"}
}
