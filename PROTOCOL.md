# Skill Manager 通讯协议 v2.1

2026-10-16 起生效。适用于 web 后台（账号登录）↔ exe 客户端（daemon/push/pull）与浏览器 UI 之间的全部 `/api/*` 通讯。

## 1. 总览

- 传输层：HTTP（Hub 当前部署为明文 HTTP，无 TLS）。**token 绝不明文上 wire**——认证走 HMAC-SHA256 请求签名。
- 认证三模式（v2.1 起浏览器与 API 分离）：
  - **会话模式（浏览器 UI）**：`POST /api/login` 用户名口令登录 → HttpOnly `Session` cookie（30 天）。token 与浏览器彻底解耦。
  - **签名模式（API 客户端，推荐）**：请求携带 `X-Key-ID / X-Timestamp / X-Nonce / X-Body-SHA256 / X-Sign` 五个头，不含 token。
  - **明文模式（旧客户端兼容宽限期）**：仅携带 `X-Token` 头。将被逐步淘汰；中间人可嗅探该头。
- 内容编码：端点交互均为 JSON（`Content-Type: application/json`）；`/api/push` 为 multipart 表单（bundle 字段 = tar.gz）。

## 2. HMAC 请求签名

### 2.1 五个头

| 头 | 值 |
|---|---|
| `X-Key-ID` | `sha256(token)` 十六进制前 16 位。服务端据此从候选 token 中找到 HMAC 密钥（token 本体不传输） |
| `X-Timestamp` | Unix 秒。服务端允许 ±10 分钟漂移 |
| `X-Nonce` | 每请求唯一的随机 hex（建议 16 字节 = 32 hex）。服务端在 22 分钟内拒绝重复值（防重放） |
| `X-Body-SHA256` | 请求体的 SHA-256 十六进制（GET / 空体 = `e3b0c442...` 空哈希）。服务端流式比对，防内容篡改 |
| `X-Sign` | `hex(HMAC-SHA256(key=token, msg=签名串))` |

### 2.2 签名串

```
signString = METHOD + "\n" + pathWithQuery + "\n" + X-Timestamp + "\n" + X-Nonce + "\n" + X-Body-SHA256
```

- `pathWithQuery`：URL path + 原始 query（如 `/api/skills?q=a&machine=b`）。客户端与服务端必须使用**原始未解码**形式。
- 服务端校验顺序：时间窗 → nonce 未用过 → Key-ID 找 token → `X-Body-SHA256` 比对 → HMAC 比对。任一失败返回 401（区分错误消息：`签名时间窗过期` / `nonce 已使用（疑似重放）` / `token 无效` / `body 摘要不匹配` / `签名校验失败`）。
- **不降级**：签名校验失败不会回落明文模式（防 downgrade attack）。

### 2.3 客户端实现参考

- Go：`internal/pusher/sign.go` 的 `AddAuth(req, token, body)`。
- 浏览器 JS：`static/app.js` 的 `signHeaders()`（内置纯 JS SHA-256/HMAC——http 非 localhost 下 `crypto.subtle` 不可用）。
- 时钟漂移超过 ±10 分钟的设备会 401（`签名时间窗过期`），需校时。

## 3. 心跳探活

- 客户端 daemon 每 **30 分钟**（`--heartbeat 1800`，可调）`POST /api/heartbeat`，body `{"hostname": "<本机名>"}`。
- 服务端刷新该机器 `last_seen`；**超过 35 分钟无心跳/推送判离线**。
- 响应（同时承担指令下发通道，见 §5）：

```json
{
  "ok": true,
  "server_time": "2026-10-16T00:49:55Z",
  "offline_after_min": 35,
  "dispatches": [{"id": 1, "machine": "hb-dev", "action": "sync"}]
}
```

- daemon 启动时立即心跳一次（不等首个 ticker），保证上线即在线、积压指令即领。

## 4. 设备-skill 分配（管理平面）

- 数据模型：`assignments(machine_id, skill_id)`。
- `GET /api/assignments?machine=<hostname>`：列出分配给该设备的 skill（user token 可读，客户端拉取用）。
- `PUT /api/assignments/<hostname>` body `{"skill_ids": [1,2,...]}`：全量替换该设备的分配集合（**admin only**）。
- 客户端拉取分配集合并落盘：
  - CLI：`skill-hub pull --server URL --token TOK --dest ~/.agents/skills --assigned`（`--machine` 缺省取本机名；分配项**覆盖落盘**——分配即管理意图）。
  - 托盘菜单：「下载云端分配 skill」。
  - 落盘目录：daemon 取监控根中第一个用户级目录（跳过 `plugins/` 缓存目录），CLI 用 `--dest`。

## 5. 双向同步与下发指令

### 5.1 指令生命周期

```
后台(admin) POST /api/dispatches {"machine":"hb-dev","action":"sync"}
  → dispatches.status = pending
  → 客户端下次心跳响应捎带（或 GET /api/dispatches?machine=hb-dev 补拉）→ status = running（记 picked_at）
  → 客户端执行 → POST /api/dispatches/<id>/result {"status":"done|failed","result":"..."} → 终态
```

- `action` 取值：`sync`（推送本机变更 + 应用云端合并 + 部署分配 skill）或 `pull`（仅部署分配 skill）。
- 后台查看历史：`GET /api/dispatches/list`（admin，最近 50 条）。

### 5.2 客户端主动同步

- 托盘「立即同步」= sync 全套；「上传本机全部 skill」= 强制全量推送（不经增量签名门控）；「下载云端分配 skill」= 仅 pullAssigned。
- CLI：`skill-hub pull --assigned`；`skill-hub upload`（= push 别名）。
- 常规自动周期不变：增量监控推送 30s、应用云端合并 5min、远程任务轮询 5s。

## 6. 系统 skill 标记

- 系统 skill = ZCode 客户端内置 + Claude Code 内置（其插件缓存目录下采集到的 skill）。
- 判定规则（服务端 `store.ClassifySystem`）：采集来源根路径（相对用户 home）命中 `.zcode/cli/plugins/cache`、`.zcode/plugins`、`.claude/plugins`、`.claude-code/plugins`、`/plugins/cache` 任一前缀 → `is_system=1`。
- 客户端 push/daemon 上传时携带 `source_root` 表单字段；已有手动标记不被采集降级清除（只置 1 不清 0）。
- UI：Skills 列表与关系图均有「隐藏系统 skill」开关（默认隐藏，记忆于 localStorage）；详情页可手动标记/取消。

## 7. 端点权限矩阵

| 端点 | 方法 | 认证 | 角色 |
|---|---|---|---|
| `/api/login`、`/api/logout` | POST | 免 | 任意（登录/登出） |
| `/api/whoami` | GET | 免 | 任意（自检） |
| `/api/push` | POST | 签名/明文 | user+ |
| `/api/heartbeat` | POST | 签名/明文 | user+ |
| `/api/password` | POST | 会话 | 登录用户本人改密 |
| `/api/settings/llm`（POST 保存） | POST | 签名/明文/会话 | **admin**（key 热生效，落 DB meta） |
| `/api/skills*`、`/api/graph`、`/api/stats`、`/api/families`、`/api/machines`、`/api/canonical`、`/api/assignments`、`/api/settings`（GET） | GET | 签名/明文/会话 | user+（主 token 字段仅 admin 可见） |
| `/api/skills/<id>/system`、`/api/dup/*`（scan/merge/adopt/dismiss）、`/api/edges/scan`、`/api/assignments/<host>`（PUT） | POST/PUT | 签名/明文 | **admin** |
| `/api/dispatches`（POST 下发）、`/api/dispatches/list` | POST/GET | 签名/明文 | **admin** |
| `/api/dispatches`（GET 认领）、`/api/dispatches/<id>/result` | GET/POST | 签名/明文 | user+（客户端） |
| `/api/jobs`（POST 下发/GET 列表）、`/api/tokens*`、`/api/settings/llm*`、`/api/users*` | POST/GET | 签名/明文/会话 | **admin** |
| `/api/jobs/next`（认领） | GET | 签名/明文 | **admin**（user 客户端返回空任务不领取） |
| `/api/client/<bin>` | GET | 签名/明文 | user+（内嵌请求者自身 token） |
| `/dl/<bin>` | GET | 免 | 分发引导用（不含敏感数据） |

## 8. 安全性质小结

- **防嗅探**：签名模式下 token 不出现在任何请求中；浏览器走会话 cookie，token 与浏览器解耦
- **登录防爆破**：同用户名连续失败 5 次锁 60 秒（内存态）；`X-Key-ID` 仅泄露 sha256 前 64 bit，不可逆。
- **防重放**：时间窗 + nonce 缓存（重启清空由时间窗兜底）。
- **防篡改**：`X-Body-SHA256` 覆盖完整请求体（含 multipart bundle）。
- **降级攻击**：服务端见 `X-Sign` 头即只走签名校验，失败不回落。
- 已知限制：Hub 无 TLS，响应内容（skill 正文/报告）仍可被链路窃听；需要机密性时请在反向代理层加 TLS。
