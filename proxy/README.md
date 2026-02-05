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

## API

### 代理端点 (9000)
- `GET /health` - 健康检查
- `ANY /v1/*` - 代理到上游

### 管理端点 (9001)
- `GET /api/connections` - 连接列表
- `GET /api/stats` - 统计信息
- `GET /api/transformers` - 转换器配置
- `WS /api/ws` - WebSocket 实时推送
