@echo off
rem 本机全盘 skill 推送到 VPS hub（双击执行；与一键引导命令同口径）
rem 真实地址/机器名放 .push-env（不入库）：SRV=http://...  H=...
cd /d "%~dp0"
if exist .push-env (
  for /f "usebackq eol=# tokens=1,2 delims==" %%a in (".push-env") do set %%a=%%b
)
set /p TOK=<.token
if "%SRV%"=="" set SRV=http://your-server:8790
if "%H%"=="" set H=your-host
skillshooter.exe push --server %SRV% --token %TOK% --path "%USERPROFILE%\.agents\skills" --hostname %H%
skillshooter.exe push --server %SRV% --token %TOK% --path "%USERPROFILE%\.claude\skills" --hostname %H%
skillshooter.exe push --server %SRV% --token %TOK% --path "%USERPROFILE%\.claude\plugins" --hostname %H%
skillshooter.exe push --server %SRV% --token %TOK% --path "%USERPROFILE%\.zcode\cli\plugins\cache" --hostname %H%
echo == push 完成，hub: %SRV% ==
