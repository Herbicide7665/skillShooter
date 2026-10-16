#!/bin/sh
# 本机全盘 skill 推送到 VPS hub（与一键引导命令同口径）
# 真实地址/机器名放 .push-env（不入库）：SRV=http://...  H=...
DIR=$(cd "$(dirname "$0")" && pwd)
[ -f "$DIR/.push-env" ] && . "$DIR/.push-env"
TOK=$(cat "$DIR/.token")
SRV=${SRV:-http://your-server:8790}
H=${H:-your-host}
HOME_DIR=${HOME}
for p in "$HOME_DIR/.agents/skills" "$HOME_DIR/.claude/skills" "$HOME_DIR/.claude/plugins" "$HOME_DIR/.zcode/cli/plugins/cache"; do
  [ -d "$p" ] && "$DIR/skillshooter.exe" push --server "$SRV" --token "$TOK" --path "$p" --hostname "$H"
done
echo "== push 完成，hub: $SRV =="
