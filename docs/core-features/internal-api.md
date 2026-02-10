# 内部 API

AgentGear 内部 API 运行在端口 9001，提供管理和监控接口。

## 接口列表

### 连接管理

```
GET  /api/connections              # 连接列表
GET  /api/connections/:id          # 连接详情
DELETE /api/connections            # 清空连接
```

### 标签系统

```
GET  /api/tags                     # 标签统计
GET  /api/tagging/rules            # 标签规则
POST /api/tagging/test             # 测试标签匹配
```

### 转换器管理

```
GET  /api/transformers             # 转换器列表
PUT  /api/transformers/:name       # 更新转换器
```

### 系统信息

```
GET  /api/stats                    # 统计信息
WS   /api/ws                       # WebSocket 实时推送
```

## WebSocket 实时推送

通过 `/api/ws` 建立 WebSocket 连接后，可实时接收：
- 新连接建立通知
- 连接状态变更
- 请求/响应数据更新
