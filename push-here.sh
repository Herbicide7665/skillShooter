#!/bin/sh
# 本机全盘 skill 推送到 VPS hub（与设置页一键引导命令同口径）
DIR=$(cd "$(dirname "$0")" && pwd)
TOK=$(cat "$DIR/.token")
SRV=http://your-server:8790
H=your-host
"$DIR/skillrelay.exe" push --server "$SRV" --token "$TOK" --path "~/your-user/.agents/skills" --hostname $H
"$DIR/skillrelay.exe" push --server "$SRV" --token "$TOK" --path "~/your-user/.claude/skills" --hostname $H
"$DIR/skillrelay.exe" push --server "$SRV" --token "$TOK" --path "~/your-user/.claude/plugins" --hostname $H
"$DIR/skillrelay.exe" push --server "$SRV" --token "$TOK" --path "~/your-user/.zcode/cli/plugins/cache" --hostname $H
echo "== push 完成，hub: $SRV =="
