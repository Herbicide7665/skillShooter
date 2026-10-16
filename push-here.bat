@echo off
rem 本机全盘 skill 推送到 VPS hub（双击执行；与一键引导命令同口径）
cd /d "%~dp0"
set /p TOK=<.token
set SRV=http://your-server:8790
skill-manager.exe push --server %SRV% --token %TOK% --path "~/your-user/.agents/skills" --hostname your-host
skill-manager.exe push --server %SRV% --token %TOK% --path "~/your-user/.claude/skills" --hostname your-host
skill-manager.exe push --server %SRV% --token %TOK% --path "~/your-user/.claude/plugins" --hostname your-host
skill-manager.exe push --server %SRV% --token %TOK% --path "~/your-user/.zcode/cli/plugins/cache" --hostname your-host
echo == push 完成，hub: %SRV% ==
pause
