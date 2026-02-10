# 部署指南

## 服务器部署 (仅 CLI)

### 本地构建

```bash
cd proxy && go build -o agentgear ./cmd/agentgear
```

### Docker 构建

```bash
docker build -t agentgear:latest ./proxy
```

### Docker Compose

```bash
docker-compose up -d
```

详细的 Docker 部署说明参见项目根目录的 [README.Docker.md](../../README.Docker.md)。

## 桌面使用 (GUI + CLI)

GUI 可连接本地或远程 CLI 服务进行管理。

### 本地模式
GUI 启动时自动连接本地 CLI 服务（默认端口 9001）。

### 远程模式
GUI 可配置连接远程 CLI 服务地址，用于管理远程部署的 AgentGear 实例。

## 端口说明

| 端口 | 用途 |
|------|------|
| 9000 | 透明代理端口，接收 Agent API 请求 |
| 9001 | 内部 API 端口，管理和监控接口 |
