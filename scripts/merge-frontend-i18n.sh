#!/bin/bash
# 合并前端 i18n 拆分文件到主文件
# 用法: ./scripts/merge-frontend-i18n.sh
#
# 目录结构:
#   frontend/src/locales/split/
#   ├── zh-CN/
#   │   ├── errors.json
#   │   ├── logs.json
#   │   ├── messages.json
#   │   ├── notifications.json
#   │   └── other.json
#   └── en-US/
#       └── (同上)
#
# 输出:
#   frontend/src/locales/zh-CN.json
#   frontend/src/locales/en-US.json

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
SPLIT_DIR="$PROJECT_DIR/frontend/src/locales/split"
OUTPUT_DIR="$PROJECT_DIR/frontend/src/locales"

merge_locale() {
    local locale="$1"
    local split_dir="$SPLIT_DIR/$locale"
    local output_file="$OUTPUT_DIR/$locale.json"
    
    if [ ! -d "$split_dir" ]; then
        echo "错误: 目录不存在 $split_dir"
        return 1
    fi
    
    echo "合并 $locale ..."
    
    local tmp_file=$(mktemp)
    local first=true
    for f in "$split_dir"/*.json; do
        if [ -f "$f" ]; then
            if [ "$first" = true ]; then
                cp "$f" "$tmp_file"
                first=false
            else
                local merged=$(jq -s '.[0] * .[1]' "$tmp_file" "$f")
                echo "$merged" > "$tmp_file"
            fi
        fi
    done
    
    jq 'to_entries | [{"key":"_comment","value":"⚠️ 此文件由 scripts/merge-frontend-i18n.sh 自动生成，请勿手动编辑！修改请编辑 split/ 目录下的文件后重新合并。"}] + (.[1:] | sort_by(.key)) | from_entries' "$tmp_file" > "$output_file"
    rm -f "$tmp_file"
    
    local count=$(jq 'length' "$output_file")
    echo "  -> $output_file ($count keys)"
}

# 主流程
echo "=== 前端 i18n 文件合并 ==="
echo ""

merge_locale "zh-CN"
merge_locale "en-US"

echo ""
echo "完成!"
