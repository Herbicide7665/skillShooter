# 🎯 SkillShooter

[English](README.md) | 中文

[![License](https://img.shields.io/badge/license-Apache--2.0-brightgreen.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.21-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20Linux%20%7C%20macOS-blue)](#环境要求)
[![Binary](https://img.shields.io/badge/%E4%BA%8C%E8%BF%9B%E5%88%B6-%E5%8D%95%E6%96%87%E4%BB%B6%E9%9B%B6%E4%BE%9D%E8%B5%96-orange)](#架构)
[![i18n](https://img.shields.io/badge/UI-%E4%B8%AD%E6%96%87%20%7C%20English-green)](#核心功能)

**跨机器的 Agent Skill 管理平台。** 把散落在各台机器、各个 AI 编程工具（ZCode / Claude Code / 通用 `.agents` 体系）里的 skill 集中到一个自托管服务：统一收录、全量版本历史、智能去重合并、关系图谱、跨机分发与远程调用。服务端与客户端是同一个 Go 二进制，无任何运行时依赖。

![总览仪表盘](docs/screenshot-dashboard.png)

## 环境要求

- 源码构建需要 Go 1.21+（`go.mod` 声明 go 1.21）
- 运行零外部依赖：SQLite 经 `modernc.org/sqlite` 纯 Go 内嵌，无需安装数据库或运行时
- 平台：Windows / Linux / macOS（系统托盘为 Windows 专属，其余平台用 CLI 常驻）

## 安装

**方式一：GitHub Codespaces 一键云实例（零服务器）**

[![Open in GitHub Codespaces](https://github.com/codespaces/badge.svg)](https://codespaces.new/Herbicide7665/skillShooter?quickstart=1)

点击按钮，Codespaces 会开一个云端沙箱，自动完成构建并后台启动服务端（端口 8790 自动转发、自动打开 web 控制台）：

- 初始管理员账号密码在启动终端打印一次（也写在实例内 `serve.log`），登录后请尽快修改
- 沙箱未配置 LLM key，去重/合并/关系归纳等智能功能自动降级，其余功能完整可玩
- 数据保存在沙箱内，删除 Codespace 即销毁；免费额度每月 120 核时
- 仓库私有期间仅协作者可拉起；转为 Public 后任何人都能一键获得自己的临时实例

**方式二：下载现成二进制**

[Releases](https://github.com/Herbicide7665/skillShooter/releases/latest) 提供 windows / linux amd64+arm64 / darwin amd64+arm64 五平台构建，每平台分 `skillshooter-server-*` 与 `skillshooter-client-*` 两个名字——**同一二进制**，仅作角色指引：服务端取 `server`，接入机器取 `client`（Linux/macOS 首次运行先 `chmod +x`）。

**方式三：源码构建**

```bash
go build -o skillshooter .
```

## 配置

全部数据与配置都在二进制同目录：`skillshooter.db`（自动创建）+ `config.yaml`（模板见 [config.yaml.example](config.yaml.example)）。

服务端：

- 监听与存储：`--addr`（默认 `0.0.0.0:8790`）、`--db`（默认二进制同目录 `skillshooter.db`）
- 初始管理员密码可用环境变量 `SKILLSHOOTER_ADMIN_PASSWORD` 预设，否则随机生成并仅打印一次

客户端公共参数：`--server` / `--token`，或环境变量 `SKILLSHOOTER_SERVER` / `SKILLSHOOTER_TOKEN`。

LLM（可选，开启去重判定/合并建议/关系归纳）——`config.yaml`：

```yaml
llm_api_key: "你的key"
llm_base_url: "https://api.openai.com/v1"   # 任意 OpenAI 兼容端点；含 /anthropic 时自动走 Anthropic 协议
llm_model: "your-model"
```

优先级：环境变量（`LLM_API_KEY`，兼容 `GLM_API_KEY` / `ZHIPUAI_API_KEY`；`SKILLSHOOTER_LLM_BASE_URL`；`SKILLSHOOTER_LLM_MODEL`）> web 设置页保存 > `config.yaml`。`model` 未配置时智能功能降级不可用，其余功能不受影响。设置页有「测试连通性」按钮。

## 快速开始

**1. 启动服务端**

```bash
./skillshooter serve                # 默认 0.0.0.0:8790
# 可选参数: --addr 0.0.0.0:9000 --db /data/skillshooter.db
```

首次启动会打印 **API token** 和**初始管理员账号密码**（仅此一次）。

**2. 登录 web 后台**

打开 `http://SERVER_IP:8790` → admin 登录。设置页可：创建绑定设备的 user token、复制新机器接入命令、配置并测试 LLM 连通性。

**3. 机器接入（每台要纳管的机器，一条命令）**

```bash
# 在 hub 设置页复制即可（token 已注入）：
curl -fsSL http://SERVER:8790/agent.sh | sh -s -- --server http://SERVER:8790 --token TOKEN
# Windows 用设置页复制的 agent.ps1 引导命令
```

引导脚本自动按 OS/架构从服务端 `/dl/` 下载二进制到 `~/.skillshooter/`，发现 `~/.agents`、`~/.claude`、`~/.zcode` 下的 skill 目录并全量推送；幂等可重复执行，`--daemon` 参数可直接进入常驻模式。也可手动推送单个目录：

```bash
skillshooter push --server http://SERVER:8790 --token TOKEN --path ~/.agents/skills --hostname my-laptop
```

**4. 常驻客户端（推荐每台机器都跑）**

```bash
skillshooter daemon     # 设置页可复制完整命令
```

默认 30s 增量监控推送、5min 自动应用云端合并、轮询领取远程任务、30min 心跳探活，均可用参数调整。Windows 下常驻为托盘图标，托盘「配置…」弹窗可视化修改服务端地址 / Token / 设备名 / 各间隔 / 监控目录；首次无配置启动直接弹出配置向导。

**5. 配置 LLM（可选）**

见[配置](#配置)一节；未配置时其余功能完整可用。

## 核心功能

**统一收录**——客户端全盘递归发现含 `SKILL.md` 的目录（插件缓存多层嵌套也能处理）自动推送，所有机器的 skill 集中在一张清单：搜索、按机器/家族过滤、版本数与内容哈希一目了然。仅收录 SKILL.md 核心文档，本机脚本与数据产物永不入库：

![统一收录：多机器 skill 集中清单](docs/screenshot-skills.png)

**全量版本历史**——每次 push 按内容寻址留存版本：时间线回看演变路线，任意两版本 unified diff，历史版本可在线浏览、打包下载：

![全量版本历史：版本时间线与任意两版本 diff](docs/screenshot-skill-detail.png)

**智能去重合并**——精确哈希分组 + 名称/描述相似度预筛 + LLM 逐对判定（置信度与理由随组展示）；合并建议并排预览、与原版 diff 对照，人工点「采纳」才落新版本，绝不自动覆盖：

![智能去重合并：LLM 判定与合并建议预览](docs/screenshot-dedup.png)

**关系图谱**——解析 SKILL.md 正文引用自动建边，LLM 归纳「相关 / 调用链」与家族；力导向图按家族着色，点图例聚焦家族，双击节点跳详情：

![关系图谱：按家族着色的力导向图](docs/screenshot-graph.png)

**跨机分发与远程调用**——在设备页给机器分配 skill，下发指令随心跳落盘到目标机（或 `pull --assigned` 手动拉取）；「远程调用」把任务交给目标机的 claude/zcode 执行并回传报告：

![跨机分发：设备管理与下发部署](docs/screenshot-devices.png)

![远程调用：任务下发与执行报告](docs/screenshot-jobs.png)

智能功能依赖 LLM 但它是可选的：未配置时去重退化到预筛分组，合并与关系归纳不可用，其余功能完整可用。

### 多机协作与安全

- **双轨身份**：web 后台 = 账号登录（admin 全权 / user 全局只读）；API token = 客户端凭据，两者互不通用
- **一 token 一设备**：user token 创建时可留空绑定，客户端首次 push/心跳自动认领锁死；只能读写「本机推送 + 分配给本机」的 skill，越权 403，管理员可改绑
- **传输与存储**：API 请求 HMAC-SHA256 签名（±10 分钟时间窗 + nonce 防重放）；web 密码 PBKDF2-HMAC-SHA256 存储；登录失败 5 次/60 秒锁定
- **客户端无提权面**：客户端下载接口内嵌请求者自己的 token——普通用户拿到的客户端永远无法提权

### 客户端体验

- Windows 托盘常驻 + 可视化配置弹窗（见[快速开始](#快速开始)第 4 步）；跨平台 CLI：`push` / `pull` / `daemon` / `config`
- 中英双语界面一键切换；系统自带 / 重复 skill 一键隐藏

## 架构

```
 ┌──────────────┐    push / pull / 心跳 / 任务认领      ┌──────────────────────────┐
 │  机器 A       │ ──────────(HMAC 签名请求)──────────▶ │  服务端: skillshooter serve│
 │  daemon + CLI │ ◀─────────(分配的 skill /           │  · SQLite WAL 内容寻址存储 │
 └──────────────┘           合并成果 / 远程任务)─────── │  · web 后台（账号会话登录） │
 ┌──────────────┐                                      └────────────▲─────────────┘
 │  机器 B …     │ ───────────────── 同上 ──────────────────────────┘
 └──────────────┘                                        浏览器 http://server:8790
```

客户端↔服务端的全部 `/api/*` 通讯协议（认证三模式、签名算法、端点清单）见 [PROTOCOL.md](PROTOCOL.md)。

## CLI 一览

| 子命令 | 作用 |
|---|---|
| `serve` | 启动 web 服务端（默认 `:8790`） |
| `push` | 扫描目录推送 skill（递归发现 SKILL.md） |
| `pull` | 拉取 hub 最新版本到本地（默认跳过已存在，同名多机自动 `名字@机器` 区分） |
| `daemon` | 常驻客户端：增量监控推送 + 自动应用云端合并 + 领取远程任务 |
| `config` | 终端配置向导（回车保留当前值） |
| `upload` | `push` 的别名 |

## 数据与运维

- 全部数据在二进制同目录：`skillshooter.db`（SQLite/WAL，内容寻址 blob 存储）+ `config.yaml`
- **备份 = 拷走一个 db 文件；迁移 = 拷走 db + exe**
- 无外部依赖，端口放行防火墙即可；公网部署建议 nginx 反代加 TLS

## 测试

```bash
go test ./...
```

## 已知限制

- 明文 HTTP 传输，公网部署请套 TLS 反向代理
- diff 仅针对 SKILL.md；其余文件变更体现在版本时间线（树哈希变化）但无逐文件 diff
- LLM 判定/合并为逐对串行调用，skill 数量大时扫描耗时随之增长
- 托盘为 Windows 专属，Linux/macOS 用 CLI + systemd/nohup 常驻

## License

以 [Apache-2.0](LICENSE) 开源。
