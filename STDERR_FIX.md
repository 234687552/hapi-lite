# Claude CLI Stderr 捕获修复

## 问题描述

**现象**：
- 前端发送消息后没有任何反应
- 后端日志显示 200 成功，但 stderr 输出错误：`Error: Session ID xxx is already in use.`
- 用户无法看到错误信息，不知道发生了什么

**根本原因**：
- Claude CLI 的错误信息输出到 stderr
- 原代码将 stderr 直接输出到服务器控制台（`cmd.Stderr = os.Stderr`）
- 前端无法接收到这些错误信息

## 修复方案

### 修改文件
`internal/session/manager.go` 中的 `runClaude` 函数

### 修改内容

**修改前**：
```go
func (m *Manager) runClaude(cmd *exec.Cmd, sessionID string, proc *AgentProcess) {
    stdout, err := cmd.StdoutPipe()
    if err != nil {
        fmt.Fprintf(os.Stderr, "stdout pipe error: %v\n", err)
        return
    }
    cmd.Stderr = os.Stderr  // ❌ 错误直接输出到控制台

    // ... 处理 stdout
}
```

**修改后**：
```go
func (m *Manager) runClaude(cmd *exec.Cmd, sessionID string, proc *AgentProcess) {
    stdout, err := cmd.StdoutPipe()
    if err != nil {
        fmt.Fprintf(os.Stderr, "stdout pipe error: %v\n", err)
        m.emitMessage(sessionID, proc, buildAssistantTextMessage(...), ...)
        return
    }

    // ✅ 捕获 stderr
    stderr, err := cmd.StderrPipe()
    if err != nil {
        fmt.Fprintf(os.Stderr, "stderr pipe error: %v\n", err)
        m.emitMessage(sessionID, proc, buildAssistantTextMessage(...), ...)
        return
    }

    // ✅ 在 goroutine 中收集 stderr
    var stderrBuf strings.Builder
    go func() {
        scanner := bufio.NewScanner(stderr)
        for scanner.Scan() {
            line := scanner.Text()
            fmt.Fprintf(os.Stderr, "Claude stderr: %s\n", line)
            stderrBuf.WriteString(line)
            stderrBuf.WriteString("\n")
        }
    }()

    // ... 处理 stdout

    cmd.Wait()

    // ✅ 将 stderr 内容发送给前端
    if stderrOutput := strings.TrimSpace(stderrBuf.String()); stderrOutput != "" {
        m.emitMessage(sessionID, proc, buildAssistantTextMessage(
            fmt.Sprintf("⚠️ Claude Error:\n%s", stderrOutput)
        ), time.Now().UnixMilli())
    }
}
```

## 修复效果

### 修复前
- 用户发送消息 → 前端无反应
- 后端日志：`Error: Session ID xxx is already in use.`
- 用户体验：❌ 不知道发生了什么

### 修复后
- 用户发送消息 → 前端显示错误消息
- 前端显示：`⚠️ Claude Error: Session ID xxx is already in use.`
- 用户体验：✅ 清楚知道问题所在

## 常见错误及解决方案

### 1. Session ID already in use
**原因**：Claude CLI 检测到该 session 正在被其他进程使用

**解决方案**：
```bash
# 检查 Claude 进程
ps aux | grep claude

# 杀掉所有 Claude 进程
pkill -9 claude

# 刷新页面，创建新 session
```

### 2. Authentication failed
**原因**：Claude CLI 认证失效

**解决方案**：
```bash
# 重新登录
claude auth login
```

### 3. Rate limit exceeded
**原因**：API 调用频率超限

**解决方案**：等待一段时间后重试

## 测试验证

1. 启动服务器
2. 创建一个 session
3. 在终端手动运行相同的 Claude 命令（模拟冲突）
4. 在前端发送消息
5. 验证前端是否显示错误信息

## 注意事项

- stderr 内容会同时输出到服务器日志和前端
- 错误消息以 `⚠️ Claude Error:` 开头，便于识别
- 不影响正常的消息流处理
