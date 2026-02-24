# "Agent is Busy" 错误修复

## 问题描述

**现象**：
- 用户快速连续发送多条消息
- 后端返回 500 错误
- 前端显示消息发送失败，但不知道具体原因

**日志示例**：
```
[GIN] 2026/02/24 - 14:25:48 | 500 | ... | POST "/api/sessions/.../messages"
[GIN] 2026/02/24 - 14:25:50 | 500 | ... | POST "/api/sessions/.../messages"
[GIN] 2026/02/24 - 14:25:51 | 500 | ... | POST "/api/sessions/.../messages"
```

## 根本原因

在 `internal/session/manager.go` 的 `SendMessage` 函数中：

```go
if proc.Running {
    return fmt.Errorf("agent is busy")
}
```

当 Claude/Codex 正在处理一条消息时（`proc.Running = true`），后续的消息会被拒绝。这是为了防止并发问题，因为 Claude CLI 不支持同时处理多条消息。

## 修复方案

### 1. 后端：改进错误消息

**文件**：`internal/session/manager.go`

**修改前**：
```go
if proc.Running {
    return fmt.Errorf("agent is busy")
}
```

**修改后**：
```go
if proc.Running {
    return fmt.Errorf("agent is busy, please wait for the current message to complete")
}
```

### 2. 前端：记录错误详情

**文件**：`web/src/hooks/mutations/useSendMessage.ts`

**修改前**：
```typescript
onError: (_, input) => {
    updateMessageStatus(input.sessionId, input.localId, 'failed')
    haptic.notification('error')
},
```

**修改后**：
```typescript
onError: (error, input) => {
    updateMessageStatus(input.sessionId, input.localId, 'failed')
    haptic.notification('error')

    // Log error details for debugging
    const errorMessage = error instanceof Error ? error.message : String(error)
    console.error('Failed to send message:', errorMessage)
},
```

## 用户体验改进

### 修复前
1. 用户快速点击发送按钮 3 次
2. 第 1 条消息开始处理
3. 第 2、3 条消息返回 500 错误
4. 前端显示发送失败，但不知道原因
5. 用户困惑：为什么失败了？

### 修复后
1. 用户快速点击发送按钮 3 次
2. 第 1 条消息开始处理
3. 第 2、3 条消息返回 500 错误
4. 前端显示发送失败
5. **浏览器控制台显示**：`Failed to send message: agent is busy, please wait for the current message to complete`
6. 用户可以在控制台看到具体原因

## 最佳实践

### 前端防护（已实现）
```typescript
<HappyComposer
    disabled={props.isSending}  // ✅ 发送时禁用输入框
    ...
/>
```

### 用户操作建议
1. **等待当前消息处理完成**再发送下一条
2. 如果消息显示"发送失败"，检查浏览器控制台查看具体错误
3. 如果是"agent is busy"错误，等待几秒后重试

## 未来改进方向

### 方案 A：消息队列（推荐）
在后端实现消息队列，自动排队处理多条消息：

```go
type AgentProcess struct {
    // ... 现有字段
    messageQueue chan string  // 新增消息队列
}

func (m *Manager) SendMessage(sessionID string, text string) error {
    // 将消息放入队列，而不是直接拒绝
    proc.messageQueue <- text
    return nil
}
```

**优点**：
- 用户可以连续发送多条消息
- 自动按顺序处理
- 更好的用户体验

**缺点**：
- 需要重构代码
- 增加复杂度

### 方案 B：前端显示错误 Toast
在前端显示友好的错误提示：

```typescript
onError: (error, input) => {
    const errorMessage = error instanceof Error ? error.message : String(error)

    if (errorMessage.includes('agent is busy')) {
        addToast({
            title: 'Please wait',
            body: 'The agent is processing your previous message. Please wait a moment.',
            sessionId: input.sessionId,
            url: ''
        })
    }

    updateMessageStatus(input.sessionId, input.localId, 'failed')
}
```

**优点**：
- 用户友好的提示
- 不需要查看控制台

**缺点**：
- 仍然需要用户手动重试

## 测试验证

1. 启动服务器
2. 创建一个 session
3. 快速连续点击发送按钮 3 次
4. 验证：
   - 第 1 条消息正常发送
   - 第 2、3 条消息显示失败
   - 浏览器控制台显示错误详情
5. 等待第 1 条消息处理完成
6. 重新发送失败的消息，验证成功

## 相关文件

- `internal/session/manager.go` - 后端消息处理
- `web/src/hooks/mutations/useSendMessage.ts` - 前端发送逻辑
- `web/src/components/SessionChat.tsx` - 禁用状态传递
- `web/src/components/AssistantChat/HappyComposer.tsx` - 输入框组件
