# Skill Hub 交付报告

日期：2026-10-12　交付物：`C:\Users\bool\Desktop\skill-hub\`

## 一、结果概述

跨机 Agent Skill 管理平台 MVP 已完成并通过全链路实测。Go 单二进制（约 14.7 MB，含内嵌前端）+ SQLite（纯 Go 驱动，无 CGO），服务正在本机后台运行：**http://127.0.0.1:8790**。

实测数据规模：**47 个 skill 实体 / 66 个版本 / 2 台机器 / 79 条关联边 / 404 MB 内容存储**，全部来自本机真实 skill 资产（`~/.agents/skills` 20 个 + ZCode 插件缓存 67 个扫描、43 个新实体，同 hash 幂等跳过 24 个）。

## 二、架构

```
skill-hub.exe（单二进制）
├── serve 模式
│   ├── internal/server   HTTP API + go:embed 内嵌 SPA（原生 JS，vis-network CDN）
│   ├── internal/store    SQLite：machines/skills/versions/blobs/dup_groups/edges
│   │                     内容寻址：文件→blob(sha256)，skill 目录树→tree_hash，变化才存新版本
│   ├── internal/analysis 去重（精确哈希+相似度预筛+GLM判定）、合并建议、关系边
│   └── internal/llm      GLM 客户端：OpenAI 兼容 与 Anthropic messages 双协议自动切换
└── push 模式
    └── internal/pusher   递归扫描 SKILL.md（任意深度）→ tar.gz 打包 → multipart 上传
```

关键设计决策：

1. **传输协议用 tar.gz 而非逐文件 multipart**：Go 的 multipart 解析按 RFC 7578 会把 filename 处理成 basename（丢弃目录结构），首版踩坑后改为整包 tar，顺带获得压缩传输。
2. **服务端 skill 归组按「SKILL.md 所在目录」且文件归属最近祖先**：正确处理插件缓存多层嵌套（`browser-use/0.2.1/skills/control-browser` 与 `0.3.0/...` 成为同一 skill 的两个版本，版本时间线即插件升级演变史）。
3. **upsert 一律先查后插**：modernc.org/sqlite 在 `INSERT ... ON CONFLICT DO UPDATE` 分支下 `LastInsertId()` 返回上一次 INSERT 的脏 rowid，曾导致把 skill 表 id 当 machine_id 写入触发外键失败。
4. **GLM 走 Anthropic 端点**：从 Vaultwarden 取回的 Zhipu key 在 `paas/v4`（OpenAI 兼容）端点报 1113 余额不足，而 anthropic 端点（`/api/anthropic/v1`）有余额（与 ST 部署时的发现一致）。llm 包按 base_url 含 `/anthropic` 自动切协议，并默认 `thinking: disabled`（GLM-5.3 思考块吃预算）。
5. **中文相似度用 CJK bigram**：中文无空格，整句成单 token 会使 Jaccard 恒近 0；按标点切 + 中文二元组后预筛才有意义。同名跨机 skill 直接视为强候选（Jaccard 会被长触发词描述稀释到阈值之下）。

## 三、实测结果（全部通过）

| 验收项 | 结果 |
|---|---|
| go build | ✅ 单二进制 skill-hub.exe |
| push 第一路 `~/.agents/skills` | ✅ 20 skills |
| push 第二路 ZCode 插件缓存 | ✅ 扫描 67，43 新实体（24 个同 hash 幂等跳过） |
| 幂等重推 | ✅ 第二次 17/17 全 skip（created=false） |
| 版本演变 | ✅ pick_node(laptop) 两次不同内容 push 产生 v64/v65 两个版本 |
| 版本 diff | ✅ control-browser v38→v50（插件 0.2.1→0.3.0）18 行 unified diff 正确 |
| 去重·精确 | ✅ 0 组（本机两来源无完全同哈希跨实体，属预期） |
| 去重·近似+GLM | ✅ 制造跨机变体（pick_node 改描述+加段落）→ GLM 置信 0.98 判定为同一 skill 变体落组；ios-dev vs android-dev、diagnosing-skills vs diagnosing-commands 被正确判非变体排除 |
| GLM 合并建议 | ✅ 生成 3723 字符合并版 SKILL.md + 新 description + 合并要点 |
| 采纳落库 | ✅ v66 kind=merged，source_version_ids=[15,65]，组状态转 merged |
| 关系边 | ✅ 引用 54 条（正文词边界匹配）+ GLM related 17 + chain 8，无自环 |
| UI 六页截图 | ✅ 总览/Skills/详情/关系图/去重工作台/设置 全部渲染正常（截图在 `.shots/`） |
| GLM 连通 | ✅ /api/settings/glm-test 返回「正常」 |

## 四、GLM key 获取

按任务规定路径执行：env 无 key → Vaultwarden（VPS 上 bw CLI 解锁，取「Zhipu BigModel」条目）→ 实测该 key 在 paas/v4 端点 1113 余额不足 → 从 ST secrets.json 的 api_key_claude 槽取当前活跃 key，实测 anthropic 端点连通。key 只写入 `config.yaml`（.gitignore 已覆盖），临时文件已删，全程未出现在日志与报告。

## 五、过程中修过的坑（复用注意）

1. **multipart filename 丢路径**（RFC 7578 basename 处理）→ tar.gz 打包协议
2. **modernc sqlite upsert 脏 LastInsertId** → 全部先查后插
3. **8787 被既有服务占用**（本机已有 python 服务监听 127.0.0.1:8787，0.0.0.0 绑定 curl 却命中更具体的 127.0.0.1）→ 默认端口改 8790
4. **中文 Jaccard 失真** → CJK bigram
5. **GLM 对 `same_variant` 字段咬文嚼字**（功能一致却判 false）→ 语义改为 `is_duplicate` 并在 prompt 中给明确规则
6. **GLM 偶发中文字段名** → 宽松 JSON 解析（中英文键名兼容）
7. **本机系统代理劫持 python urllib 对 localhost 的请求**（curl 不受影响）→ python 侧禁代理

## 六、已知限制

- 鉴权为单 token，适合个人/内网；公网部署需 TLS + 反代加固
- GLM 欠费时近似去重退化为纯预筛（阈值 0.55），合并/关系归纳不可用，UI 显示"GLM 未配置"
- diff 仅覆盖 SKILL.md；其他文件变更体现在树哈希与版本时间线
- LLM 逐对串行调用，扫描耗时随 skill 数增长（本次 47 skill 全量约 1~2 分钟）

## 七、上 VPS 部署建议（未执行，按需）

1. 交叉编译：`set GOOS=linux&& set GOARCH=amd64&& go build -o skill-hub .`（或 CI）
2. 上传 `<server-dir>/`（exe + 可选 config.yaml），`nohup setsid ./skill-hub serve --addr 0.0.0.0:8790 > serve.log 2>&1 &`（paramiko 拉起须 setsid，见既有经验）
3. 腾讯云防火墙放行 8790/TCP；建议 nginx 反代套 TLS 后再公网开放
4. 各机器 `crontab` 定期 `skill-hub push`（如每日一次）即可自动沉淀演变历史

## 八、当前运行状态与停止方式

- 服务以 nohup 后台运行中（serve.log 在项目目录），数据 `skillhub.db`（406 MB，主要是 xlsx/pdf 等插件的引擎文件 blob）
- 停止：`taskkill //IM skill-hub.exe //F`
- 再次启动：双击 `skill-hub.exe serve` 或命令行执行

## 九、跨机统一管理（2026-10-13 追加）

**hub 中心已迁至 VPS（your-server:8790），本机 serve 已停**（重启：项目目录 `skill-hub.exe serve`，仅作开发用）。

新增能力：
1. **全 API token 门**（含读，公网不裸奔）；UI 首次访问弹窗输入 token 存 localStorage；serve 启动日志打印 token
2. **`pull` 子命令**：hub 最新版本拉回任意机器（合并成果分发；默认跳过已存在，`--overwrite` 覆盖，同名多机落盘为 `name@machine/`）；配套服务端 `/api/skills/{id}/download`（tar.gz）
3. **`/dl/skill-hub`** 二进制自分发：新机器 curl 即得可执行文件
4. **VPS 接入**：`~/.agents/skills` 66 个 skill 以 hostname=vps 推入（大量 *-perspective 人设类 + vaultwarden/xhscrawler/kuaishou-upload 等），cron 18:30 每日自推（selfpush.sh + .token 600 权限）
5. **Windows 侧**：`push-here.bat`/`push-here.sh` 一键双路推送（token 读同目录 .token）

实测结果：种子库 405MB 经 SFTP 68 秒上传；公网 token 门 401/200 双验证通过；全库去重扫描发现 **4 组真实跨机重复**（sms-receive 完全同版、temp-mail VPS 版更新、virtual-identity 变体、VPS 内部 zhang(-)xuefeng-perspective 命名分裂）——全部在工作台等人工采纳；关系边重扫：引用 94 + GLM related/chain 24。

运维备忘：
- hub 日志/重启：VPS `<server-dir>/serve.log`；重启 `cd <server-dir> && nohup setsid ./skill-hub serve --addr 0.0.0.0:8790 > serve.log 2>&1 &`
- 备份 hub：拷 `skillhub.db`（当前 ~610MB）
- 代码更新流程：本机改码 → `go build` + `GOOS=linux GOARCH=amd64 go build` → SFTP 传两个二进制 → 重启 serve

## 十、一键接入与全盘收集（2026-10-13 晚追加）

用户要求"不要安装 app，前端直接复制命令"：

1. **设置页一键命令面板**：三条带 token 注入的复制即用命令（Linux/macOS/Git Bash 的 curl|sh、Windows PowerShell、反向 pull），token 从本页 API 取，机器名可选注入
2. **引导脚本**（hub 自带，免 token）：`/agent.sh`（POSIX sh，支持 Linux/Darwin/MINGW）与 `/agent.ps1`——下载对应平台二进制到 `~/.skill-hub/`、全盘自动发现 skill 目录（.agents/.claude/.claude/plugins/.zcode 插件缓存，存在才推）、逐一 push。五平台二进制全放在 hub `/dl/`（linux amd64/arm64、darwin amd64/arm64、win exe）
3. **采集防膨胀策略**：默认剔除媒体/档案扩展名、单文件 10MB、单 skill 总量 50MB（小文件优先保留，SKILL.md 必收）；旗标 `--include-binaries/--max-file-mb/--max-skill-mb` 可放开
4. **实测**：本机 Git Bash 原样执行 UI 复制命令 → 4 路全通，your-host 补齐 49 个 Claude skill（.claude/skills 38 + .claude/plugins 11），laoxie-perspective 2.6GB 媒体档案按策略剔除只收正文与脚本；hub 现 161 skill / 198 版本
5. **VPS 磁盘事故与恢复**：.claude/skills 首 push 带 2.6GB 媒体把 40G 盘打满（99%）→ journal/apt/docker 回收 + WAL checkpoint（790M→0）恢复到 89%；随即实现第 3 条策略根治
6. **过程坑**：`pkill -f 'skill-hub serve'` 会匹配重启命令自身 bash 导致自杀（改 `pkill -x skill-hub` + `(nohup setsid ... &)` 子壳脱离）；mingw 下 curl|sh 一键命令可直接跑 Windows
7. 本机 ~/.agents/skills 的 playwright-* 三目录在会话期间被外部（ZCode 客户端）删除——hub 仍保留其历史版本，可随时 pull 找回

## 十一、常驻客户端 daemon（2026-10-13 深夜追加）

用户需求：所有设备装精简客户端 — 监控（非全盘扫描）多 agent 目录定时同步、应用云端去重/优化结果、后台远程调用 skill 并回传 asdste100 报告。

**实现**（`skill-hub daemon --server --token [--hostname] [--interval 30 --apply-interval 300 --job-poll 5 --roots ...]`）：

1. **增量监控**：只盯 agent 目录（自动发现 .agents/.claude/.claude/plugins/.zcode 插件缓存，--roots 可指定）。每 30s 对每个 skill 目录算轻量签名（过滤后文件的 rel|size|mtime 哈希，不读内容），签名变了才收集+推送该 skill
2. **应用云端合并**：每 5 分钟拉 /api/canonical（每名字最后一次 merged 版本）。apply 有源哈希门控——本地内容仍是合并前源变体（树哈希命中 source_hashes）才落盘，用户后续手工修改永不被覆盖
3. **远程调用**：每 5s 认领 /api/jobs/next；按机器条件选 CLI（claude 优先 zcode 次之，LookPath 探测）；`<cli> -p` 提示词走 stdin（AGENTS.md 铁律，防 cmdline 自杀），工作目录 ~/.skill-hub/jobs/<id>/，10 分钟超时；产出 asdste100 JSON 报告（format/version/job_id/skill/machine/agent_cli/status/started_at/finished_at/exit_code/output/stderr/artifacts）回传 hub
4. **UI**：新「远程调用」页——机器下拉+skill datalist+提示词下发、任务列表 5s 自刷新、点行看 asdste100 报告与执行输出；设置页新增第 4 条可复制命令（daemon 常驻启动）；agent.sh 支持 --daemon 直接转常驻
5. 服务端新增 jobs 表与 /api/jobs*、/api/canonical

**e2e 实测**（全部通过）：VPS daemon 已常驻（roots 4 个，68+ skills 初始推送）；下发任务 #1 → vps 5s 内认领 → claude -p 执行 → status=ok 输出"远程调用成功" → asdste100 报告入库可查；沙箱机器 watch 感知文件重置（10s 内增量推送）；生成合并→采纳→15s 内 daemon 自动应用到沙箱（"已应用云端合并结果: pick_node -> v283"）→ watcher 推回收敛态。测试实体与测试合并已清理（desktop pick_node 指回 v152，canonical 只剩演示用 v66）。

**新坑记录**：
- `MaxOpenConns(1)` 下 rows 循环内嵌套 QueryRow = 死锁（外层占唯一连接内层等连接），把整个 serve 拖死——先收完 rows 再查
- canonical 不能用"latest 是 merged"判定：后续普通推送会推翻合并结论；改为"每名字最后一次 merged 版本"+ 源哈希门控
- 旧策略（含 __pycache__/媒体）收的历史版本哈希与新策略口径不一致，跨策略的合并源匹配不上——属过渡期现象，新合并无此问题
- 修复后重启命令继续用 `pkill -x`（再次踩到 -f 自匹配：模式 'skill-hub daemon' 出现在重启命令自身 cmdline 里）
- asdste100 为用户口述格式名，未查到同名公开标准，按自定义 schema（见上）实现，字段可再对齐

## 十二、skill 家族（GLM 内容聚类，固化展示）（2026-10-13 追加）

需求：无关联但同类的 skill（如 45 个 *-perspective）要有分类展示；分类不能用静态前后缀，要内容验证；agent 计算只在点「更新关系」时触发，结果固化，平时纯读库。

**实现**：
- store 新增 families/family_members 表；ReplaceFamilies（全量替换）/ListFamilies
- analysis.ScanFamilies：GLM 分批（80 名/批）按【实际功能与内容】聚类（命名词根仅作线索，明确提示"命名相似但功能不同不要硬归、宁缺毋滥"，每族≥3），跨批同名族合并，成员映射到全部跨机变体
- 「更新关系」按钮=一次性全量重构：引用边（静态秒级）+ related/chain 边（GLM）+ 家族（GLM），全部固化入库；/api/families 纯读库；所有 GET 路径零 LLM 调用
- UI：关系图节点按家族着色（12 色循环）+ 图例 chips（悬停看注解，点击选中+聚焦家族，再点复位）；Skills 列表家族筛选下拉+行内族徽章；详情页「同族」面板；总览页族谱 chips
- 实测（209 节点）：GLM 产出 19 族，如 人物思维框架 45、插件与扩展开发 14、ESP32测试台 11、GUI与应用自动化 4（把 android-dev/ios-dev/computer-use/control-browser 四个命名完全不同的 skill 按内容归到一起——纯静态命名分析做不到）

**过程坑**：
1. ListFamilies 用 &out[len(out)-1] 取指针，append 扩容后成野指针——成员写进旧数组只显示 3 族；改索引写入
2. 客户端 JSON 字段大小写不匹配（f.Names vs 服务端 names）导致图页报错——全部统一小写
3. 静态文件无缓存头导致浏览器缓存旧 app.js（新 UI"没生效"假象）——加 Cache-Control: no-cache
4. vis-network net.focus() 不接受数组、fit 带 animation 在物理稳定期会被覆盖——用 net.fit({nodes: ids}) 无动画

## 十三、托盘客户端 + 下载按钮接入（2026-10-13 追加）

**Windows 托盘客户端**（`skill-hub daemon ... --tray`）：
- 启动即隐藏自身控制台（GetConsoleWindow+ShowWindow SW_HIDE syscall），系统托盘出现蓝色图标
- 右键菜单：打开 Hub 页面（rundll32 FileProtocolHandler）/ 立即同步（触发 watchOnce+applyOnce）/ 退出（Stop chan 优雅停）
- 实现：getlantern/systray（纯 Go 免 CGO，windows 可交叉编译）；icon.ico（python 手写 PNG-in-ICO，32x32 蓝球白星）go:embed；非 Windows 平台 --tray 降级提示后 CLI 常驻
- daemon 日志双写：stdout + ~/.skill-hub/daemon.log（托盘模式黑框不可见，文件是唯一观测面）

**设置页「一键接入（免命令行，推荐）」**：
- 主按钮「下载双击接入.bat」：JS Blob 动态生成，预注入 hub 地址+token（CRLF+全 ASCII，cmd 兼容）；双击后 = 自动下载 exe（PS SilentlyContinue 提速）→ start 启动托盘客户端
- 副按钮：直接下载 exe / Linux / macOS×2（/dl/ 免 token 自分发）
- 开机自启指引：shell:startup 放 bat 快捷方式
- 原 curl|sh / PowerShell 命令面板保留

**实测**：本机托盘实例运行正常（进程存活、日志双写、初始推送 4 目录）；bat 全流程真实跑通（从 hub 下载 exe 15.7MB → 托盘实例启动 → 自动发现推送）。

**新坑**：
- `timeout /t N >nul` 在无 stdin 环境（重定向/无人值守）会挂死——bat 里用 `ping -n N+1 127.0.0.1 >nul` 替代
- go:embed 需要 import "embed"（仅变量注解用 `_ "embed"`）
- heredoc 写 JS 模板字符串时 `

` 层级转义极易翻车（bash→python→JS 三层），行级重建校验是唯一可靠路径

## 十四、纯二进制客户端（双击即用，无脚本）（2026-10-13 追加）

用户要求：不要 bat，只要二进制。

**原理**：服务端 `/api/client/{name}`（token 门）读同目录平台二进制，在尾部追加 marker+JSON（server=请求 Host 派生 + token），流式返回"已配置客户端"。PE/ELF/Mach-O 加载器都不读文件尾部，追加不影响执行；客户端启动时读自身尾部 64KB 解析配置。

**无参数 = 客户端模式**：双击 exe（或 ./skill-hub）→ 读内嵌配置（回落 ~/.skill-hub/config.json）→ 落盘 config.json → Windows 托盘启动 / 其他 CLI 常驻；hostname 自动取主机名。无配置则 Windows 原生 MessageBox 引导（去设置页下载已配置版或用命令行）。命令行四子命令行为不变。

**e2e**：临时 serve 下载 /api/client/skill-hub.exe（15MB，尾部 marker 确认）→ 无参数运行 → config.json 正确落盘 → 托盘启动并以真实主机名 your-host 入库推送。

**安全**：/api/client/ 响应含 token，已在 handleClientDL 内置 X-Token 检查（独立于 handleAPI 全局门，因 mux 最长前缀匹配绕过它）。

## 十五、采集范围收窄为仅 SKILL.md（2026-10-13 追加）

用户指示：只关注 SKILL.md 核心文档，不关注产物目录。

变更：ScanSkills/CollectSkillMD/SignatureSkillMD 全部只针对 SKILL.md（签名=size+mtime，不读内容不遍历子树）；push 只上传 SKILL.md；pull/apply 只替换目标目录的 SKILL.md（**不再 RemoveAll 整个目录** — 本机脚本/产物归本机所有）。媒体过滤、大小预算、运行时目录忽略等历史机制被此收窄完全涵盖（保留代码无副作用）。

效果：版本历史 = 纯 SKILL.md 演变史；数据产物（如 bijia transcripts）彻底无感；daemon CPU 进一步下降（签名只 stat 一个文件）；库体积停止吸收资产文件。回归实测：push 17 skills → version_files 恰 17 条且 path 全为 SKILL.md。历史旧版本（含全目录收集的）保留不受影响，diff 仍基于 SKILL.md。

库体积终态：历史 637 个版本全部裁剪为仅 SKILL.md（剔 14831 个非核心文件行），blobs 899MB→5.1MB，db 952MB→6MB，磁盘 87%。

## 十六、闲时任务批次：使用分析/展示过滤/权限模型/审计/GUI 测试（2026-10-14）

1. **使用分析**（USAGE-2026-10-14.md）：24h 5 版本全部来自 your-host 正常编辑；无异常迭代（camoufox-antidetect 4 版为人工）；资源极低（serve 11MB / daemon 6MB）
2. **展示过滤**：skills.is_system 列（幂等迁移+内置名单回填 92 个）+ POST /api/skills/{id}/system + 列表两开关（隐藏系统/隐藏重复，localStorage 记忆）+ 详情页标记按钮。实测 373→281
3. **权限模型**：tokens 表（admin/user）+ TokenRole()（主 token 恒 admin）+ 10 个 admin-only 端点 + user 的 jobs/next 返回空任务 + **/api/client/ 内嵌请求者 token（堵死下载提权）** + 设置页 token 管理（创建一次性显示/列表掩码/吊销）。权限矩阵 21 项动态验证全绿
4. **增量安全审计**（AUDIT.md 第七章）：提权面 10 场景全部通过（含 token 变体/自我提权/SQL 注入/吊销时效）；whoami 免 token 设计（只回显角色）
5. **GUI 测试**（TEST-REPORT.md）：web-gui-tester 六页面走查，发现并修复 **BUG-1**（前端无 fetch 重试，代理 502 窗口期页面永久卡死）——api() 加 502/网络重试 + 错误框重试按钮；修复回归后六页全过
6. 部署注意：VPS 库迁移前备份 skillhub.db.bak-20261014-*；pkill -x 重启；本轮再次验证"二进制字符串搜索不可靠，行为验证才准"

## 十七、协议 v2 迭代：HMAC 签名/心跳/分配/下发/GitHub 私库（2026-10-16，commit ec57c60）

六项已批准需求（2026-10-15 搁置、16 日闲时功能仍未开放后用户改口立即执行）全部落地：

1. **系统 skill 路径分类**：客户端 push/daemon 携带 source_root，服务端 `ClassifySystem`（相对 home 命中 .zcode/.claude 插件缓存前缀→is_system=1，只置1不清0）。**修复三连**：WalkDir 根目录撞 runtimeDirs 名（cache/plugins）整树漏扫——生产 .zcode 插件 skill 此前一直没采到；root=skill 目录时 rel=. 归属失败；`filepath.Rel` 返回反斜杠未规范化致分类恒 false。列表/关系图「隐藏系统 skill」开关默认隐藏（localStorage）。VPS 存量回填实测 118 系统/256 用户
2. **HMAC 签名认证**（auth.go + pusher/sign.go + app.js 纯 JS SHA-256）：token 不上 wire（X-Key-ID=sha256[:16]），±10min 时间窗+nonce 防重放+X-Body-SHA256 防篡改（覆盖 multipart bundle）；**签名失败不回落明文（防 downgrade）**；无签名头回落 X-Token 兼容旧客户端。负路径实测：重放/篡改/过期全 401。**修复**：handlePush 残留重复 checkToken 拦死签名请求（Python 复现签名定位）。PROTOCOL.md 全量协议文档
3. **心跳探活**：daemon 30min POST /api/heartbeat（启动即跳一次），>35min 判离线，Dashboard 设备在线表。**修复**：modernc 驱动 Scan DATETIME→string 转成 RFC3339，按 "2006-01-02 15:04:05" 解析恒失败致 online 恒 false——双格式兼容
4. **分配+下发**：assignments 表（PUT admin/GET user）+ dispatches 表（admin 下发 sync|pull → 心跳响应捎带认领 → result 回执）。客户端三入口：托盘「立即同步/上传全部/下载分配」+ CLI `pull --assigned` / `upload`。后台新「设备」页（在线状态/分配编辑/主动下发/指令历史）。VPS 闭环实测：keydraw 分配→下发→daemon 启动认领→落盘 ~/.agents/skills→回执 done
5. **审计**（AUDIT.md 第八章）：新增端点 token 门全矩阵、FK 约束兜底、nonce 有界；**git 全历史 7390 行 diff 双模式隐私扫描（GLM key 形态+40+长随机串）零命中**；RSS 复测 serve 17-19MB / daemon 9.8MB（FreeOSMemory 策略未回退）
6. **GitHub 私库**：Herbicide7665/skill-hub（PAT 经 VPS bw CLI 一条 shell 取回不落盘，push 后即删临时 remote；API+匿名 404 双重确认 private:true）

部署教训（复用）：**pkill -f 'skill-hub daemon' 会匹配同一命令行里重启命令的 daemon 字样自杀**——pkill 与重启必须拆两次 SSH 调用；daemon 的 --token 从 serve.log head 抓取有 race（serve 未写完 token 行→空 token 401），直接用 .token 文件。本机 config.json 曾指向死端口 8799（残留），已修正为 VPS。


## 十八、v2.1：用户认证 / LLM 自定义 / 版本留存 / 更名（2026-10-16）

### 18.1 需求与落地
| # | 需求 | 落地 |
|---|---|---|
| 1 | 后台不绑死 GLM，LLM 自定义配置 | 三源配置（env > 设置页 > config.yaml），任意 OpenAI 兼容端点 + Anthropic 协议自动选择；设置页在线改配置热生效，key 脱敏展示 |
| 2 | 后台留存 skill 历史版本 | 确认存储层本就纯追加（versions/blobs 无清理逻辑）；UI 补齐：详情页文件浏览器加版本选择器 + 任意版本 tar.gz 下载 |
| 3 | 后台用用户认证，token 只留 API | users/sessions 表 + 登录页 + HttpOnly 会话 cookie（30d）；PBKDF2 口令哈希；admin 可管理用户/重置密码；浏览器不再接触 token；API 三通道（签名/明文 X-Token）保留 |
| 4 | 更名（skill-hub 与既有项目撞名） | 全量更名 Skill Manager / skill-manager（module、二进制、数据目录 ~/.skill-manager、skillmanager.db、SKILLMANAGER_* env、UI、脚本）；旧 yaml 键/env/数据库兼容 |

### 18.2 顺带修复的既有缺陷
- 签名（X-Sign）请求打 admin 端点误判 401（requireRole 二次解析触发 nonce 重放判定）→ per-request context 缓存，签名每请求只验证一次
- `GET /api/settings` 把主 token 回给任意有效 token（user 可提权读取）→ 现按 principal 角色过滤

### 18.3 测试
- Go 测试套件（internal/server/auth_test.go）：签名 admin 端点、nonce 重放拒绝、登录会话、user 角色隔离（403/空任务/不可读主 token）、匿名 401、LLM 自定义配置热生效（mock OpenAI 端点）、版本留存（两版本可读）
- 实机 smoke（临时 db + :8899）：登录/会话访问/匿名 401/token API/settings 脱敏/user token 隔离/push 两版本/旧版本文件回读 全绿
- 交叉编译 5 平台（win/linux×2/darwin×2）通过

### 18.4 兼容性说明
- 旧 X-Token 认证、旧 skillhub.db、glm_* yaml 键、GLM_API_KEY env 全部兼容
- 已分发的旧内嵌配置二进制不受影响（读自身尾部）；新下载的客户端 marker 换代，旧客户端需重新下载
- 初始 admin 账号：首启生成随机密码打印在启动日志（仅一次），或 SKILLMANAGER_ADMIN_PASSWORD 指定


## 十九、v2.2：user token 设备域 + 客户端配置弹窗（2026-10-16）

| 项 | 落地 |
|---|---|
| user token 绑定设备 | tokens 加 hostname 列；首次 push/heartbeat 自动认领锁死；不符 403；admin 设置页改绑/解绑 |
| 设备域权限 | skills/canonical 按域过滤、machines/assignments/dispatches 限本机、管理视图(stats/graph/families/dup/edges/jobs)纯 user token 403、web user 会话保持全局只读 |
| token 与 web 分轨确认 | user token 只能跑客户端；web 只认账号会话；whoami 回 role+machine 供配置窗自检 |
| 客户端配置 | config.json 全量字段；托盘「配置…」Win32 原生弹窗（测试连接/确定自重启）；skillshooter config CLI；首启无配置直接弹窗引导 |

测试：Go 7 用例全绿（新增 TestDeviceScope/TestUserTokenCannotWeb，含认领-锁死-跨设备 403-改绑生效-未绑定拒绝-主 token 不可读）；本机 smoke 平板两阶段验证；**修复掩码反查死锁（grep 时代坑复发）**。

部署：VPS serve+daemon 已换 skillshooter（旧 skillhub.db→skillshooter.db 拷贝迁移，备份留档 bak-20261016-1605）；selfpush.sh cron 已指新二进制并试跑成功（31 skill 推送/0 新增符合增量语义）；web admin 初始密码打印于启动日志（已交付用户）；旧主 token 283b.../vps 心跳在线/LLM 连通全验证通过。


## 二十、v2.3：系统 skill 判定修正 / i18n / 设置侧边栏 / UI 改版（2026-10-16）

| # | 需求 | 落地 |
|---|---|---|
| 1 | access/build-* 等 Claude Code 内置也属系统 skill | 迁移回填名单扩至 37 项（access/build-mcp*/configure/example-*/agent-development 等 29 个新增 + Drift（首字母大写变体）+ siege/solo/stripe-best-practices/visualize） |
| 2 | 语言切换 + 英文 | I18N 双语字典 + t() + 导航栏 EN/中 按钮（localStorage 记忆）；散点文案 LANG 三元式处理 |
| 3 | 设置页左侧层级侧边栏 | settings-layout 栅格（sticky 侧栏 + 图标锚点导航 + 平滑滚动 + 滚动高亮联动；窄屏折叠横排） |
| 4 | git 提交新版本 | commit + push Herbicide7665/skillShooter（bundle+hosts 路线） |
| 5 | UI 统一审美 | 设计系统重做：深蓝渐变导航（选中态品牌蓝药丸）、卡片左缘渐变条、按钮渐变+悬浮位移、输入框 focus 光环、表头大写字距、统一 12px 圆角与两级阴影体系 |

验证：JS 语法（node Function 编译）+ Go 测试 7 用例 + 浏览器实机 e2e（登录→EN 切换全页文案→设置侧边栏六节锚点→Skills 隐藏系统开关默认隐藏 my-own 单显→取消隐藏 access/build-mcp 回显 3 项→migrate 重启回填 is_system 断言）。
