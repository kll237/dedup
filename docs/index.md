---
title: dedup 技术博客
---

# dedup 技术博客

[`dedup`](https://github.com/kll237/dedup) 是一个用 Go 编写、零第三方依赖的本地文件去重 / 相似图片查找 CLI。这里记录它背后的算法与工程取舍。

## 文章

- [用感知哈希（dHash）识别"相似图片"](blog-dhash.html)
  —— 灰度、缩放、差分、汉明距离、阈值调参，以及 dHash 在"纯色/均匀图"上退化的坑。

- [为什么"删除"要用回收站而不是 `rm`](blog-recyclebin.html)
  —— `SHFileOperationW` 的 `FOF_ALLOWUNDO` 机制、返回码为 2 实则成功的怪癖、跨平台方案与"绝不永久删除"的安全设计。

## 项目

- 仓库：<https://github.com/kll237/dedup>
- 许可证：MIT
