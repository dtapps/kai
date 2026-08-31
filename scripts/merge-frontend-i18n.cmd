@echo off
REM 合并前端 i18n 拆分文件到主文件（Windows 版本）
REM 用法: scripts\merge-frontend-i18n.cmd

setlocal enabledelayedexpansion

set "SPLIT_DIR=%~dp0..\frontend\src\locales\split"
set "OUTPUT_DIR=%~dp0..\frontend\src\locales"

echo === merging frontend i18n ===

for %%L in (zh-CN en-US) do (
    echo merging %%L ...
    set "SPLIT_LOCALE_DIR=!SPLIT_DIR!\%%L"
    if not exist "!SPLIT_LOCALE_DIR!" (
        echo error: directory not found: !SPLIT_LOCALE_DIR!
        exit /b 1
    )

    REM Windows 下 jq 不展开 glob，用 for 收集所有 json 文件路径
    set "FILES="
    for %%F in ("!SPLIT_LOCALE_DIR!\*.json") do set "FILES=!FILES! %%F"

    REM 用 jq 合并 split 目录下的所有 json
    jq -s "reduce .[] as $f ({}; . * $f)" !FILES! > "%TEMP%\fe-i18n-%%L-merged.json"

    REM 添加注释头并按键排序
    jq "to_entries | ([{\"key\":\"_comment\",\"value\":\"⚠️ 此文件由 scripts/merge-frontend-i18n.cmd 自动生成，请勿手动编辑！修改请编辑 split/ 目录下的文件后重新合并。\"}] + .) | sort_by(.key) | from_entries" "%TEMP%\fe-i18n-%%L-merged.json" > "!OUTPUT_DIR!\%%L.json"

    del "%TEMP%\fe-i18n-%%L-merged.json" 2>nul

    for /f %%A in ('jq "length" "!OUTPUT_DIR!\%%L.json"') do (
        echo   -^> frontend\src\locales\%%L.json %%A keys
    )
)

echo done.
