---
title: 为什么"删除"要用回收站而不是 rm
---

# 为什么"删除"要用回收站而不是 `rm`

> 本文对应 [dedup](https://github.com/kll237/dedup) 项目里 `trash` 包的实现。
> 一个去重工具的"删除"功能，最重要的不是快，而是**绝不误删、且随时可恢复**。

## 直接 `os.Remove` 是危险的

`dedup` 的 `-delete` / `-delete-similar` 会移除"重复副本"。最直观的写法是 `os.Remove(path)`——但这是个**危险设计**：

- 一旦移错（比如哈希碰撞、路径拼错、用户点错），文件**永久消失**，没有后悔药。
- 删除是批量操作，某一个判断出错就可能毁掉一批文件。
- 对"相似图片"这类**非逐字节相同**的删除，判断本身就有误差，更不该用不可逆手段。

所以 `dedup` 的原则是：**所有删除都走系统回收站（Recycle Bin / Trash），绝不直接永久删除。** 用户随时能在回收站里找回。

## Windows：SHFileOperationW + FOF_ALLOWUNDO

Windows 提供了把文件送进回收站的 API：`shell32.dll!SHFileOperationW`，关键是给 `fFlags` 加上 `FOF_ALLOWUNDO`（值 `0x0040`）。

结构体布局要跟 Windows 的 `SHFILEOPSTRUCTW` 严格对齐（64 位下 56 字节），否则会内存错乱：

```go
var fo struct {
    hwnd                  uintptr
    wFunc                 uint32  // FO_DELETE = 0x0003
    pFrom                 *uint16 // 双 NUL 结尾的 UTF-16 路径串
    pTo                   *uint16
    fFlags                uint32  // FOF_ALLOWUNDO | FOF_NOCONFIRMATION | FOF_NOERRORUI
    fAnyOperationsAborted int32
    hNameMappings         uintptr
    lpszProgressTitle     *uint16
}
fo.wFunc = 0x0003
fo.pFrom = &buf[0]
fo.fFlags = 0x0040 | 0x0010 | 0x0400 // ALLOWUNDO | NOCONFIRMATION | NOERRORUI
```

几个标志的含义：

- `FOF_ALLOWUNDO`：允许撤销 → 文件进回收站而不是被永久删除。
- `FOF_NOCONFIRMATION`：不弹系统确认框（确认逻辑由我们自己的 CLI 负责）。
- `FOF_NOERRORUI`：不弹系统错误框，错误由返回值告诉我们。

`pFrom` 必须是**双 NUL 结尾**的 UTF-16 字符串（多个路径用单个 NUL 分隔，整体再补一个 NUL）。Go 里这样构造：

```go
var buf []uint16
for _, p := range paths {
    w, _ := syscall.UTF16FromString(p)
    buf = append(buf, w...)
}
buf = append(buf, 0) // 第二个 NUL
```

## 一个真实的坑：返回码为 2，但其实成功了

按文档，`SHFileOperationW` 返回 `0` 表示成功。但实际测试中发现：在不少环境里，文件**确实进了回收站**，函数却返回了非零（例如 `2`）。这是该 API 历史悠久的怪癖之一——返回值并不可靠。

如果天真地写 `if r != 0 { return error }`，就会频频误报失败，用户明明看到文件消失了，工具却说"删除失败"。

`dedup` 的判据改成**以"文件是否真的被移走"为准**：

```go
// 优先相信官方返回码
if r == 0 && fo.fAnyOperationsAborted == 0 {
    return nil
}
// 否则用副作用验证：每个源文件都不在了，就视为成功
allGone := true
for _, p := range paths {
    if _, err := os.Stat(p); err == nil {
        allGone = false
        break
    }
}
if allGone {
    return nil
}
// 真失败了再报错
if r != 0 {
    return fmt.Errorf("SHFileOperation 失败，代码 %d", r)
}
return fmt.Errorf("部分文件未移入回收站（操作被中止）")
```

这个"以副作用验证"的思路，比迷信返回值稳健得多。

## 批量删除的鲁棒性：缺一个不能全崩

`SHFileOperationW` 有个麻烦特性：**只要有一个源路径不存在或无效，整批操作可能整体失败**，导致已经处理好的其他文件也没动。

`dedup` 在调用前先过滤一遍，只保留**当前确实存在**的路径：

```go
existing := toDelete[:0]
for _, p := range toDelete {
    if _, err := os.Stat(p); err == nil {
        existing = append(existing, p)
    }
}
toDelete = existing
```

这样即使个别文件在扫描后被外部改动，也不会让整批删除流产。

## 二次确认：相似图片尤其要谨慎

`dedup` 对两类删除区别对待：

- `-delete`（精确重复）：副本与保留件**逐字节相同**，误删损失几乎为零，因此可在确认预览后直接执行（仍支持 `-dry-run`）。
- `-delete-similar`（相似图片）：副本**不是**逐字节相同，删除即可能丢掉略有差异的原图。因此**强制二次确认**：

```go
fmt.Fprintln(os.Stderr, "[相似图片] 以下副本将被移入回收站(保留每组代表图):")
for _, p := range toDelete {
    fmt.Fprintf(os.Stderr, "  - %s\n", p)
}
if !*yes {
    if !confirm("确认将上述相似副本移入回收站？相似图片并非完全相同，删除前请确认(可在回收站找回)") {
        fmt.Fprintln(os.Stderr, "已取消。")
        return
    }
}
```

`confirm` 从 stdin 读入 `y/N`，**默认是"否"**（读到 EOF 也视为否），避免管道 / 非交互环境下被意外放行。需要无人值守时再用 `-yes` 显式跳过。

## 跨平台：Linux / macOS

`trash` 包用 build tag 区分平台。Windows 走上面那套；其他系统调用桌面环境自带的回收机制：

- Linux：优先 `gio trash`（GNOME）或 `trash-put`（trash-cli）；
- macOS：用 `osascript` / `trash` 命令把文件送进 `~/.Trash`。

核心思想一致：**永远可恢复，绝不 `rm`**。

## 小结

一个"安全删除"的最小正确实现，要点其实是这几条：

1. **走回收站，不走 `os.Remove`**——可恢复是底线。
2. **别迷信 API 返回码**——用副作用（文件是否真的移走）验证。
3. **过滤掉已不存在的源**，避免整批失败。
4. **对不可逆判断加二次确认**，且默认拒绝。

这四点都不是炫技，而是真实踩过的坑。把它们写进简历项目里，比"会用 `os.Remove`"更能体现工程成熟度。

---

*实现见 [`trash/trash_windows.go`](../trash/trash_windows.go) 与 [`trash/trash_other.go`](../trash/trash_other.go)。上一篇讲 [dHash 感知哈希](blog-dhash.md)。*
