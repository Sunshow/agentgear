# Replay 工具使用说明

## 概述

`agentgear replay` 是一个命令行工具，用于重放已记录的 session 日志到指定的目标服务器。主要用于：

- **问题复现**：使用生产环境的真实请求在测试环境重现问题
- **性能测试**：批量重放请求测试服务器性能
- **端点测试**：通过覆盖 path 参数测试不同的 gateway 或 upstream
- **转换器验证**：对比原始请求和转换后请求的行为差异

## 基本用法

```bash
agentgear replay -s <server_url> -d <log_directory> [options]
```

### 必需参数

| 参数 | 简写 | 说明 | 示例 |
|------|------|------|------|
| `--server` | `-s` | 目标服务器 URL | `http://127.0.0.1:9000` |
| `--dir` | `-d` | session 日志目录路径 | `./logs/sessions` |

### 可选参数

| 参数 | 简写 | 默认值 | 说明 |
|------|------|--------|------|
| `--path` | `-p` | - | 覆盖请求路径（用于测试不同端点） |
| `--transformed` | - | `false` | 使用转换后的请求体（`_request_transformed.body`） |
| `--header` | `-H` | - | 额外/覆盖的请求头，格式 `Key:Value`，可多次指定 |
| `--timeout` | `-t` | `600` | 请求超时秒数 |
| `--quiet` | `-q` | `false` | 安静模式，只输出摘要不输出响应体 |
| `--seq` | - | - | 只重放指定序号的请求，逗号分隔（如 `1,3,5`） |

## Session 日志结构

每个 session 目录包含以下文件：

```
20260212-230152_5e0b909d/
├── 001_request.json              # 请求元数据（method, path, headers）
├── 001_request.body              # 原始请求体
├── 001_request_transformed.body  # 转换后的请求体（如果有转换）
├── 001_response.body             # 原始响应体
└── 001_response_transformed.body # 转换后的响应体（如果有转换）
```

- 目录名格式：`YYYYMMDD-HHMMSS_<short_uuid>`
- 文件前缀 `NNN` 为序号（001, 002, ...），支持多轮对话

## 使用场景

### 1. 重放单个 session

```bash
agentgear replay \
  -s http://127.0.0.1:9000 \
  -d ./logs/sessions/20260212-230152_5e0b909d
```

### 2. 批量重放所有 sessions

```bash
agentgear replay \
  -s http://127.0.0.1:9000 \
  -d ./logs/sessions
```

自动扫描 `./logs/sessions` 下的所有 session 子目录，按时间戳顺序重放。

### 3. 测试转换器效果

对比原始请求和转换后请求的行为差异：

```bash
# 重放原始请求
agentgear replay -s http://127.0.0.1:9000 -d ./logs/sessions/xxx

# 重放转换后的请求
agentgear replay -s http://127.0.0.1:9000 -d ./logs/sessions/xxx --transformed
```

### 4. 测试不同 gateway

通过 `-p` 参数覆盖请求路径，将同一请求发送到不同的 gateway：

```bash
# 原始请求发往 /timi/v1/messages
# 覆盖为 /warp_us10/v1/messages 测试不同 upstream
agentgear replay \
  -s http://127.0.0.1:9000 \
  -d ./logs/sessions/xxx \
  -p /warp_us10/v1/messages
```

### 5. 补充敏感头信息

日志中的敏感头（如 `X-Api-Key`）会被脱敏为 `[REDACTED]`，重放时需要补充：

```bash
agentgear replay \
  -s http://127.0.0.1:9000 \
  -d ./logs/sessions \
  -H "X-Api-Key:sk-ant-xxx" \
  -H "Authorization:Bearer token"
```

### 6. 只重放特定序号的请求

```bash
# 只重放第 1 和第 3 个请求
agentgear replay \
  -s http://127.0.0.1:9000 \
  -d ./logs/sessions \
  --seq 1,3
```

### 7. 安静模式（性能测试）

批量重放时不输出响应体，只看统计信息：

```bash
agentgear replay \
  -s http://127.0.0.1:9000 \
  -d ./logs/sessions \
  -q
```

## 输出格式

### 正常模式

```
=== Session: 20260212-230152_5e0b909d ===
[001] POST /timi/v1/messages
  → Status: 200, Duration: 5388ms
  → Response: 3096 bytes (streaming)
  → First event: message_start

event: message_start
data: {"type":"message_start",...}
...

=== Session: 20260212-230158_d14e908e ===
[001] POST /timi/v1/messages
  → Status: 200, Duration: 2100ms
  → Response: 1024 bytes

{"id":"msg_xxx","content":[...]}

=== Summary ===
Total: 2 requests, 2 success, 0 failed
```

### 安静模式（`-q`）

```
=== Session: 20260212-230152_5e0b909d ===
[001] POST /timi/v1/messages
  → Status: 200, Duration: 5388ms
  → Response: 3096 bytes (streaming), First event: message_start

=== Summary ===
Total: 2 requests, 2 success, 0 failed
```

### 错误输出

```
[001] POST /timi/v1/messages
  → Error: send request: dial tcp 127.0.0.1:9000: connect: connection refused
```

## 流式响应处理

replay 工具自动检测 SSE 流式响应（`Content-Type: text/event-stream`）：

- 逐行读取并输出 `event:` 和 `data:` 行
- 统计总字节数和首个事件类型
- 支持大型 SSE 数据（缓冲区 1MB）

## 注意事项

### 1. 敏感信息

- 日志中的 `Authorization`、`X-Api-Key`、`Anthropic-Api-Key` 等敏感头会被脱敏
- 重放时需通过 `-H` 参数手动补充
- 请求体和响应体**不会**脱敏，注意保护日志文件

### 2. 会话状态

- replay 工具是无状态的，每个请求独立发送
- 不会自动传递 `X-Session-Id` 或 cookie
- 多轮对话的上下文依赖请求体中的 `messages` 数组

### 3. 超时设置

- 默认超时 600 秒，适合长时间流式响应
- 可通过 `-t` 调整，如 `-t 30` 设置 30 秒超时

### 4. 错误处理

- 单个请求失败不会中断批量重放
- 错误会记录在输出中，最终统计成功/失败数量

## 典型工作流

### 问题排查

1. 在生产环境遇到问题，查看 GUI 连接监控找到对应 session
2. 复制 session 目录到测试环境
3. 重放请求复现问题：
   ```bash
   agentgear replay -s http://test-server:9000 -d ./session_xxx -H "X-Api-Key:test-key"
   ```
4. 对比原始请求和转换后请求的差异：
   ```bash
   agentgear replay -s http://test-server:9000 -d ./session_xxx --transformed
   ```

### 转换器开发

1. 修改转换器配置
2. 使用历史 session 验证转换效果：
   ```bash
   agentgear replay -s http://127.0.0.1:9000 -d ./logs/sessions --transformed -q
   ```
3. 检查输出中的状态码和错误信息

### 性能测试

1. 收集一批真实请求的 session 日志
2. 批量重放测试服务器性能：
   ```bash
   time agentgear replay -s http://perf-test:9000 -d ./logs/sessions -q
   ```
3. 分析 Duration 统计和总耗时

## 与其他工具集成

### 与 GUI 配合

1. GUI 实时监控连接，发现异常
2. 记录 session ID，从 `logs/sessions/` 找到对应目录
3. 使用 replay 工具在测试环境复现

### 与 CI/CD 集成

```bash
#!/bin/bash
# 回归测试脚本
set -e

# 启动测试服务器
./agentgear serve -c test-config.yaml &
SERVER_PID=$!
sleep 2

# 重放测试用例
agentgear replay -s http://127.0.0.1:9000 -d ./testdata/sessions -q

# 清理
kill $SERVER_PID
```

## 限制

- 不支持修改请求体内容（只能选择原始或转换后）
- 不支持并发重放（按顺序串行发送）
- 不支持自动重试或速率限制
- 不支持 WebSocket 或 HTTP/2 推送

## 相关文档

- [内部 API](./internal-api.md) - 查看实时连接信息
- [Streaming](../core-features/streaming.md) - 流式响应处理机制
- [Transformers](../core-features/transformers.md) - 转换器配置
