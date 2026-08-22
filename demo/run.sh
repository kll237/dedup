#!/usr/bin/env bash
# 复现 README「效果演示」中的全部真实产物。
# 前置：已在本仓库根目录执行过 `go build -o dedup.exe .`（或 go build 生成 dedup）。
set -e
cd "$(dirname "$0")/.."

# 构建（若已存在二进制则跳过）
if [ -x ./dedup.exe ]; then BIN=./dedup.exe
elif [ -x ./dedup ]; then BIN=./dedup
else go build -o dedup.exe . && BIN=./dedup.exe; fi

rm -rf demo/output
mkdir -p demo/output

"$BIN" -path demo/input -mode both -format html -out demo/output/report.html 2>demo/output/progress.txt
"$BIN" -path demo/input -mode both -csv demo/output/report.csv 2>/dev/null
"$BIN" -path demo/input -mode both > demo/output/stdout.txt 2>>demo/output/progress.txt
"$BIN" -path demo/input -delete -dry-run >/dev/null 2>demo/output/delete_exact_preview.txt
"$BIN" -path demo/input -delete-similar -dry-run >/dev/null 2>demo/output/delete_similar_preview.txt
go test -v ./... > demo/TEST_OUTPUT.txt 2>&1

echo "复现完成 -> 见 demo/output/ 与 demo/TEST_OUTPUT.txt"
