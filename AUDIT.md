# Skill Hub 安全与质量审计报告

日期：2026-10-13　范围：`Desktop\skill-hub` 全部代码 + VPS 部署形态
方法：静态扫描（token 覆盖矩阵、LastInsertId 全量清点、单连接嵌套查询模式、前端转义覆盖）+ go vet + 动态回归（无 token 请求矩阵、路径穿越实测）

## 一、发现并已修复的问题（5 项，均已部署）

| # | 严重度 | 问题 | 修复 |
|---|---|---|---|
| 1 | 中高 | **daemon 增量推送绕过采集策略**：watchOnce 用零值 `pusher.Options{}` 扫描/收集/签名，单文件 10MB 与单 skill 50MB 预算在增量路径失效（媒体扩展名仍过滤），大文件可经增量通道绕过全量推送时的限制 | watchOnce/applyOnce 全部改用 opt.PushOpt，签名口径与收集口径一致 |
| 2 | 中 | **CreateMergedVersion 脏 LastInsertId**：`INSERT OR IGNORE` 撞已有 tree_hash 时 modernc 返回上一次插入的脏 rowid，重复采纳同一建议会把文件写到错误版本上 | 先查后插（同 SaveSkill 模式） |
| 3 | 中 | **关系图 tooltip 存储型 XSS**：vis-network 的 node/edge `title` 按 HTML 渲染，而 skill name/description/note 来自任意推送者的 frontmatter，未转义即插值 | 两处 title 加 esc() |
| 4 | 低 | **UpsertMachine 并发撞 UNIQUE**：两路 push 同名机器并发时第二条 INSERT 报错返回 500 | INSERT 失败重查一次 |
| 5 | 低 | **applyOnce 半成品风险**：RemoveAll 后解压若中断（磁盘满等），本地残缺目录会被 watcher 当作变更推回 hub，且 hash 不匹配源导致永远不再修复 | 先解压到 `target.incoming`，成功后原子 rename；失败保留原目录 |

修复后：go vet 干净、五平台编译通过、动态回归全绿（见下）。

## 二、动态回归结果（修复版本地实测）

- **token 覆盖矩阵**：stats/skills/families/graph/jobs/canonical/client-dl（读）+ push/dup-scan/edges-scan/job-create/adopt/dismiss（写）共 **13 个端点无 token 一律 401**；带 token 200 ✅
- **/dl/ 路径穿越**（`..%2f..%2fconfig.yaml`）：未泄露内容，落入安全 fallback ✅
- **/dl/ 免 token 分发**：仅允许 skill-hub* 前缀文件名，设计意图 ✅

## 三、安全面结论

**做得对的**：
- 认证模型：单 token + `X-Token` header → 天然免疫 CSRF（恶意网页无法携带自定义 header，JSON body 触发预检被 CORS 挡）
- tar.gz 解析双向防路径穿越（服务端收包、客户端解包均拒 `..`、绝对路径）
- API 写操作与 GLM 家族/关系分析全走 token 门；`/api/client/` 响应含 token，单独设了门
- GLM key 仅存 config.yaml（600），不入日志/报告/git
- 前端所有服务端数据插值经 esc()；vis label 按 SVG 文本渲染无注入面

**接受的剩余风险（设计权衡，已在文档声明）**：
1. **token = 完全信任**：持有 token 者可向任意在线客户端下发任务，daemon 会拉起 claude/zcode 执行提示词（agent 有本机全工具权限）→ **hub 凭据泄露等于全部客户端机器 RCE**。个人内网可接受；公网部署建议前置反代加 IP 白名单
2. token 在 serve.log 首行明文（服务器本地文件）；token 明文比对无哈希 — 个人规模可接受
3. 无 token 轮换 API（需手改 meta 表）；无速率限制（公网可被 401 爆破猜测，48 字符 hex 熵 192bit，实际不可行）
4. `/dl/` 免 token 下载二进制本体是设计（新机器引导需要），不含配置

## 四、正确性面结论

- **单连接嵌套查询死锁模式**（MaxOpenConns(1) 下 rows.Next() 循环内再查询）全库清点：ListDupGroups 在循环外嵌套（rows 遍历完自动归还连接，安全）；CanonicalMerges/ListFamilies 已改先收后查 ✅
- LastInsertId 全量清点：SaveSkill/UpsertMachine 先查后插 ✅；CreateMergedVersion 本次修复 ✅；ReplaceFamilies 事务内纯 INSERT 可靠 ✅
- 内容寻址 + 幂等 push + merged 来源追溯链路完整；WAL + busy_timeout + 单连接写串行化符合规模
- go vet 零输出

## 五、改进建议（未实施，按优先级）

1. **serve 用 systemd 托管**（当前 nohup，VPS 重启后需手动拉起）
2. **孤儿 blob 清理**：删除路径不回收 blobs 空间（曾手动清洗一次），可加 `skill-hub prune` 子命令
3. **skill 删除 API + UI**（当前只有推送/合并，无删除）
4. daemon 任务执行可加 `--permission-mode` 白名单（当前依赖 claude -p 默认权限，写文件类任务可能被权限确认卡住至超时）
5. 磁盘水位监控（VPS 曾被媒体打满，现有采集策略已根治推送侧，但 hub 自身增长无告警）
6. 轮换 token 的一键命令/脚本（当前手改 SQL）

## 六、结论

核心安全边界（token 门覆盖、路径穿越、注入转义）经动态回归全部通过；5 个确认 bug 已修复部署。当前形态适合个人/可信内网长期运行；若公开到公网，按第五节第 1 条加固后再暴露。

## 七、增量安全审计：权限模型（2026-10-14，范围：任务3新增的 admin/user 双角色体系）

### 7.1 权限模型设计复核

- **token 源**：meta.push_token（主 token，恒为 admin）+ tokens 表角色 token（admin/user）。TokenRole() 先比主 token 再查表，查表有 role 白名单校验（非 admin/user 即无效）✅
- **admin-only 面收敛**：POST /api/jobs、dup/scan、dup/groups/{merge-proposal,adopt,dismiss}、edges/scan、skills/{id}/system、settings/glm-test、tokens*（GET/create/revoke）— 共 10 个端点全部 requireRole(admin) ✅
- **user 可达面**：读端点、push、jobs/next（返回 id=0 空任务）、jobs/{id}/result、client 下载 ✅
- **关键提权面——/api/client/ 内嵌 token**：已改为内嵌**请求者自己的 token**（admin 下载内嵌主 token，user 下载内嵌 user token）。动态验证：user 下载的客户端尾部 JSON 内含 user token 且**不含** admin 主 token ✅（注意验证方法：marker 位置可能在尾部 300 字节之外，应在尾部 64KB 内找 marker 再查）

### 7.2 动态验证（本地实测，全部通过）

| 攻击场景 | 结果 |
|---|---|
| user token 打全部 admin 端点（jobs/dup-scan/tokens-create/tokens-list 等） | 全部 403 ✅ |
| user token 变体（前后空格） | 403/401，拒绝 ✅ |
| 伪造 admin 变体 token（末位改动） | 401 ✅ |
| user 尝试自我提权创建 admin token | 403 ✅ |
| user 尝试吊销主 token | 403 ✅（主 token 不在 tokens 表，且 revoke 需 admin） |
| user 下载客户端内嵌 user token（非 admin） | ✅ 无提权 |
| SQL 注入（搜索参数 / token 字段） | 参数化查询，401/正常业务 ✅ |
| 存储型 XSS（push 恶意 description） | API 按设计存原文，前端 esc() 覆盖（见第一章）✅ |
| token 吊销时效 | 吊销后立即 401 ✅ |
| whoami 免 token | 设计（只回显角色状态，不泄露信息）✅ |

### 7.3 新增风险与建议

1. **中**：user token 的 daemon 以 machine 名认领任务……user 角色返回空任务已堵住执行面，但 user token 仍可调用 GET /api/jobs 看到任务列表（元信息泄露：任务提示词对 user 可见）。建议：jobs 列表 admin-only（未实施，留待需要时）
2. **低**：tokens/create 支持 admin 角色意味着 admin 可造第二个 admin token——合理功能，但建议 UI 文案提示主 token 不可吊销（当前确实不可，主 token 不在表内）
3. **低**：RevokeToken 按掩码（前6后4）匹配删除——48hex 下 6+4 字符碰撞概率可忽略，可接受
4. **信息**：daemon 命令行含明文 token（ps 可见）——单用户机器可接受；若要收敛可改环境变量传递

### 7.4 结论

权限模型实现与设计一致，动态攻击面验证 10/10 通过，无发现可利用提权路径。主风险不变：**admin token = 全部客户端执行权**（设计本质），务必妥善保管。

## 八、增量安全审计：协议 v2 + 分配/下发（2026-10-16，范围：本次六项迭代新增面）

### 8.1 新增认证层——HMAC 请求签名（internal/server/auth.go + pusher/sign.go + app.js signHeaders）

- **token 不上 wire**：签名模式五个头（X-Key-ID/X-Timestamp/X-Nonce/X-Body-SHA256/X-Sign），Key-ID=sha256(token)[:16] 不可逆。协议详见 PROTOCOL.md ✅
- **防重放**：时间窗 ±10min + nonce 缓存 22min（容量 4096 惰性 FIFO 清理，重启清空由时间窗兜底）。实测：同 nonce 重放 401、过期时间窗 401、body/nonce 篡改 401、正确签名 200 ✅
- **防篡改**：X-Body-SHA256 覆盖完整请求体含 multipart bundle（push 10MB+ 包实测通过）；签名失败**不回落**明文模式（防 downgrade）✅
- **兼容窗口**：无签名头回落 X-Token（旧客户端可用）；UI/CLI/daemon 已全部切签名模式，明文模式仅为过渡
- 已知限制：明文 HTTP 下响应内容（skill 正文/任务报告）仍可被链路窃听——机密性需反代 TLS（PROTOCOL.md 已声明）

### 8.2 新增端点权限覆盖（全部经 handleAPI 统一认证门）

| 端点 | 权限 | 审计结论 |
|---|---|---|
| POST /api/heartbeat | user+ | 响应只回 server_time 与该机器 pending 指令；**只认 machine=body.hostname**，不可代领他机指令 ✅ |
| GET /api/assignments | user+ | 读分配（客户端拉取），泄露面=skill id/名/描述，user 本可读 skills，无增量泄露 ✅ |
| PUT /api/assignments/<host> | **admin** | requireRole(admin) ✅ |
| POST /api/dispatches、GET /api/dispatches/list | **admin** | requireRole(admin) ✅ |
| GET /api/dispatches?machine=x、POST .../result | user+ | 认领按 machine 过滤；**heartbeat 认领与补拉均校验 machine 参数与请求者身份一致性靠 token 信任域**（单租户设计，user token 可代任何 machine 认领——与旧版 jobs/next 同构，user 已被禁止 jobs/next，见 8.4 建议） |
| handlePush | 统一门 | **修复：移除 handlePush 内残留的重复 checkToken**（曾致签名请求被拦且认证逻辑双轨化）；重复来源路径分类 source_root 服务端判定，客户端不可伪造 is_system 绕过（只能自标为系统，影响面=展示分类，低） |
| handleClientDL | 统一门 | 签名模式已支持 ✅ |

### 8.3 新增面其他审计点

- **SQL 注入**：assignments/dispatches/heartbeat 全部参数化查询 ✅
- **上传校验**：分配/下发接受任意 skill_id/hostname，存在性校验（机器不存在 404）；skill_id 无存在性校验但 FK 约束兜底 ✅
- **DoS 面**：nonce 缓存有界（4096+惰性清理）；dispatches 表按 machine+status 索引，认领是 O(pending)，恶意客户端不认领不会堆积爆炸（pending 行数=后台下发的 admin 行为）✅
- **前端**：设备管理页全部插值经 esc()；PUT body 为纯 ID 数组 ✅
- **ClassifySystem 路径归一**：Rel 失败保留绝对路径匹配；markers 含 `/plugins/cache` 兜底——恶意推手可把 skill 目录命名为 `x/plugins/cache` 误标为系统（影响=默认隐藏，无权限含义），低风险可接受
- **隐私**：git 全历史（7390 行 diff）双模式扫描（GLM key 形态 `[0-9a-f]{28,40}\.[A-Za-z0-9]{8,24}` + 通用 40+ 随机串）零命中；跟踪文件现扫零命中 ✅

### 8.4 遗留建议（新增）

1. **低**：user token 可 GET /api/dispatches?machine=任意 认领他机指令——与单租户信任模型一致；若引入多租户需把 machine 与 token 绑定
2. **低**：明文 X-Token 兼容窗口建议观察 1-2 周后关闭（全部内置客户端已切签名）
3. **信息**：daemon 心跳间隔默认 30min，离线判定 35min——对"下发指令 30min 内触达"的预期成立

### 8.5 结论

协议 v2 的签名层经负路径动态验证（重放/篡改/过期全拒）；新增端点 token 门全覆盖、admin 面收敛正确；git 历史无任何密钥/隐私数据。可推送 GitHub 私库。
