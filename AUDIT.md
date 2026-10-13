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
