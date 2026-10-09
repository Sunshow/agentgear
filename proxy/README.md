# AgentGear Proxy

Go CLI 代理模块，可独立部署运行。

## 构建

```bash
go build -o agentgear ./cmd/agentgear/
```

## 运行

```bash
# 使用默认配置
./agentgear

# 指定配置文件
./agentgear --config ./configs/config.yaml
```

## Docker

```bash
# 构建镜像
docker build -t agentgear:latest .

# 运行
docker run -d -p 9000:9000 -p 9001:9001 agentgear:latest
```

## 端口

- `9000` - 代理端口，转发 API 请求
- `9001` - 内部 API 端口，管理和监控

## 配置

参考 `configs/config.example.yaml`

## HTTP 传输压缩

- 支持 `Content-Encoding: gzip` 和 `Content-Encoding: zstd`，也支持按声明顺序叠加的编码。
- 请求体先解压，再进行标签匹配、JSON 解析和转换，并以解压后的正文转发到上游。
- 默认向上游发送 `Accept-Encoding: gzip, zstd`；普通响应、错误响应和 SSE 流式响应均在解压后处理。SSE 使用增量解压，保持实时输出。
- 解压后的正文用于连接详情和 session 日志，转发时移除压缩头和旧的 `Content-Length`。
- 损坏的压缩请求返回 `400`，不支持的请求编码返回 `415`；上游解压失败且尚未开始输出响应时返回 `502`。

## API

### 代理端点 (9000)
- `GET /health` - 健康检查
- `ANY /v1/*` - 代理到上游

### 管理端点 (9001)
- `GET /api/connections` - 连接列表
- `GET /api/stats` - 统计信息
- `GET /api/transformers` - 转换器配置
- `WS /api/ws` - WebSocket 实时推送
