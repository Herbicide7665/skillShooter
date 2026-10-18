#!/bin/sh
# skillshooter 新机器引导：下载二进制到 ~/.skillshooter/ 并全盘收集本机所有 agent 工具的 skill 推送到 hub
# 用法: curl -fsSL http://HUB/agent.sh | sh -s -- --server http://HUB --token TOKEN [--hostname 名字] [--paths 额外路径,逗号分隔]
set -eu

SERVER=""
TOKEN=""
HOSTNAME_ARG=""
EXTRA_PATHS=""
DAEMON=0
while [ $# -gt 0 ]; do
  case "$1" in
    --server)   SERVER="$2"; shift 2 ;;
    --token)    TOKEN="$2"; shift 2 ;;
    --hostname) HOSTNAME_ARG="$2"; shift 2 ;;
    --paths)    EXTRA_PATHS="$2"; shift 2 ;;
    --daemon)   DAEMON=1; shift ;;
    *) shift ;;
  esac
done
if [ -z "$SERVER" ] || [ -z "$TOKEN" ]; then
  echo "需要 --server 与 --token 参数（在 hub 设置页复制完整命令即可）" >&2
  exit 1
fi

HUBDIR="$HOME/.skillshooter"
mkdir -p "$HUBDIR"

OS="$(uname -s)"
ARCH="$(uname -m)"
BIN=""
EXE="skillshooter-client"
case "$OS" in
  Linux)
    case "$ARCH" in
      aarch64|arm64) BIN="skillshooter-client-linux-arm64" ;;
      *)             BIN="skillshooter-client-linux-amd64" ;;
    esac
    ;;
  Darwin)
    case "$ARCH" in
      arm64) BIN="skillshooter-client-darwin-arm64" ;;
      *)     BIN="skillshooter-client-darwin-amd64" ;;
    esac
    ;;
  MINGW*|MSYS*|CYGWIN*)
    BIN="skillshooter-client.exe"
    EXE="skillshooter-client.exe"
    ;;
  *)
    echo "不支持的系统: $OS（请手动到 $SERVER/dl/ 下载二进制）" >&2
    exit 1
    ;;
esac

echo "==> 下载 $BIN -> $HUBDIR/$EXE"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL -o "$HUBDIR/$EXE" "$SERVER/dl/$BIN"
else
  wget -q -O "$HUBDIR/$EXE" "$SERVER/dl/$BIN"
fi
if [ "$EXE" = "skillshooter-client" ]; then
  chmod +x "$HUBDIR/$EXE"
fi
SH="$HUBDIR/$EXE"

HN="$HOSTNAME_ARG"
if [ -z "$HN" ]; then
  HN="$(hostname 2>/dev/null || echo unknown)"
fi

# 全盘收集：所有已存在的常见 agent skill 位置（跨工具）
PATHS=""
for d in "$HOME/.agents/skills" "$HOME/.claude/skills" "$HOME/.claude/plugins" "$HOME/.zcode/cli/plugins/cache"; do
  if [ -d "$d" ]; then
    PATHS="$PATHS $d"
  fi
done
if [ -n "$EXTRA_PATHS" ]; then
  OLDIFS="$IFS"
  IFS=","
  for d in $EXTRA_PATHS; do
    if [ -d "$d" ]; then
      PATHS="$PATHS $d"
    fi
  done
  IFS="$OLDIFS"
fi
if [ -z "$PATHS" ]; then
  echo "未发现任何 skill 目录；用 --paths 手动指定后重试" >&2
  exit 1
fi

FAIL=0
if [ "$DAEMON" = "1" ]; then
  echo "==> 以常驻模式运行（增量监控 + 应用云端合并 + 远程调用接收）"
  exec "$SH" daemon --server "$SERVER" --token "$TOKEN" --hostname "$HN"
fi
for d in $PATHS; do
  echo "==> push $d （机器名: $HN）"
  if ! "$SH" push --server "$SERVER" --token "$TOKEN" --path "$d" --hostname "$HN"; then
    FAIL=1
  fi
done

echo "==> 完成。二进制保留在 $HUBDIR，以后重复执行安全（幂等）。"
echo "    反向分发（把 hub 合并成果拉回本机）:"
echo "    $SH pull --server $SERVER --token \$TOKEN --dest ~/.agents/skills"
exit $FAIL
