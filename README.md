# dedup — 本地文件去重 / 相似图片查找 CLI

一个用 **Go** 编写的命令行工具，用来：
1. **精确去重**：按文件内容（SHA-256）找出完全相同的文件；
2. **相似图片查找**：用感知哈希（dHash）找出肉眼几乎一样的图片（即使被压缩 / 改尺寸 / 调了亮度）。

全部使用 **Go 标准库 + Windows API** 实现，**零第三方依赖**，编译出来是单个二进制文件，可直接分发。

## 特性
- 🔍 精确去重（SHA-256 内容哈希），并发计算，自动按可回收空间排序
- 🖼️ 相似图片查找（64 位 dHash + 汉明距离），支持 JPG/PNG/GIF/BMP/WebP/TIFF
- 🗑️ **安全删除**：把重复文件移入系统回收站（绝不 `rm`），支持 `--dry-run` 预览
- 📄 三种报告格式：终端文本 / JSON / HTML（HTML 带图片缩略图）
- ⚡ 并发扫描与哈希（worker pool），可指定 `-workers`
- 🧹 忽略规则：隐藏文件、指定目录（`.git` / `node_modules`）、最小/最大文件大小、扩展名过滤

## 安装

### 从源码
```bash
git clone https://github.com/<your-name>/dedup.git
cd dedup
go build -o dedup .
```

### go install
```bash
go install dedup@latest
```

## 使用

```bash
# 1) 找出某目录下所有精确重复文件（默认 text 报告，输出到终端）
dedup -path "D:/Photos"

# 2) 同时查精确重复 + 相似图片
dedup -path "D:/Photos" -mode both

# 3) 只查相似图片，汉明阈值调到 8（更严格）
dedup -path "D:/Photos" -mode image -threshold 8

# 4) 生成 HTML 报告（带缩略图），写到文件
dedup -path "D:/Photos" -format html -out report.html

# 5) 机器可读的 JSON 报告
dedup -path "D:/Photos" -format json -out report.json

# 6) 预览将要删除的重复文件（不真正删除）
dedup -path "D:/Photos" -delete -dry-run

# 7) 真正把重复副本移入回收站（每组保留按路径排序的第一个）
dedup -path "D:/Photos" -delete
```

### 常用参数
| 参数 | 说明 | 默认 |
|------|------|------|
| `-path` | 要扫描的路径（可多次指定），也可用位置参数 | — |
| `-mode` | `exact` / `image` / `both` | `both` |
| `-format` | `text` / `json` / `html` | `text` |
| `-out` | 报告输出文件（默认 stdout） | 空 |
| `-threshold` | 相似图片汉明距离阈值（0-64，越小越严格） | `10` |
| `-min-size` / `-max-size` | 文件大小过滤，如 `1KB` / `10MB` | `0` |
| `-workers` | 并发数（默认 = CPU 核数） | `0` |
| `-skip-hidden` | 跳过隐藏文件和目录 | `true` |
| `-ignore` | 跳过的目录名，逗号分隔 | `.git,.node_modules` |
| `-delete` | 把精确重复的额外副本移入回收站 | `false` |
| `-dry-run` | 仅预览待删除文件 | `false` |

## 架构

```
main.go          命令行解析、流程编排、删除决策
├── scan/        递归遍历文件系统，应用过滤规则
├── hash/        内容哈希（SHA-256）+ 并发分组（精确去重）
├── imageph/     图片解码 → 灰度 → 缩放 → dHash → 并查集分组（相似图片）
├── report/      文本 / JSON / HTML 三种报告渲染（HTML 内嵌缩略图）
├── trash/       调用系统回收站（Windows: SHFileOperationW；其他: gio/trash-put）
└── result/      跨包共享的数据结构
```

### 关键设计
- **精确去重**：`hash.FindExact` 用固定数量的 goroutine 并发计算 SHA-256，结果按内容哈希归组，每组文件数 > 1 即为重复。
- **相似图片**：先算每张图的 64 位 dHash 指纹，再用**并查集（union-find）**把汉明距离 ≤ 阈值的图片合并成组，时间复杂度 O(n²)，对常规相册规模足够快。
  - dHash 流程：解码 → 灰度化 → 缩放到 9×8 → 比较相邻像素亮度 → 得到 64 位指纹。对缩放、压缩、轻微调色鲁棒。
- **安全删除**：Windows 下通过 `shell32.dll!SHFileOperationW`（`FOF_ALLOWUNDO`）把文件移入回收站，可还原；绝不调用 `os.Remove`。

## 性能
- 内容哈希瓶颈在磁盘 I/O，并发读取可充分利用多核 / 多磁盘。
- 相似图片为 O(n²) 两两比较；对上万张图片建议在 CI/脚本里分批，或提高 `-threshold` 先用精确哈希去重减少候选。

## License
[MIT](LICENSE)
