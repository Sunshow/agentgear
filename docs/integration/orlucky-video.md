# new-api 视频渠道 → orlucky 适配分析（修正版）

> 场景：使用方调用 `https://ai.cloudtriple.tech`（外层 new-api）的 `/v1/videos`，
> 外层 new-api 的渠道类型是 **New API**，绑定 **doubao 插件**，上游指向 AgentGear
> （本次要模拟的“另一个 new-api”）。
>
> 本文结论基于：
> - new-api 源码 `/Users/sunshow/GIT/github/new-api`（`plugins/tasks/doubao/plugin.js`、
>   `plugins/tasks/sora/plugin.js`、`pkg/jsplugin/routing.go`、`controller/video_proxy.go`）；
> - orlucky 官网前端 bundle、公开模型目录、`/docs` 页面；
> - 使用方给出的三条 curl（外层 new-api 的客户端视角）。
>
> 状态：P1/P2 已实现（doubao/Ark create/get/content + 单元测试 + 本地 e2e），
> 真实 orlucky 与真实外层 new-api 联调待做。

---

## 0. 关键修正

上一版分析假设 AgentGear 直接接收 `/v1/videos`。**在“New API 渠道 + doubao 插件”下并非如此。**

new-api 的 doubao 插件声明了 `upstreams: ["vendor", "new_api"]`。当渠道是 New API 类型时：

```js
// plugins/tasks/doubao/plugin.js
function apiRoot(ctx) {
  return ctx.baseUrl + (ctx.upstream && ctx.upstream.kind === "new_api" ? "/doubao" : "");
}
```

外层 new-api 会调用 AgentGear 的 **doubao/Ark 原生路径**：

| 客户端（使用方） | 外层 new-api 转发给 AgentGear |
|------------------|-------------------------------|
| `POST /v1/videos` | `POST {base}/doubao/api/v3/contents/generations/tasks` |
| `GET /v1/videos/{task_id}` | `GET {base}/doubao/api/v3/contents/generations/tasks/{upstream_id}` |
| `GET /v1/videos/{task_id}/content` | **不发给 AgentGear**：外层从查询响应里的 `content.video_url` 直接下载 |

结论：

1. AgentGear 的首要入站协议是 **doubao/Ark 任务协议**（2 个端点），不是 `/v1/videos`。
2. 下载默认不经过 AgentGear；只有当查询响应里的 `content.video_url` 指向 AgentGear 时，
   才会出现第三个请求（可选实现）。
3. `/v1/videos` 是外层 new-api 的**客户端协议**，由 doubao 插件的 `openai_video` 宿主协议
   负责呈现，AgentGear 不需要实现它（除非要做直连兼容）。

### 0.1 为什么是 `/doubao`，而不是 `/v1/videos`

new-api 同时暴露两类路径：

1. **宿主协议（host protocol）**：`/v1/videos`、`/v1/responses`、`/v1/images/...`
   由 `router/task-plugin-protocol-router.go` **无条件注册**。
2. **插件原生路由（native routes）**：每个已安装插件声明的厂商路径，
   以及（除非路径已在插件自己的前缀下）自动生成的 `/{key}` 副本
   （`pkg/jsplugin/routing.go` 的 `PluginRouteBindings`）。

因此一个装了 doubao 插件的 new-api 同时有：

- `POST /v1/videos`（宿主协议，客户端用）
- `POST /api/v3/contents/generations/tasks`（插件声明的 Ark 原生路径）
- `POST /doubao/api/v3/contents/generations/tasks`（自动生成的 `/{key}` 副本）

调用方 `/v1/videos` 能工作，是因为插件声明了 `openai_video` 并覆盖该 model；
装插件影响的是**宿主协议的模型路由**，不是把 `/v1/videos` 改写成别的路径。

上游路径则由**执行插件的驱动代码**决定。type 60（New API 渠道）时：

```go
// relay/channel/task/jsplugin/adaptor.go
if channelType == constant.ChannelTypeNewAPI {
    ctx["upstream"] = map[string]any{"kind": pluginruntime.UpstreamKindNewAPI}
}
```

各插件据此给自己的原生路径加前缀：

```js
// doubao
return ctx.baseUrl + (ctx.upstream?.kind === "new_api" ? "/doubao" : "");
// alibaba
return ctx.baseUrl + (viaNewAPI(ctx) ? "/ali" : "");
```

所以 wan3（Alibaba Bailian 插件 + New API 渠道）的上游实际是
`{base}/ali/api/v1/services/aigc/...`，不是 `/v1/videos`；A 能直接接
`/v1/videos` 是因为它同时暴露了宿主协议，二者并不矛盾。

| 渠道/插件 | 客户端路径 | 上游路径（type 60） |
|-----------|-----------|---------------------|
| New API + doubao | `/v1/videos` | `{base}/doubao/api/v3/contents/generations/tasks` |
| New API + Alibaba | `/v1/videos` | `{base}/ali/api/v1/services/aigc/...` |
| OpenAI(type 1) + sora | `/v1/videos` | `{base}/v1/videos` |

> 若希望 AgentGear 也能像 A 一样直接接 `/v1/videos`，需要再实现一组宿主协议
> 路由（create/retrieve/content）；这与本渠道实际走的 `/doubao` 路径是两套入口，
> 可共用同一份任务存储与 orlucky client。

---

## 1. 端到端链路

```text
使用方
  │  POST /v1/videos {model, prompt, seconds, size}
  ▼
外层 new-api (ai.cloudtriple.tech)
  │  - TokenAuth / 选渠道
  │  - doubao 插件 openai_video.decodeRequest
  │  - buildSubmitRequest：ctx.upstream.kind == "new_api" → 加 /doubao 前缀
  ▼
AgentGear（本次实现）
  │  POST /doubao/api/v3/contents/generations/tasks
  │  GET  /doubao/api/v3/contents/generations/tasks/{id}
  ▼
orlucky 任务 API
     POST /v1/tasks
     GET  /v1/tasks/{taskId}
```

下载链路（默认）：

```text
使用方 GET /v1/videos/{id}/content
  → 外层 new-api VideoProxy
  → 读取 task.data.content.video_url（AgentGear 查询响应里给的）
  → credentialless GET 该 URL（通常是 orlucky CDN）
```

---

## 2. 外层 new-api 发给 AgentGear 的请求（doubao/Ark 协议）

### 2.1 创建

```http
POST {base}/doubao/api/v3/contents/generations/tasks
Authorization: Bearer <外层渠道 key>
Content-Type: application/json
```

请求体（由 doubao 插件的 `buildSubmitRequest` 生成）：

```json
{
  "model": "doubao-seedance-2-0-mini-260615",
  "content": [
    { "type": "text", "text": "一只橘猫在阳光下钓路亚，坐着飞艇" }
  ],
  "duration": 5
}
```

图生视频/参考素材：

```json
{
  "model": "doubao-seedance-2-0-mini-260615",
  "content": [
    { "type": "image_url", "image_url": { "url": "https://.../ref.png" } },
    { "type": "video_url", "video_url": { "url": "https://.../ref.mp4" } },
    { "type": "text", "text": "..." }
  ],
  "duration": 5,
  "resolution": "720p",
  "ratio": "16:9"
}
```

字段来源说明：

| 字段 | 来源 |
|------|------|
| `model` | 客户端 model（经外层 `model_mapping` 后的 upstream model） |
| `content` | 客户端 prompt / `metadata.content` / 参考素材 |
| `duration` | 客户端 `seconds`（或 `duration`）；可为 `-1` |
| `resolution`、`ratio`、`seed`、`watermark`、`generate_audio` 等 | 客户端 `metadata` 的同名键，原样透传 |
| `size` | **不会出现在这里**：doubao 插件 `buildSubmitRequest` 只透传 `metadata`，不转发顶层 `size` |

> 客户端的 `size: "1280*720"` 只参与外层 new-api 的**预估计费**，不会传给 AgentGear。
> 想控制输出比例/清晰度，客户端应改用 `metadata: { "ratio": "16:9", "resolution": "720p" }`。
>
> 图生视频同理：`input_reference`（字符串或文件）不会被 doubao 插件放进 `content`。
> 客户端应传 `metadata: { "content": [{ "type": "image_url", "image_url": { "url": "https://..." } }] }`
> （或顶层 `images: ["https://..."]`）。文件形态会被 doubao 插件直接拒绝。

> **渠道 Base URL 配置**：`gateways[].path` 是 AgentGear 的本地挂载点（例如 `/orlucky`）。
> 外层 new-api 渠道的 Base URL 填 `{origin}{gw.Path}`（例如 `http://agentgear:9000/orlucky`），
> **不要**自己再加 `/doubao`；doubao 插件会自动追加。
> 最终请求是 `http://agentgear:9000/orlucky/doubao/api/v3/contents/generations/tasks`。
>
> 也就是说：`gw.Path` 是 AgentGear 的挂载点，`/doubao` 是 doubao 插件的协议前缀，
> adapter 会在挂载点之后匹配 `/doubao/api/v3/...`（同时兼容不带 `/doubao` 的 vendor 形式）。

响应（最小必需）：

```json
{ "id": "task_xxx" }
```

`parseSubmitResponse` 只做 `if (!resp.body || !resp.body.id) throw ...`，然后把这个 `id`
作为后续查询的 `taskId`。

### 2.2 查询

```http
GET {base}/doubao/api/v3/contents/generations/tasks/{task_id}
Authorization: Bearer <外层渠道 key>
```

响应必须让 `parseTaskResult` 能识别：

```json
{
  "id": "task_xxx",
  "model": "doubao-seedance-2-0-mini-260615",
  "status": "succeeded",
  "content": {
    "video_url": "https://cdn.../out.mp4",
    "resolution": "720p"
  },
  "usage": { "completion_tokens": 12345, "total_tokens": 12345 }
}
```

状态取值（`parseTaskResult`）：

| AgentGear 返回 | 外层 new-api 结果 |
|----------------|-------------------|
| `queued` | `QUEUED`，progress 10% |
| `running` | `IN_PROGRESS`，progress 50% |
| `succeeded` | `SUCCESS`，进度 100%，`content.video_url` 作为结果 URL |
| `failed` / `expired` / `cancelled` | `FAILURE`，`error.message` 作为失败原因 |
| 其他 | `UNKNOWN` |

失败响应：

```json
{
  "id": "task_xxx",
  "status": "failed",
  "error": { "code": "video_generation_failed", "message": "上游返回的原因" }
}
```

> `extractUsageOnComplete` 还会读取 `body.content.resolution || body.resolution`，
> 用于完成时校正分辨率计费；建议固定回传实际请求的 `resolution`（如 `720p`）。

### 2.3 下载

doubao 插件的 `buildContentRequest`：

```js
const data = artifactData(ctx);
url = data.content.video_url;              // 或 last_frame_url
return { url, method: ctx.clientRequest.method, credentialless: true };
```

外层 `VideoProxy` 会用 `credentialless` 方式直接 GET 这个 URL。
所以：

- 默认方案：AgentGear 在查询响应里返回 orlucky 的素材 URL（公网可访问即可），
  下载不经过 AgentGear；
- 代理方案：若配置了 `public_base_url`，返回
  `{public_base_url}/doubao/api/v3/contents/generations/tasks/{id}/content`，
  AgentGear 再多实现一个流式下载端点（透传 Range/HEAD）。

### 2.4 图片/文件

doubao 插件的 `protocols.openai_video.decodeRequest` 明确拒绝 multipart 文件：

```js
if ((ctx.body.files || []).length)
  throw new Error("Doubao requires image and video references to be URLs inside metadata.content");
```

因此：**这条链路没有 multipart 透传问题，参考图/视频必须是 URL**。
使用方如果给外层 `/v1/videos` 传 `input_reference` 文件，会在外层就被拒绝。

---

## 3. orlucky 任务 API

### 3.1 根地址与鉴权

- 站点 `https://orluckyai.com/docs` 是 SPA；直接访问 `orluckyai.com/v1/tasks` 返回 HTML。
- 真实接口根：**`https://router.orlucky.com/model/api`**。
- 鉴权：**`X-API-Key: <key>`**。
- 幂等：`Idempotency-Key`（创建时）。

用户已确认：**API Key 从入站 `Authorization: Bearer` 透传**，即外层 new-api 的渠道 key
就是 orlucky key；AgentGear 把它改写成 `X-API-Key`。

### 3.2 创建

```http
POST https://router.orlucky.com/model/api/v1/tasks
X-API-Key: <透传的 key>
Idempotency-Key: <uuid>
Content-Type: application/json
```

```json
{
  "category": "VIDEO",
  "operation": "TEXT_TO_VIDEO",
  "model": "seedance-mini",
  "prompt": "一只橘猫在阳光下钓路亚，坐着飞艇",
  "parameters": {
    "duration": 5,
    "resolution": "720p",
    "aspectRatio": "16:9"
  },
  "media": []
}
```

响应：

```json
{
  "success": true,
  "data": {
    "taskId": "tsk_xxx",
    "state": "PENDING",
    "category": "VIDEO",
    "data": null,
    "error": null
  },
  "traceId": "..."
}
```

### 3.3 查询

```http
GET https://router.orlucky.com/model/api/v1/tasks/{taskId}
X-API-Key: <透传的 key>
```

成功后：

```json
{
  "success": true,
  "data": {
    "taskId": "tsk_xxx",
    "state": "SUCCEEDED",
    "data": {
      "text": null,
      "assets": [
        {
          "type": "VIDEO",
          "url": "https://cdn.../out.mp4",
          "downloadUrl": "https://cdn.../out.mp4"
        }
      ]
    },
    "error": null
  }
}
```

取消：`POST /v1/tasks/{taskId}/cancel`（本期可不接）。

### 3.4 状态

`PENDING / RUNNING / SUCCEEDED / COMPLETED / FAILED / CANCELLED / EXPIRED / CLOSED / UNKNOWN`

终态：`SUCCEEDED`、`COMPLETED`、`FAILED`、`CANCELLED`、`EXPIRED`、`CLOSED`。
orlucky **不提供进度百分比**；对外层 doubao 协议也不需要进度（插件自己映射 10%/50%）。

### 3.5 模型目录（公开，可无鉴权拉取）

```http
GET https://router.orlucky.com/model/api/models/public/catalog?categories=VIDEO&page=1&pageSize=100
```

本期只适配 Seedance（SD）系列，目录中的关键约束：

| orlucky 模型 | duration | resolution | aspectRatio | 参考素材 |
|--------------|----------|------------|-------------|----------|
| `seedance-mini` | 4–15，默认 5 | 480p/720p | 21:9/16:9/4:3/1:1/3:4/9:16/adaptive | image 9、video 3、audio 3 |
| `seedance-fast` | 4–15，默认 5 | 480p/720p | 同上（无 adaptive） | image 9、video 3、audio 3 |
| `seedance-2.0` | 4–15，默认 4 | 480p/720p/1080p/4k | 同上（无 adaptive） | image 9、video 3、audio 3 |
| `seedance-2.5` | 4–30，默认 5 | 480p/720p/1080p | 同上（含 adaptive） | image 30、video 10、audio 10 |

---

## 4. 映射设计

### 4.1 模型映射

| 外层 model（doubao 插件声明） | orlucky model |
|-------------------------------|---------------|
| `doubao-seedance-2-0-260128` | `seedance-2.0` |
| `doubao-seedance-2-0-fast-260128` | `seedance-fast` |
| `doubao-seedance-2-0-mini-260615` | `seedance-mini` |
| `doubao-seedance-2-5-260628` | `seedance-2.5` |
| `doubao-seedance-1-0-*` | 本期不支持（返回明确错误） |

建议用配置显式列出；未命中时可回退到名称解析（`2-0-mini` → `seedance-mini`），
但必须打日志，避免静默错配。

### 4.2 创建字段映射

| doubao/Ark 字段 | orlucky 字段 | 规则 |
|-----------------|--------------|------|
| `model` | `model` | 上表映射 |
| `content[type=text].text` | `prompt` | 多个 text 用 `\n` 拼接 |
| `content[type=image_url].image_url.url` | `media[]` | `{type:"IMAGE", url}` |
| `content[type=video_url].video_url.url` | `media[]` | `{type:"VIDEO", url}` |
| `content[type=audio_url].audio_url.url` | `media[]` | `{type:"AUDIO", url}` |
| `content[type=draft_task]` | — | 不支持，`400` |
| `duration` | `parameters.duration` | `-1` → `400`（orlucky 无此语义）；否则校验目标模型范围，越界 → `400` |
| `resolution` | `parameters.resolution` | 缺省 `720p`；`WxH` 归一为档位；裁剪到可选值 |
| `ratio` | `parameters.aspectRatio` | 缺省 `16:9`；不在可选值时取默认 |
| `frames` | `parameters.duration` | 无 duration 时 `round(frames/24)` |
| `seed` / `watermark` / `generate_audio` / `camera_fixed` | — | 目标模型 `optionRules` 没有，丢弃并记日志 |
| — | `category` | 固定 `"VIDEO"` |
| — | `operation` | 有视频参考 `VIDEO_TO_VIDEO`；有图片 `IMAGE_TO_VIDEO`；否则 `TEXT_TO_VIDEO` |

### 4.3 状态映射

| orlucky | doubao/Ark |
|---------|------------|
| `PENDING` | `queued` |
| `RUNNING` | `running` |
| `SUCCEEDED` / `COMPLETED` | `succeeded` |
| `FAILED` / `CLOSED` | `failed` |
| `EXPIRED` | `expired` |
| `CANCELLED` | `cancelled` |
| 其他 | `queued` + 日志 |

### 4.4 结果映射

```json
{
  "id": "<AgentGear 公共 id>",
  "model": "<外层原始 model>",
  "status": "succeeded",
  "content": {
    "video_url": "<orlucky assets[] 中 type/kind 为 video 的 url 或 downloadUrl>",
    "resolution": "<本次请求实际使用的 resolution>"
  },
  "usage": { "completion_tokens": N, "total_tokens": N }
}
```

- `usage`：orlucky 返回了才透传，否则省略（外层按 reservation 结算）。
- `content.resolution`：必须回传，外层用它校正完成计费。
- 失败时不要带 `content`，只带 `error`。

### 4.5 ID

对外生成 `task_` + 32 位随机字符（与 new-api 风格一致），持久化
`public_id → orlucky taskId`。原因：

- 外层直接把它拼进 URL，不能含特殊字符；
- 便于保留请求快照（resolution、model、created_at）用于查询响应；
- 便于将来切换上游或做内容代理。

### 4.6 错误映射

| 场景 | HTTP | 体 |
|------|------|----|
| 请求体非法 / 缺 model / content 非法 | 400 | `{error:{code:"invalid_request_error",message}}` |
| 模型未映射 | 400 | `model_not_supported` |
| 缺入站 Bearer | 401 | `unauthorized` |
| 公共 ID 不存在 | 404 | `task_not_found` |
| orlucky 4xx（`success:false`） | 400/402/403 透传 | `data.message` |
| orlucky 401/403 | 502 | `upstream_auth_failed`（不暴露 key） |
| orlucky 429 | 429 | 透传 `Retry-After` |
| orlucky 超时 | 504 | `upstream_timeout` |
| 其他 | 502 | `upstream_error` |

---

## 5. AgentGear 实现方案

### 5.1 模块

```text
proxy/internal/video/
  types.go        # Ark 请求/响应 DTO、orlucky 任务 DTO
  orlucky.go      # orlucky client：X-API-Key、信封、超时、Idempotency-Key
  catalog.go      # Seedance 模型约束、模型映射
  mapper.go       # create/query 的纯函数映射
  store.go        # bbolt：public id ↔ orlucky taskId + 请求快照
  handler.go      # Adapter 接口、NewAdapter 工厂、doubao 路由/处理
  mapper_test.go
  handler_test.go
```

`NewAdapter` 是协议扩展点：

```go
switch cfg.Protocol {
case "", "doubao": return newDoubaoHandler(cfg, logger)
// case "ali": return newAliHandler(cfg, logger)   // 后续实现
default: return nil, fmt.Errorf("video protocol %q is not implemented", cfg.Protocol)
}
```

### 5.2 路由（由 gateway 的 path 决定）

```text
{gw.Path}/doubao/api/v3/contents/generations/tasks             POST   create
{gw.Path}/doubao/api/v3/contents/generations/tasks/{id}        GET    get
{gw.Path}/doubao/api/v3/contents/generations/tasks/{id}/content  GET/HEAD  可选

{gw.Path}/api/v3/contents/generations/tasks/...                # vendor 形式，兼容
```

`gw.Path` 来自 `gateways[].path`（必填），是**本地挂载点**，例如 `/orlucky`；
`/doubao` 是 New API type-60 渠道下 doubao 插件追加的协议前缀。注册用
`r.Any(gw.Path+"/*path", adapter.Handle)`，避免和 `/v1/*path` 这类透明网关冲突；
handler 内部先剥掉可选的 `/doubao`，再匹配 `/api/v3/...`。

> 渠道 Base URL = `{origin}{gw.Path}`，例如 `http://agentgear:9000/orlucky`。
> 若前面还有反向代理子路径，`gw.Path` 要把子路径算进去，例如
> Base URL `http://host/upstream` → `gw.Path: /upstream/orlucky`。

### 5.3 配置：每个视频协议是一个 gateway

端点（path / upstream / timeout）沿用 `gateways`；`protocol` 非空时该 gateway
由协议适配器接管，而不是透明代理。所有视频 gateway 共享 `video` 下的全局设置。

```yaml
video:
  store_path: ./data/video_tasks.db
  task_ttl_hours: 72
  create_timeout_seconds: 300
  query_timeout_seconds: 30
  cache_ttl_ms: 1500
  # 默认上游 key；空 = 透传入站 Authorization Bearer
  api_key: ""
  # 默认内容代理根地址；空 = 直接返回上游素材 URL
  public_base_url: ""

gateways:
  - name: orlucky-doubao
    path: /orlucky          # 本地挂载点；渠道 Base URL = http://host:9000/orlucky
    upstream: https://router.orlucky.com/model/api
    protocol: doubao
    timeout: 300
    enabled: true
    video:
      # 可覆盖全局 api_key / public_base_url
      models:
        doubao-seedance-2-0-260128: seedance-2.0
        doubao-seedance-2-0-fast-260128: seedance-fast
        doubao-seedance-2-0-mini-260615: seedance-mini
        doubao-seedance-2-5-260628: seedance-2.5

  # 预留：Alibaba Bailian 协议。配置结构已就绪，ali adapter 实现后即可启用。
  - name: orlucky-ali
    path: /orlucky-ali      # 渠道 Base URL = http://host:9000/orlucky-ali
    upstream: https://router.orlucky.com/model/api
    protocol: ali
    timeout: 300
    enabled: false
    video:
      models:
        wan3.0-video: wan-3.0
```

鉴权：入站 `Authorization: Bearer <key>` 原样作为 orlucky `X-API-Key`；
不落日志、不返回给客户端。

与透明网关的区别：`protocol` 为空的 gateway 走原有 `ProxyRequest`（路径拼接、
transformer、session 日志）；`protocol` 非空的 gateway 由对应协议 handler 接管，
`path` 只作为本地前缀，`upstream` 是该协议的上游根地址。

### 5.4 存储

bbolt bucket `video_tasks`，key = `task_xxx`：

```go
type Task struct {
    PublicID     string
    UpstreamID   string
    ClientModel  string   // 外层 model
    TargetModel  string   // orlucky model
    Prompt       string
    Duration     int
    Resolution   string
    AspectRatio  string
    Status       string   // queued/running/succeeded/failed...
    CreatedAt    int64
    CompletedAt  int64
    AssetURL     string
    Usage        json.RawMessage
    LastSyncedAt int64
}
```

- 创建时写入；查询时更新；
- 终态后保留 `task_ttl_hours`；
- 查询加 1.5s 缓存 + 单飞，避免外层高频轮询打爆 orlucky。

### 5.5 创建时序

```text
1. 校验 Authorization（透传 key）
2. 解析 Ark JSON；校验 model / content
3. 模型映射（未命中 → 400）
4. content → prompt + media；duration/resolution/ratio 归一化并裁剪
5. POST /v1/tasks（X-API-Key + Idempotency-Key）
6. success=false → 错误映射
7. 生成 task_xxx，落库，返回 {"id": "task_xxx"}
```

### 5.6 查询时序

```text
1. 按公共 ID 查库（无 → 404）
2. 缓存命中且未过期 → 直接返回
3. GET /v1/tasks/{upstreamId}
4. 状态映射；成功时提取 assets 里的 video url
5. 更新库；返回 {id, model, status, content{video_url,resolution}, usage?}
```

### 5.7 可选下载端点

当 `public_base_url` 非空时：

```text
GET/HEAD {gw.Path}/doubao/api/v3/contents/generations/tasks/{id}/content
  → 查库 → 取 AssetURL → 透传 Range/HEAD → 流式返回
```

`public_base_url` 填**origin**（如 `http://agentgear:9000`），adapter 会拼上
`gw.Path + /doubao + /api/v3/...` 生成最终内容 URL。

外层 new-api 以 `credentialless` 方式访问，因此该端点不能要求鉴权，
依赖 `task_` 随机 ID 的不可猜测性；也可以额外加 IP 白名单（部署层）。

### 5.8 日志与测试

日志：

- `video_create`：model 映射、duration/resolution、orlucky taskId、耗时
- `video_query`：状态迁移、asset 是否就绪
- `video_error`：上游状态码、traceId（不记录 key）

测试：

1. mapper：content 解析、模型映射、duration/resolution 裁剪、状态映射
2. handler：假 orlucky（httptest）跑 create/query、错误分支、缓存
3. e2e：本地起 agentgear + 假 orlucky，用 doubao 风格 curl 走通
4. （可选）真实 orlucky 联调：`doubao-seedance-2-0-mini-260615` 生成 5s 视频

---

## 6. 风险与待确认

1. **确认外层渠道配置**：必须是 type 60 “New API” + doubao 插件，且渠道 key 是
   orlucky 的 API Key；否则鉴权透传会 401。
2. **模型范围**：本期只做 4 个 Seedance 模型；`doubao-seedance-1-0-*` 先明确报错。
3. **下载是否走 AgentGear**：默认直连 orlucky CDN；若 CDN 需要鉴权/过期，需配置
   `public_base_url` 启用内容代理。
4. **计费**：外层按 token/表达式计费；orlucky 不一定返回 `usage`，缺失时只能按预留额度结算。
   需要确认是否需要 AgentGear 估算 `usage`（不建议伪造）。
5. **`size` 丢失**：外层 doubao 插件不转发顶层 `size`；比例/清晰度必须走 `metadata`。
6. **无进度**：orlucky 只有状态，插件固定映射 10%/50%，不影响协议。
7. **orlucky 素材 URL 可达性**：外层 new-api 的服务器必须能访问该 URL。
8. **`duration=-1`**：Ark 表示由模型决定；orlucky 无此语义。建议直接 `400`，
   避免外层按最长时长预留、实际生成默认时长造成计费偏差。
9. **时长越界**：Ark 允许 1–12，orlucky Seedance 最小 4。建议越界直接 `400`
   （而不是静默裁剪），否则外层按请求时长计费、实际生成时长不同。
10. **图片入口**：客户端必须用 URL（`metadata.content` 或 `images`）；multipart 文件
    在外层 doubao 插件就会被拒绝，AgentGear 收不到。

---

## 7. 分阶段计划

| 阶段 | 内容 | 验收 |
|------|------|------|
| P0 | 确认第 6 节 1/3/4；锁定模型映射 | 配置评审通过 |
| P1 | Ark DTO + orlucky client + store + create/get | ✅ 假上游跑通创建/查询 |
| P2 | 模型约束校验、状态/错误映射、缓存、日志 | ✅ 单测 + 本地 e2e |
| P3 | 可选内容代理、真实 orlucky 联调、外层 new-api 联调 | 待做：使用方三条 curl 全通 |

---

## 8. 实现落地（P1/P2）

### 8.1 文件

| 文件 | 职责 |
|------|------|
| `proxy/internal/video/types.go` | Ark 请求/响应 DTO、orlucky 任务 DTO |
| `proxy/internal/video/catalog.go` | Seedance 模型约束、模型映射（含名称启发式） |
| `proxy/internal/video/mapper.go` | content/prompt/media、duration/resolution/ratio、状态映射 |
| `proxy/internal/video/orlucky.go` | orlucky client：`X-API-Key`、信封、幂等键、素材请求 |
| `proxy/internal/video/store.go` | bbolt：`task_xxx` ↔ orlucky taskId + 请求快照 |
| `proxy/internal/video/handler.go` | 路由、create/get/content、缓存、错误映射 |
| `proxy/internal/video/*_test.go` | 映射单测 + httptest 假上游 e2e |

### 8.2 配置

```yaml
video:
  store_path: ./data/video_tasks.db
  task_ttl_hours: 72
  create_timeout_seconds: 300
  query_timeout_seconds: 30
  cache_ttl_ms: 1500
  api_key: ""            # 默认；空 = 透传入站 Bearer
  public_base_url: ""    # 默认；空 = 直连上游素材 URL

gateways:
  - name: orlucky-doubao
    path: /orlucky          # 本地挂载点；渠道 Base URL = http://host:9000/orlucky
    upstream: https://router.orlucky.com/model/api
    protocol: doubao
    timeout: 300
    enabled: true
    video:
      models:
        doubao-seedance-2-0-mini-260615: seedance-mini
        doubao-seedance-2-0-260128: seedance-2.0
        doubao-seedance-2-0-fast-260128: seedance-fast
        doubao-seedance-2-5-260628: seedance-2.5
```

新增协议（例如 `ali`）只需要两步：

1. 在 `gateways` 增加一个 `protocol: ali` 的条目，配 `path` / `upstream` / `video.models`；
2. 在 `proxy/internal/video` 实现 `ali` 的协议 handler（路由、请求映射、响应呈现），
   并在 `NewAdapter` 的 protocol 分支注册。

存储、超时、缓存、鉴权透传、内容代理都是协议无关的，不需要重复配置。

### 8.3 本地验证

```bash
# 1. 启动假 orlucky（返回 create/query/asset）
# 2. 启动 agentgear
cd proxy && go run ./cmd/agentgear serve -c /path/to/config.yaml

# 3. 模拟外层 new-api 的 doubao 驱动
curl -X POST http://127.0.0.1:9000/doubao/api/v3/contents/generations/tasks \
  -H "Authorization: Bearer $ORLUCKY_KEY" -H "Content-Type: application/json" \
  -d '{"model":"doubao-seedance-2-0-mini-260615","content":[{"type":"text","text":"一只橘猫"}],"duration":5}'

curl http://127.0.0.1:9000/doubao/api/v3/contents/generations/tasks/task_xxx \
  -H "Authorization: Bearer $ORLUCKY_KEY"
```

### 8.4 测试

```bash
cd proxy
go test ./internal/video/... -race
go test ./...
```

已覆盖：文本/图生/视频输入、时长与分辨率校验、媒体数量限制、
unknown model、无鉴权、上游 401、任务失败、unknown task、
content 代理 200 与未就绪、multipart 拒绝。

入站只接受 `application/json`；`multipart/form-data` 返回 `415 unsupported_media_type`。
这与外层 doubao 插件的行为一致（它本身也拒绝文件，参考素材必须是 URL）。

---

## 附录 A：new-api 关键代码

| 内容 | 文件 |
|------|------|
| doubao 插件（本期入站协议） | `plugins/tasks/doubao/plugin.js` |
| `/doubao` 前缀逻辑 | `plugins/tasks/doubao/plugin.js` `apiRoot()` |
| 创建解析 | `parseSubmitResponse` |
| 查询解析 | `parseTaskResult` |
| 下载 URL | `buildContentRequest` |
| openai_video 宿主协议 | `protocols.openai_video` |
| 视频下载代理 | `controller/video_proxy.go` |
| New API 渠道 upstream 语义 | `docs/plugin-api/v1.md` |

## 附录 B：orlucky 接口

| 用途 | 方法与路径 |
|------|------------|
| 创建 | `POST /v1/tasks` |
| 查询 | `GET /v1/tasks/{taskId}` |
| 取消 | `POST /v1/tasks/{taskId}/cancel` |
| 模型目录 | `GET /models/public/catalog?categories=VIDEO&page=1&pageSize=100` |
| 鉴权 | `X-API-Key` |
| 幂等 | `Idempotency-Key` |
| 信封 | `{success, data, traceId}` |
