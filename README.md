# hapi-lite 架构与代码说明

## 一、项目定位

hapi-lite 是一个轻量级的 AI 编程助手聚合平台，提供统一的 Web UI 来管理和使用多种 AI Coding Agent（claude / codex / gemini / opencode）。后端由 Go 编写，前端为 React/TypeScript SPA，两者打包为单一二进制部署。

---

## 二、整体架构图

```
┌─────────────────────────────────────────────────────────────────┐
│                        Browser / SPA                            │
│  React + TypeScript (web/src)                                   │
│  ┌──────────┐  ┌────────────┐  ┌───────────┐  ┌────────────┐  │
│  │SessionList│  │SessionChat │  │  FileView │  │  Settings  │  │
│  └────┬─────┘  └─────┬──────┘  └─────┬─────┘  └────────────┘  │
│       │              │               │                          │
│       └──────────────┴───────────────┘                          │
│              REST API + SSE (EventSource)                        │
└──────────────────────────┬──────────────────────────────────────┘
                           │ HTTP
┌──────────────────────────▼──────────────────────────────────────┐
│                     Go Backend (main.go)                         │
│                                                                  │
│  ┌─────────────┐   ┌──────────────┐   ┌──────────────────────┐  │
│  │  Gin Router │   │  SSE Broker  │   │   SQLite Store       │  │
│  │  /api/...   │──▶│  (sse/)      │   │   sessions / msgs    │  │
│  └──────┬──────┘   └──────┬───────┘   └──────────────────────┘  │
│         │                 │                                      │
│  ┌──────▼─────────────────▼───────────────────────────────────┐ │
│  │                  session.Manager                            │ │
│  │  agents map[sessionID]*AgentProcess                        │ │
│  │  SpawnAgent / SendMessage / StopAgent / AbortAgent         │ │
│  └───────────────────────┬────────────────────────────────────┘ │
│                          │                                       │
│     ┌────────────────────┼──────────────────────┐               │
│     ▼                    ▼                       ▼               │
│  Claude              Codex               Gemini / Opencode       │
│  (in-process         (in-process         (subprocess +           │
│   stdout parse)       stdout parse)       file scanner)          │
└─────────────────────────────────────────────────────────────────┘
                          │ exec.Cmd
    ┌─────────────────────┼────────────────────────────┐
    ▼                     ▼                            ▼
claude CLI           codex CLI              gemini / opencode CLI
(stream-json)        (--json)               (output → ~/.xxx/sessions/*.jsonl)
```

---

## 三、多 Agent 管理与会话流程

### 3.1 会话创建流程

```
Client                  API                  Manager               Agent CLI
  │                      │                      │                      │
  │  POST /api/sessions  │                      │                      │
  │─────────────────────▶│                      │                      │
  │  {directory, agent,  │                      │                      │
  │   model, yolo}       │                      │                      │
  │                      │ Store.CreateSession() │                      │
  │                      │──────────────────────▶ (SQLite insert)      │
  │                      │                      │                      │
  │                      │ Mgr.SpawnAgent()     │                      │
  │                      │─────────────────────▶│                      │
  │                      │                      │ (create AgentProcess)│
  │                      │                      │                      │
  │                      │                      │ start file scanner   │
  │                      │                      │ (gemini/opencode)    │
  │                      │                      │                      │
  │  {sessionId}         │                      │                      │
  │◀─────────────────────│                      │                      │
  │                      │                      │                      │
  │  GET /api/events     │                      │                      │
  │─────────────────────▶│ SSE long connection  │                      │
```

### 3.2 发送消息 / Agent 运行流程

```
Client                  API                  Manager               Agent CLI
  │                      │                      │                      │
  │  POST /sessions/:id/ │                      │                      │
  │       messages       │                      │                      │
  │─────────────────────▶│                      │                      │
  │                      │ Mgr.SendMessage()    │                      │
  │                      │─────────────────────▶│                      │
  │                      │                      │ go runOnce()         │
  │                      │                      │─────────────────────▶│
  │  {ok:true, requestId}│                      │                      │
  │◀─────────────────────│                      │  exec.Command(...)   │
  │                      │                      │  cmd.Start()         │
  │                      │         ┌────────────┘                      │
  │                      │         │ stdout scan / file tail           │
  │                      │         │ parse JSON lines                  │
  │                      │         │                                   │
  │                      │         │ emitMessage()                     │
  │                      │         │  → Store.InsertMessage()          │
  │                      │         │  → Broker.Publish(SyncEvent)      │
  │                      │         │                                   │
  │  SSE: message-appended         │                                   │
  │◀───────────────────────────────┘                                   │
```

当前发送链路采用统一派发：
- `request -> sender dispatch -> <agent>Sender`
- `<agent>Sender` 内部包含 `command builder map`（new/resume 等）
- `<agent>Sender` 内部包含 `parse handler map`（event/item -> action）

### 3.3 删除会话流程

> **前提条件**：会话必须处于非活跃状态（已归档）。若会话仍在运行，需先调用 Archive 接口将其停止，否则返回 409。

```
Client                  API                  Manager               SQLite
  │                      │                      │                      │
  │  DELETE              │                      │                      │
  │  /api/sessions/:id   │                      │                      │
  │─────────────────────▶│                      │                      │
  │                      │ Store.GetSession(id) │                      │
  │                      │─────────────────────────────────────────────▶
  │                      │◀─────────────────────────────── sess ────────
  │                      │                      │                      │
  │  ← 404 Not Found     │ [sess == nil]        │                      │
  │◀─────────────────────│                      │                      │
  │                      │                      │                      │
  │  ← 409 Conflict      │ [sess.Active==true]  │                      │
  │◀─────────────────────│ "Archive it first"   │                      │
  │                      │                      │                      │
  │                      │ Mgr.StopAgent(id)    │                      │
  │                      │─────────────────────▶│                      │
  │                      │                      │ 关闭 stdout 管道 /   │
  │                      │                      │ 停止 file scanner    │
  │                      │                      │                      │
  │                      │ Store.DeleteSession()│                      │
  │                      │─────────────────────────────────────────────▶
  │                      │                      │ (SQLite 事务)        │
  │                      │                      │ DELETE messages      │
  │                      │                      │   WHERE session_id   │
  │                      │                      │ DELETE sessions      │
  │                      │                      │   WHERE id           │
  │                      │                      │                      │
  │                      │ Broker.Publish(      │                      │
  │                      │  "session-removed")  │                      │
  │  SSE: session-removed│                      │                      │
  │◀─────────────────────│                      │                      │
  │  {ok: true}          │                      │                      │
  │◀─────────────────────│                      │                      │
```

### 3.4 四种 Agent 的接入方式对比

| Agent        | 启动命令                                       | 消息获取方式                                    | 会话文件路径                         |
|--------------|------------------------------------------------|-------------------------------------------------|--------------------------------------|
| **claude**   | `claude --print --output-format stream-json`   | 直接读取 stdout，按行解析 JSON                  | N/A（in-process）                    |
| **codex**    | `codex exec --json --skip-git-repo-check`      | 直接读取 stdout，解析 `item.completed` 事件     | N/A（in-process）                    |
| **gemini**   | `gemini <text>`                                | GeminiScanner tail `*.jsonl`                    | `~/.gemini/sessions/*.jsonl`         |
| **opencode** | `opencode <text>`                              | OpencodeScanner tail `*.jsonl`                  | `~/.opencode/sessions/*.jsonl`       |

> 统一映射入口：`internal/agentmap/catalog.go`  
> 在这里维护“上游报文字段 -> 内部动作 -> 状态 -> 输出结构”的规则，新增字段/新增 agent 优先改这里。

**Claude 消息格式**（stream-json 每行）：
```json
{"type": "assistant", "content": [...]}
{"type": "user", "content": [...]}
{"type": "summary", ...}
```

**Codex 消息格式**（stdout 事件流，解析 `item.completed`）：
```json
{"type": "item.completed", "item": {"type": "agent_message", "text": "..."}}
{"type": "item.completed", "item": {"type": "reasoning", "text": "..."}}
{"type": "item.completed", "item": {"type": "tool_call", "name": "...", "call_id": "..."}}
{"type": "item.completed", "item": {"type": "tool_result", "call_id": "...", "output": "..."}}
```

**Gemini / Opencode 消息格式**（JSONL 文件行，过滤 user/assistant/summary）：
```json
{"type": "user", "content": {...}}
{"type": "assistant", "content": [...]}
```

### 3.5 SSE 实时推送

所有 Agent 消息和会话状态变更均通过 SSE 推送到前端：

| SyncEvent type      | 触发时机                        |
|---------------------|---------------------------------|
| `session-added`     | 创建新会话                      |
| `session-state-changed` | 会话运行态变更（INACTIVE/READY/RUNNING） |
| `session-removed`   | 删除会话                        |
| `message-appended`  | Agent 输出一条新消息            |

---

## 四、鉴权说明

采用两阶段认证：

1. **第一步**：POST `/api/auth`，提交 `accessToken`（与 config.yaml 中的 `access_token` 做常量时间比对），返回 JWT（24h 有效期，HS256，密钥为 `jwt_secret`）。

2. **第二步**：后续所有 `/api/*` 请求携带 `Authorization: Bearer <JWT>`，SSE 连接支持 `?token=<JWT>` 查询参数。

---

## 五、目录结构与文件说明

```
hapi-lite/
│
├── main.go                          # 入口：加载配置、初始化 SQLite/SSE Broker/session.Manager，启动 Gin HTTP 服务
├── config.yaml                      # 运行时配置：port / jwt_secret / access_token / db_path
├── go.mod                           # Go 模块定义，依赖：gin / sqlite3 / jwt / uuid 等
├── go.sum                           # 依赖校验文件
│
├── internal/
│   ├── config/
│   │   └── config.go                # 配置结构体 Config，从 config.yaml 加载，提供全局变量 C
│   │
│   ├── auth/
│   │   └── auth.go                  # JWT 生成（GenerateToken）与验证（Middleware gin.HandlerFunc）
│   │
│   ├── store/
│   │   └── sqlite.go                # SQLite 数据访问层：sessions 表 + messages 表的 CRUD，消息序号重建
│   │
│   ├── session/
│   │   ├── types.go                 # 核心数据类型：Session / Message / Metadata / AgentState /
│   │   │                            #   AgentFlavor 常量 / CreateSessionRequest / SyncEvent 等
│   │   └── manager.go               # Agent 进程管理器：SpawnAgent / SendMessage / StopAgent /
│   │                                #   runClaude / runCodex / emitMessage；维护 agents map
│   │
│   ├── scanner/
│   │   ├── scanner.go               # Scanner 接口定义：Start / Stop；ScannedMessage / MessageCallback
│   │   ├── codex.go                 # 共享工具函数：findNewestFile / tailJSONL
│   │   │                            #   供 GeminiScanner / OpencodeScanner 使用
│   │   ├── gemini.go                # GeminiScanner：监听 ~/.gemini/sessions/*.jsonl，tail 新增行
│   │   └── opencode.go              # OpencodeScanner：监听 ~/.opencode/sessions/*.jsonl，tail 新增行
│   │
│   ├── sse/
│   │   └── broker.go                # SSE 事件总线：Client（订阅者）/ Broker（发布/订阅/取消订阅）
│   │
│   └── api/
│       ├── router.go                # 路由注册：公开路由（/auth）+ 受保护路由（JWT Middleware）
│       ├── base.go                  # BaseHandler：聚合 Store / Broker / Manager 三个依赖
│       ├── auth_handler.go          # POST /api/auth：accessToken 校验 → 返回 JWT
│       ├── session_handler.go       # Session CRUD + 控制操作：
│       │                            #   List / Get / Create / Delete / Resume / Abort /
│       │                            #   Archive / Rename / SetPermissionMode / SetModel /
│       │                            #   ListSlashCommands / ListSkills
│       ├── message_handler.go       # GET  /sessions/:id/messages（分页，支持 beforeSeq）
│       │                            # POST /sessions/:id/messages（发消息，懒启动 Agent）
│       ├── permission_handler.go    # POST /sessions/:id/permissions/:reqId/approve|deny（接口存根）
│       ├── file_handler.go          # 会话工作目录的文件操作：
│       │                            #   ListFiles（含关键词搜索）/ GetFile（base64）/
│       │                            #   ListDirectory / UploadFile / DeleteUploadFile
│       ├── git_handler.go           # 会话目录的 Git 查询：
│       │                            #   GitStatus / GitDiffNumstat / GitDiffFile
│       ├── machine_handler.go       # /machines：返回本机信息；/machines/:id/spawn 创建会话；
│       │                            #   /machines/:id/paths/exists 检查路径是否存在
│       ├── misc_handler.go          # POST /visibility（页面可见性心跳）
│       └── sse_handler.go           # GET /api/events：SSE 长连接，支持 sessionId 过滤 / all=true 全订阅
│
└── web/                             # 前端（React + TypeScript + Vite + Tailwind）
    ├── package.json
    ├── vite.config.ts               # Vite 构建配置，开发时反代到 :8080
    ├── tailwind.config.ts
    ├── tsconfig.json
    ├── vitest.config.ts             # Vitest 测试配置
    │
    └── src/
        ├── main.tsx                 # React 入口，挂载 App
        ├── App.tsx                  # 应用根组件，组装 Provider / Router
        ├── router.tsx               # 路由定义（TanStack Router）
        ├── index.css                # 全局样式
        ├── sw.ts                    # Service Worker（PWA 支持）
        │
        ├── api/
        │   └── client.ts            # REST API 封装：所有后端接口的 fetch 函数
        │
        ├── types/
        │   ├── api.ts               # 后端 API 响应的 TypeScript 类型定义
        │   ├── diff.d.ts            # diff 库类型声明
        │   ├── global.d.ts          # 全局类型扩展
        │   └── pwa.d.ts             # PWA 相关类型
        │
        ├── protocol/
        │   ├── types.ts             # 前端内部协议类型（Message / Session / Agent 消息格式）
        │   ├── messages.ts          # 消息内容的结构定义与类型守卫
        │   ├── schemas.ts           # Zod schema 校验
        │   ├── modes.ts             # 权限模式 / 模型模式枚举
        │   ├── sessionSummary.ts    # SessionSummary 前端表示
        │   ├── socket.ts            # SSE 连接管理（EventSource 封装、重连逻辑）
        │   ├── utils.ts             # 协议工具函数
        │   ├── version.ts           # 协议版本号
        │   └── index.ts             # 统一导出
        │
        ├── chat/
        │   ├── types.ts             # 聊天视图的内部类型（ChatMessage / ToolCall 等）
        │   ├── normalize.ts         # 统一入口：将原始消息 normalize 为 ChatMessage
        │   ├── normalizeAgent.ts    # Agent（assistant/codex/gemini）消息的 normalize 逻辑
        │   ├── normalizeUser.ts     # 用户消息的 normalize 逻辑
        │   ├── reducer.ts           # Chat 状态 reducer（消息列表增量更新）
        │   ├── reducerCliOutput.ts  # CLI 输出类消息的 reducer 片段
        │   ├── reducerEvents.ts     # 事件类消息（tool_use 等）的 reducer 片段
        │   ├── reducerTimeline.ts   # 时间线排序 reducer 片段
        │   ├── reducerTools.ts      # 工具调用 reducer 片段
        │   ├── reconcile.ts         # 消息列表的增量对账（去重、排序）
        │   ├── modelConfig.ts       # 各 Agent 支持的模型列表配置
        │   ├── presentation.ts      # 消息展示层工具（文本提取、摘要）
        │   └── tracer.ts            # 开发调试用消息追踪
        │
        ├── components/
        │   ├── AssistantChat/       # 聊天主界面
        │   │   ├── context.tsx      # Chat 页面 Context（session / messages / state）
        │   │   ├── HappyThread.tsx  # 消息列表渲染（线程视图）
        │   │   ├── HappyComposer.tsx# 输入框组件
        │   │   ├── StatusBar.tsx    # 会话状态栏（thinking 状态、模型切换）
        │   │   ├── ComposerButtons.tsx # 输入框功能按钮（发送、附件、slash 命令）
        │   │   ├── AttachmentItem.tsx  # 附件预览项
        │   │   └── messages/
        │   │       ├── UserMessage.tsx      # 用户消息气泡
        │   │       ├── AssistantMessage.tsx # AI 回复气泡（支持 Markdown）
        │   │       ├── ToolMessage.tsx      # 工具调用消息
        │   │       ├── SystemMessage.tsx    # 系统消息
        │   │       ├── MessageAttachments.tsx # 消息附件展示
        │   │       └── MessageStatusIndicator.tsx # 消息状态指示器
        │   │
        │   ├── ChatInput/
        │   │   ├── Autocomplete.tsx # slash 命令 / 技能自动补全弹层
        │   │   └── FloatingOverlay.tsx # 输入框浮层包装
        │   │
        │   ├── NewSession/          # 新建会话对话框
        │   │   ├── index.tsx        # 新建会话主组件（目录 + Agent + Model + Yolo 等）
        │   │   ├── AgentSelector.tsx# Agent 选择器（claude/codex/gemini/opencode）
        │   │   ├── ModelSelector.tsx# 模型选择器
        │   │   ├── DirectorySection.tsx # 工作目录选择
        │   │   ├── MachineSelector.tsx  # 机器选择（目前只有本机）
        │   │   ├── SessionTypeSelector.tsx # 会话类型选择器（UI 展示，后端不处理）
        │   │   ├── YoloToggle.tsx   # Yolo 模式开关（跳过权限确认）
        │   │   ├── ActionButtons.tsx# 确认/取消按钮
        │   │   ├── preferences.ts   # 用户偏好持久化（localStorage）
        │   │   ├── preferences.test.ts
        │   │   └── types.ts
        │   │
        │   ├── ToolCard/            # 工具调用卡片展示
        │   │   ├── ToolCard.tsx     # 工具卡片容器
        │   │   ├── knownTools.tsx   # 已知工具的图标/名称映射
        │   │   ├── PermissionFooter.tsx    # 权限审批操作区（approve/deny）
        │   │   ├── AskUserQuestionFooter.tsx # 用户问答操作区
        │   │   ├── RequestUserInputFooter.tsx # 用户输入操作区
        │   │   ├── askUserQuestion.ts
        │   │   ├── requestUserInput.ts
        │   │   ├── icons.tsx
        │   │   └── views/           # 各工具的自定义展示视图
        │   │       ├── _all.tsx
        │   │       └── _results.tsx
        │   │
        │   ├── SessionFiles/
        │   │   └── DirectoryTree.tsx # 会话工作目录的树形文件浏览器
        │   │
        │   ├── Terminal/
        │   │   └── TerminalView.tsx  # xterm.js 终端视图（CLI 输出展示）
        │   │
        │   ├── assistant-ui/        # Markdown 渲染 / 代码高亮（Shiki）相关组件
        │   │   ├── markdown-text.tsx
        │   │   ├── markdown-utils.ts
        │   │   ├── reasoning.tsx    # 推理过程折叠展示
        │   │   └── shiki-highlighter.tsx
        │   │
        │   ├── SessionList.tsx      # 左侧会话列表
        │   ├── SessionChat.tsx      # 会话聊天页面顶层组件
        │   ├── SessionHeader.tsx    # 会话顶栏（名称、操作菜单）
        │   ├── SessionActionMenu.tsx# 会话操作下拉菜单（重命名/归档/删除）
        │   ├── SpawnSession.tsx     # 通过 machine API 创建会话的组件
        │   ├── MachineList.tsx      # 机器列表展示
        │   ├── LoginPrompt.tsx      # 登录界面（accessToken 输入）
        │   ├── LoginPrompt.test.tsx
        │   ├── RenameSessionDialog.tsx # 重命名会话对话框
        │   ├── CodeBlock.tsx        # 代码块（带复制按钮）
        │   ├── CliOutputBlock.tsx   # CLI 输出块（monospace）
        │   ├── DiffView.tsx         # 文件 diff 展示（git diff）
        │   ├── MarkdownRenderer.tsx # 通用 Markdown 渲染器
        │   ├── FileIcon.tsx         # 文件类型图标
        │   ├── icons.tsx            # 通用图标库
        │   ├── InstallPrompt.tsx    # PWA 安装提示
        │   ├── LanguageSwitcher.tsx # 语言切换（中/英）
        │   ├── LazyRainbowText.tsx  # 彩虹文字（装饰）
        │   ├── LoadingState.tsx     # 加载状态占位
        │   ├── OfflineBanner.tsx    # 离线状态横幅
        │   ├── ReconnectingBanner.tsx # SSE 重连横幅
        │   ├── SyncingBanner.tsx    # 数据同步中横幅
        │   ├── Spinner.tsx          # 加载动画
        │   └── ToastContainer.tsx   # 全局 Toast 通知容器
        │
        ├── routes/
        │   ├── sessions/
        │   │   ├── file.tsx         # 路由页面：单文件查看（base64 解码展示）
        │   │   ├── files.tsx        # 路由页面：文件列表 / 目录树
        │   │   └── terminal.tsx     # 路由页面：终端输出视图
        │   └── settings/
        │       ├── index.tsx        # 设置页面（主题、语言等）
        │       └── index.test.tsx
        │
        ├── hooks/
        │   ├── useCurrentSession.ts # 获取当前激活会话的 hook
        │   ├── useMessages.ts       # 消息列表分页加载 hook（含 SSE 增量更新）
        │   ├── useSession.ts        # 单会话数据 + SSE 监听
        │   ├── useSessions.ts       # 会话列表 + SSE 监听
        │   └── useVisibilityReporter.ts # 页面可见性上报 hook
        │
        ├── lib/
        │   ├── app-context.tsx      # 全局 AppContext（token / user / SSE 连接状态）
        │   ├── assistant-runtime.ts # assistant-ui 运行时适配层
        │   ├── attachmentAdapter.ts # 附件上传适配
        │   ├── clipboard.ts         # 剪贴板工具
        │   ├── fileAttachments.ts   # 文件附件管理
        │   ├── gitParsers.ts        # git status/diff 输出解析
        │   ├── i18n-context.tsx     # 国际化 Context
        │   ├── locales/
        │   │   ├── en.ts            # 英文翻译
        │   │   ├── zh-CN.ts         # 中文翻译
        │   │   └── index.ts
        │   ├── message-window-store.ts # 消息窗口状态（滚动位置等）
        │   ├── messages.ts          # 消息工具函数
        │   ├── query-client.ts      # TanStack Query 客户端实例
        │   ├── query-keys.ts        # Query key 常量
        │   ├── recent-skills.ts     # 最近使用的 skill 记录（localStorage）
        │   ├── runtime-config.ts    # 运行时配置（API base URL 等）
        │   ├── shiki.ts             # Shiki 代码高亮懒加载
        │   ├── terminalFont.ts      # 终端字体加载
        │   ├── toast-context.tsx    # Toast 通知 Context
        │   ├── toolInputUtils.ts    # 工具调用输入解析工具
        │   ├── use-translation.ts   # useTranslation hook
        │   ├── agentFlavorUtils.ts  # Agent 类型工具（label / icon / 颜色）
        │   └── utils.ts             # 通用工具函数（cn / clsx 等）
        │
        └── utils/
            ├── applySuggestion.ts   # 将 slash 命令/自动补全建议应用到输入框
            ├── findActiveWord.ts    # 查找光标处的活跃词（用于自动补全触发）
            └── path.ts              # 路径工具函数
```

---

## 六、API 路由一览

### 公开路由
| 方法   | 路径         | 说明                          |
|--------|--------------|-------------------------------|
| POST   | `/api/auth`  | accessToken 换取 JWT          |

### 受保护路由（需 Bearer JWT）

**会话**
| 方法   | 路径                                      | 说明                       |
|--------|-------------------------------------------|----------------------------|
| GET    | `/api/sessions`                           | 列出所有会话               |
| POST   | `/api/sessions`                           | 创建会话并启动 Agent       |
| GET    | `/api/sessions/:id`                       | 获取会话详情               |
| DELETE | `/api/sessions/:id`                       | 删除会话（需先归档）       |
| PATCH  | `/api/sessions/:id`                       | 重命名会话                 |
| POST   | `/api/sessions/:id/resume`                | 恢复会话（重新 Spawn）     |
| POST   | `/api/sessions/:id/abort`                 | 中止 Agent 运行            |
| POST   | `/api/sessions/:id/archive`               | 归档会话（停止 Agent）     |
| POST   | `/api/sessions/:id/permission-mode`       | 设置权限模式               |
| POST   | `/api/sessions/:id/model`                 | 切换模型                   |
| GET    | `/api/sessions/:id/slash-commands`        | 列出 slash 命令            |
| GET    | `/api/sessions/:id/skills`                | 列出 codex skills          |

**消息**
| 方法   | 路径                          | 说明                           |
|--------|-------------------------------|--------------------------------|
| GET    | `/api/sessions/:id/messages`  | 分页获取消息（支持 beforeSeq） |
| POST   | `/api/sessions/:id/messages`  | 发送消息（懒启动 Agent）       |

**权限审批**
| 方法   | 路径                                               | 说明         |
|--------|----------------------------------------------------|--------------|
| POST   | `/api/sessions/:id/permissions/:reqId/approve`     | 批准权限请求 |
| POST   | `/api/sessions/:id/permissions/:reqId/deny`        | 拒绝权限请求 |

**文件**
| 方法   | 路径                              | 说明                       |
|--------|-----------------------------------|----------------------------|
| GET    | `/api/sessions/:id/files`         | 搜索 / 列出文件            |
| GET    | `/api/sessions/:id/file`          | 读取文件（base64）         |
| GET    | `/api/sessions/:id/directory`     | 列出目录                   |
| POST   | `/api/sessions/:id/upload`        | 上传文件（base64，≤50MB）  |
| POST   | `/api/sessions/:id/upload/delete` | 删除已上传文件             |

**Git**
| 方法   | 路径                                   | 说明              |
|--------|----------------------------------------|-------------------|
| GET    | `/api/sessions/:id/git-status`         | git status        |
| GET    | `/api/sessions/:id/git-diff-numstat`   | git diff numstat  |
| GET    | `/api/sessions/:id/git-diff-file`      | 单文件 diff       |

**机器 / 其他**
| 方法   | 路径                              | 说明                    |
|--------|-----------------------------------|-------------------------|
| GET    | `/api/machines`                   | 列出本机信息            |
| POST   | `/api/machines/:id/spawn`         | 在指定机器创建会话      |
| POST   | `/api/machines/:id/paths/exists`  | 检查路径是否存在        |
| POST   | `/api/visibility`                 | 页面可见性心跳          |
| GET    | `/api/events`                     | SSE 实时事件流          |

---

## 七、核心数据模型

### Session（会话）
```
Session {
  id, createdAt, updatedAt, active, activeAt
  metadata: { path(工作目录), flavor(agent类型), name, host, worktree }
  agentState: { requests(待审批权限请求), completedRequests }
  permissionMode, modelMode, thinking
}
```

### Message（消息）
```
Message {
  id, sessionId, seq(自增序号), createdAt
  content: JSON RawMessage
    - claude:   {"type":"assistant","content":[...]}
    - codex:    {"role":"agent","content":{"type":"codex","data":{...}}}
    - 用户消息:  {"role":"user","content":{"type":"text","text":"..."}}
}
```

### CreateSessionRequest
```
CreateSessionRequest {
  directory string  // 工作目录（必填）
  agent     string  // claude | codex | gemini | opencode（默认 claude）
  model     string  // 指定模型（可选）
  yolo      bool    // 跳过权限确认
}
```

### SyncEvent（SSE 推送事件）
```
SyncEvent {
  type: "session-added" | "session-state-changed" | "session-removed" | "message-appended"
  sessionId: string
  message?: Message   // 仅 message-appended 时附带
}
```

---

## 八、消息的记录与展示

### 8.1 统一存储：SQLite messages 表

无论哪种 Agent，消息最终都经由 `emitMessage()` → `Store.InsertMessage()` 写入同一张 SQLite 表：

```sql
messages (
  id          TEXT PRIMARY KEY,
  session_id  TEXT,
  seq         INTEGER,   -- 会话内自增序号，用于分页
  content_json TEXT,     -- 各 Agent 原始 JSON，格式不同
  created_at  INTEGER    -- Unix 毫秒时间戳
)
```

### 8.2 各 Agent 的消息捕获方式

| Agent | 捕获方式 | 用户消息写入 |
|-------|----------|-------------|
| **claude** | stdout pipe → 逐行解析 stream-json | claude 输出中自带 `type:user`，直接存储 |
| **codex** | stdout pipe → 解析 `item.completed` 事件 | codex 不回显用户消息，由 hapi-lite 在发送前**显式写入** |
| **gemini** | GeminiScanner tail `~/.gemini/sessions/*.jsonl` | 从 JSONL 文件读取，含 `type:user` 行 |
| **opencode** | OpencodeScanner tail `~/.opencode/sessions/*.jsonl` | 从 JSONL 文件读取，含 `type:user` 行 |

### 8.3 content_json 格式差异

**claude**（stream-json 原始行）：
```json
{"type": "user",      "content": [{"type": "text", "text": "你好"}]}
{"type": "assistant", "content": [{"type": "text", "text": "你好！"}]}
{"type": "summary",   "summary": "...", "leafUuid": "..."}
```

**codex**（hapi-lite 包装后）：
```json
// 用户消息（hapi-lite 主动写入）
{"role": "user",  "content": {"type": "text", "text": "你好"}}

// Agent 回复
{"role": "agent", "content": {"type": "codex", "data": {"type": "message",  "message": "你好！", "id": "..."}}}
{"role": "agent", "content": {"type": "codex", "data": {"type": "reasoning", "message": "...",   "id": "..."}}}
{"role": "agent", "content": {"type": "codex", "data": {"type": "tool-call", "name": "Bash", "callId": "...", "input": {...}, "id": "..."}}}
{"role": "agent", "content": {"type": "codex", "data": {"type": "tool-call-result", "callId": "...", "output": "...", "id": "..."}}}
```

**gemini / opencode**（JSONL 文件原始行，与 claude 格式接近）：
```json
{"type": "user",      "content": {...}}
{"type": "assistant", "content": [...]}
```

### 8.4 实时推送链路

```
emitMessage()
  │
  ├── onMessage(sessionID, msg)
  │     └── Store.InsertMessage()   → 持久化到 SQLite
  │
  └── onEvent(sessionID, SyncEvent{Type: "message-appended", Message: &msg})
        └── Broker.Publish()        → 推送给所有 SSE 订阅者
```

前端通过 EventSource 收到 `message-appended` 事件后，将消息追加到本地 TanStack Query 缓存，无需重新请求接口。

### 8.5 消息查询与分页

`GET /api/sessions/:id/messages?limit=50&beforeSeq=<seq>`

- 按 `seq DESC` 倒序查询，再反转为升序返回（最新消息在最后）
- `beforeSeq` 实现向上翻页（加载更早消息），类似 IM 的"上拉加载更多"
- 返回结构：

```json
{
  "messages": [...],
  "page": {
    "limit": 50,
    "beforeSeq": null,
    "nextBeforeSeq": 12,
    "hasMore": true
  }
}
```

---

## 九、关键技术依赖

| 层       | 技术栈                                                                                          |
|----------|-------------------------------------------------------------------------------------------------|
| Go 后端  | Gin（HTTP）、go-sqlite3（存储）、golang-jwt（鉴权）、uuid                                       |
| 前端     | React 18、TypeScript、Vite、TanStack Router、TanStack Query、Tailwind CSS、Shiki、xterm.js      |
| 实时通信 | SSE（Server-Sent Events），前端 EventSource                                                      |
| 持久化   | SQLite（WAL 模式），单文件                                                                       |
| Agent    | claude CLI、codex CLI、gemini CLI、opencode CLI（需系统已安装）                                  |
