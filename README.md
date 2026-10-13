# Skill Hub — 跨机 Agent Skill 管理平台

一个单二进制、零依赖的自托管 web 平台，用于集中管理散落在多台机器、多个 agent 工具（ZCode / Claude Code 等）里的 skill（每个 skill = 一个含 `SKILL.md` 的目录）。

## 特性

- **双模式单二进制**：`serve` 起 web 服务，`push` 从任意机器上传 skill，复制 exe 即接入
- **版本与演变路线**：每次 push 按目录树 sha256 内容寻址，内容变化才存新版本，全历史永久保留；任意两版本 unified diff
- **智能去重**：精确哈希分组 + 名称/描述相似度预筛（中文 bigram Jaccard）+ GLM 逐对判定（给置信度与理由）
- **智能合并**：GLM 生成合并版 SKILL.md 建议，UI 预览 + diff 对比，人工点「采纳」才落新版本（`merged` 标记 + 记录来源版本），绝不自动覆盖
- **关联关系**：解析 SKILL.md 正文引用建边 + GLM 归纳 related（相关）与 chain（调用链）边；vis-network 力导向图展示
- **优雅降级**：GLM 未配置/欠费时，去重退化到预筛、合并与关系归纳不可用，其余功能完整

## 当前部署（2026-10-13）

**中心 hub 已部署在 VPS：http://your-server:8790**（所有 API 含读都需 token，UI 首次访问弹窗输入；token 见 VPS `<server-dir>/serve.log` 或 `.token` 文件）。

- 已接入机器：`vps`（cron 每日 18:30 全盘自推）、`your-host`（双击 `push-here.bat` 全盘手动推）
- **新机器接入 = 打开 hub 设置页，复制一条命令到目标机器终端执行**（token 已注入命令）：
  - Linux / macOS / Git Bash：`curl -fsSL http://your-server:8790/agent.sh | sh -s -- --server ... --token ...`
  - Windows PowerShell：`iwr agent.ps1` 引导脚本
  - 脚本自动：下载二进制到 `~/.skill-hub/` → 全盘发现本机 skill 目录（.agents/.claude/.zcode 插件缓存）→ 逐一推送。重复执行安全
- **常驻客户端**（推荐每台机器都跑）：设置页复制 daemon 命令执行即可——增量监控推送（30s）+ 自动应用云端合并（5min，源变体门控防覆盖手工修改）+ 接收远程 skill 调用
- 手动分发：`skill-hub pull --server ... --token ... --dest ~/.agents/skills`
- **远程调用**：UI「远程调用」页选机器+skill+提示词下发；目标机器 daemon 拉起 claude/zcode -p 执行并回传 asdste100 报告
- 采集策略（防媒体档案撑爆库）：媒体/档案扩展名默认剔除（mp3/mp4/zip/exe 等，`--include-binaries` 放开）、单文件 10MB、单 skill 总量 50MB（小文件优先，SKILL.md 永远收）

## 快速开始

### 服务端（一次）

```bash
skill-hub.exe serve                # 默认 0.0.0.0:8790，数据/配置在二进制同目录
# 可选: skill-hub.exe serve --addr 0.0.0.0:9000 --db D:/data/skillhub.db
```

打开 `http://127.0.0.1:8790` → 设置页复制**接入 Token**。

### 其他机器接入（三步）

```bash
# 1. 复制 skill-hub.exe 到目标机器（无任何运行时依赖）
# 2. 在设置页拿 token（X-Token）
# 3. 扫描推送（可多次、可多目录；--hostname 自定义来源标记）
skill-hub push --server http://SERVER_IP:8790 --token TOKEN --path ~/.agents/skills --hostname my-laptop
skill-hub push --server http://SERVER_IP:8790 --token TOKEN --path ~/.zcode/cli/plugins/cache/zcode-plugins-official
```

递归扫描目录下**所有深度**含 `SKILL.md` 的子目录（插件缓存的多层嵌套也能处理）；同机同名 skill 内容变化自动成为新版本（演变时间线）。

### GLM 配置（可选，开启智能功能）

`config.yaml`（与二进制同目录，已被 .gitignore）：

```yaml
glm_api_key: "你的key"
glm_base_url: "https://open.bigmodel.cn/api/paas/v4"   # 若 key 只在 anthropic 端点有余额，改 https://open.bigmodel.cn/api/anthropic/v1
glm_model: "glm-5.3-flash"
```

环境变量 `GLM_API_KEY` / `ZHIPUAI_API_KEY` 优先于文件。base_url 含 `/anthropic` 时自动走 Anthropic messages 协议（含 GLM 思考块禁用）。设置页有「测试连通性」按钮。

## 页面

| 页面 | 内容 |
|---|---|
| 总览 | skill/版本/机器/重复组/关联边计数、内容存储量、最近变更流 |
| Skills | 搜索、按机器过滤、版本数与树哈希 |
| Skill 详情 | 基本信息、版本时间线、任意版本 diff、文件浏览、出入关联边 |
| 关系图 | 力导向图，节点按 GLM 家族着色、图例点击聚焦；「更新关系」按钮才触发 GLM 重构并固化，平时展示固化结果 |
| 去重合并 | 执行扫描、GLM 判定理由、生成合并建议、并排预览、采纳/忽略 |
| 家族 | GLM 按内容聚类（人物思维框架/ESP32测试台等），列表筛选+图着色+详情同族，随「更新关系」固化 |
| 远程调用 | 下发任务（机器/skill/提示词）、任务列表、asdste100 报告查看 |
| 设置 | 接入 token、GLM 状态与连通测试、一键引导/daemon 命令 |

另：`pull` 子命令把 hub 最新版本拉回本地（合并成果分发，默认跳过已存在，同名多机自动 `名字@机器` 目录区分）；`/dl/` 免 token 分发二进制本体。

## API 一览

读写分权：读无需认证；写操作（push/扫描/采纳/忽略）需 header `X-Token`。

```
POST /api/push                          multipart(bundle=tar.gz + hostname)
GET  /api/stats                         仪表盘统计 + 最近变更
GET  /api/machines                      机器列表
GET  /api/skills?q=&machine=            列表（搜索/过滤）
GET  /api/skills/{id}                   详情（含版本时间线、出入边）
GET  /api/skills/{id}/versions          版本列表
GET  /api/skills/{id}/tree?version=     某版本文件清单（path→blob hash）
GET  /api/skills/{id}/file?version=&path=  读某版本某文件原文
GET  /api/skills/{id}/diff?from=&to=    两版本 SKILL.md unified diff
POST /api/dup/scan                      执行去重（精确+预筛+GLM）
GET  /api/dup/groups?status=            重复组列表
POST /api/dup/groups/{id}/merge-proposal  GLM 生成合并建议
POST /api/dup/groups/{id}/adopt         采纳（body: {skill_md, description, notes}）
POST /api/dup/groups/{id}/dismiss       忽略
POST /api/edges/scan                    重建引用边 + GLM 关系归纳
GET  /api/edges                         全部边
GET  /api/graph                         {skills, edges}（关系图数据源）
GET  /api/settings                      token + GLM 状态
POST /api/settings/glm-test             GLM 连通测试
```

## 数据与运维

- 全部数据在二进制同目录：`skillhub.db`（SQLite/WAL，内容寻址 blob 存储）+ `config.yaml`
- 备份 = 拷走 `skillhub.db` 一个文件；迁移 = 拷走 db + exe
- 停止服务：`taskkill //IM skill-hub.exe //F`（Windows）/ `pkill skill-hub`（Linux）
- 上 VPS 建议：`nohup ./skill-hub serve > serve.log 2>&1 &` + nginx 反代加 TLS；端口放行防火墙即可，无需数据库服务

## 已知限制

- 鉴权为单 token 明文比对，仅适合个人/可信内网；公网部署请套 TLS + 反代加 IP 白名单
- 近似去重预筛对「同名不同描述风格」依赖 GLM 兜底；GLM 欠费时预筛阈值（0.55）可能漏掉描述差异大的变体
- diff 仅针对 SKILL.md；其余文件变更体现在版本时间线（树哈希变化）但无逐文件 diff
- GLM 判定/合并为逐对串行调用，skill 数量大时扫描耗时随之增长
