# AgentGear Docker 使用指南

## 快速开始

### 构建镜像

```bash
docker build -t agentgear:latest ./proxy
```

### 运行容器

```bash
docker run -d \
  --name agentgear \
  -p 9000:9000 \
  -p 9001:9001 \
  -v $(pwd)/proxy/configs/config.yaml:/app/configs/config.yaml:ro \
  -v $(pwd)/logs:/app/logs \
  agentgear:latest
```

### 使用 Docker Compose

```bash
docker-compose up -d
```

## 环境变量配置

所有配置项都支持通过环境变量设置，优先级：**环境变量 > 配置文件 > 默认值**

| 环境变量 | 说明 | 默认值 |
|---------|------|--------|
| `AGENTGEAR_SERVER_PORT` | 代理服务监听端口 | 9000 |
| `AGENTGEAR_SERVER_HOST` | 代理服务监听地址 | 0.0.0.0 |
| `AGENTGEAR_SERVER_API_PORT` | 内部 API 端口 | 9001 |
| `AGENTGEAR_SERVER_API_HOST` | 内部 API 地址 | 0.0.0.0 |
| `AGENTGEAR_LOGGING_ENABLED` | 是否启用日志 | true |
| `AGENTGEAR_LOGGING_DIR` | 日志目录 | /app/logs |
| `AGENTGEAR_LOGGING_MAX_SIZE` | 单个日志文件最大大小（MB） | 100 |
| `AGENTGEAR_LOGGING_MAX_BACKUPS` | 保留的日志文件数 | 10 |
| `AGENTGEAR_LOGGING_MAX_AGE` | 日志保留天数 | 30 |
| `AGENTGEAR_MEMORY_MAX_CONNECTIONS` | 内存中保留的最大连接数 | 1000 |
| `AGENTGEAR_MEMORY_RETENTION_MINUTES` | 连接保留时间（分钟） | 60 |

> 注意：网关 (gateways) 配置需要通过配置文件设置，不支持环境变量。

## 配置示例

### 生产环境配置

```bash
docker run -d \
  --name agentgear-prod \
  -p 9000:9000 \
  -p 9001:9001 \
  -e AGENTGEAR_LOGGING_ENABLED=false \
  -e AGENTGEAR_MEMORY_MAX_CONNECTIONS=2000 \
  -v /data/agentgear/configs/config.yaml:/app/configs/config.yaml:ro \
  -v /data/agentgear/logs:/app/logs \
  --restart always \
  agentgear:latest
```

### 开发环境配置

```bash
docker run -d \
  --name agentgear-dev \
  -p 9000:9000 \
  -p 9001:9001 \
  -e AGENTGEAR_LOGGING_ENABLED=true \
  -v $(pwd)/proxy/configs/config.yaml:/app/configs/config.yaml:ro \
  -v $(pwd)/logs:/app/logs \
  agentgear:latest
```

## 健康检查

容器提供健康检查端点：

```bash
curl http://localhost:9000/health
```

返回：
```json
{"status":"ok"}
```

## 日志查看

```bash
# 查看容器日志
docker logs -f agentgear

# 查看会话日志
docker exec agentgear ls -la /app/logs/sessions

# 复制日志到宿主机
docker cp agentgear:/app/logs ./logs
```

## 安全建议

1. **不要在生产环境直接暴露容器端口**，建议使用反向代理（Nginx/Traefik）
2. **定期清理日志文件**，避免磁盘空间耗尽
3. **使用 secrets 管理敏感信息**，不要在环境变量中直接写入 API Key

## 镜像信息

- 基础镜像：`alpine:3.19`
- Go 版本：1.25+
- 镜像大小：~41MB
- 运行用户：agentgear (UID 1000)
- 工作目录：`/app`
- 暴露端口：9000 (代理), 9001 (内部 API)

## 故障排查

### 无法连接上游 API

```bash
docker exec agentgear wget -O- https://api.anthropic.com/v1/messages
```

### 查看实际使用的配置

```bash
docker exec agentgear printenv | grep AGENTGEAR
```

### 重启容器

```bash
docker restart agentgear
```
