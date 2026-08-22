@echo off
REM 复现 README「效果演示」中的全部真实产物（Windows）。
cd /d %~dp0\..
if not exist dedup.exe go build -o dedup.exe .
if exist demo\output rmdir /s /q demo\output
mkdir demo\output
dedup.exe -path demo/input -mode both -format html -out demo/output/report.html 2>demo/output/progress.txt
dedup.exe -path demo/input -mode both -csv demo/output/report.csv 2>nul
dedup.exe -path demo/input -mode both > demo/output/stdout.txt 2>>demo/output/progress.txt
dedup.exe -path demo/input -delete -dry-run >nul 2>demo/output/delete_exact_preview.txt
dedup.exe -path demo/input -delete-similar -dry-run >nul 2>demo/output/delete_similar_preview.txt
go test -v ./... > demo/TEST_OUTPUT.txt 2>&1
echo 复现完成 -^> 见 demo/output/ 与 demo/TEST_OUTPUT.txt
