# AgentGear GUI

跨平台图形界面，用于管理和监控 AgentGear 代理。

## 技术栈

- **前端**: React 19 + TypeScript + Tailwind CSS
- **桌面框架**: Tauri 2.x (Rust)
- **状态管理**: Zustand

## 开发

```bash
# 安装依赖
npm install

# 开发模式
npm run tauri:dev

# 构建
npm run tauri:build
```

## 功能

- 连接监控：实时查看代理连接
- 请求检查：查看请求/响应详情
- 转换器管理：配置工具转换规则
- 标签规则：配置请求标签匹配

## 连接代理

GUI 通过 HTTP/WebSocket 连接到 AgentGear 代理的内部 API (默认端口 9001)。

支持连接本地或远程代理服务。
