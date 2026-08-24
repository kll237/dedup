// Package suggest analyses a set of scanned files and produces a list of
// "useless file" recommendations. It flags temporary/cache files, empty files,
// exact duplicates, large files that have not been touched for a long time,
// files that have simply not been used for a long time, the largest files in a
// tree and redundant same-base-name variants. For each flagged file it reports
// the file's function (by extension), its last used time, related files
// (siblings / same-base-name variants) and a human readable reason, so the user
// can decide what to clean up safely.
//
// The engine is intentionally conservative: it only produces *recommendations*,
// never deletes anything. Files under well-known system directories get an extra
// caution appended to the reason so the user double-checks before acting.
package suggest

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"dedup/result"
)

// Options tunes the sensitivity of the analyser. Zero values fall back to the
// defaults returned by Defaults().
type Options struct {
	OldDays       int   // 修改时间超过该天数且体积大于 OldMinBytes -> "old"
	OldMinBytes   int64 // "old" 类别的最小体积
	StaleDays     int   // 最后使用时间超过该天数 -> "stale"（不限体积）
	LargeTopN     int   // 体积最大的 N 个文件 -> "large"
	LargeMinBytes int64 // 计入 "large" 的最小体积
	Redundant     bool  // 是否检测同名不同扩展名的冗余副本
}

// Defaults returns a sensible, non-aggressive default configuration.
func Defaults() Options {
	return Options{
		OldDays:       365,
		OldMinBytes:   50 * 1024 * 1024,
		StaleDays:     180,
		LargeTopN:     20,
		LargeMinBytes: 100 * 1024 * 1024,
		Redundant:     true,
	}
}

// Suggestion describes one file that the tool recommends reviewing/removing.
type Suggestion struct {
	Path     string   // absolute/relative path on disk
	Size     int64    // bytes
	Ext      string   // lower-cased extension including the dot, e.g. ".tmp"
	ModTime  int64    // last modified time, unix milliseconds
	Atime    int64    // last used (access) time, unix milliseconds
	Func     string   // human description of what the file type is for
	Category string   // empty | junk | dup | old | stale | large | redundant
	Reason   string   // why it is recommended for removal
	Related  []string // associated files (duplicate siblings, same-base-name variants)
}

// junkExts are extensions that almost always belong to temporary or cache
// artifacts and are safe to remove in the vast majority of cases.
var junkExts = map[string]bool{
	".tmp": true, ".temp": true, ".bak": true, ".old": true, ".log": true,
	".cache": true, ".swp": true, ".swo": true, ".part": true, ".crdownload": true,
	".ds_store": true, ".thumbs.db": true, ".desktop.ini": true, "~": true,
}

// catRank orders categories so the most "obviously removable" appear first.
var catRank = map[string]int{
	"empty": 0, "junk": 1, "dup": 2, "old": 3, "stale": 4, "large": 5, "redundant": 6,
}

// Analyze scans the given files and returns recommendations. exact carries the
// duplicate groups produced by hash.FindExact so duplicate copies are flagged.
func Analyze(files []result.FileRef, exact []result.ExactGroup, opts Options) []Suggestion {
	if opts.OldDays == 0 {
		opts = Defaults()
	}

	// Map each file path to its duplicate siblings and the number of copies.
	siblings := map[string][]string{}
	dupCount := map[string]int{}
	for _, g := range exact {
		for i, f := range g.Files {
			others := make([]string, 0, len(g.Files)-1)
			for j, o := range g.Files {
				if j != i {
					others = append(others, o.Path)
				}
			}
			siblings[f.Path] = others
			dupCount[f.Path] = len(g.Files) - 1
		}
	}

	// Group files by base name (without extension) and record distinct exts, so
	// we can surface same-base-name variants ("photo.jpg" <-> "photo.png").
	baseMap := map[string][]string{}
	extByBase := map[string]map[string]bool{}
	for _, f := range files {
		base, ext := splitBaseExt(f.Path)
		baseMap[base] = append(baseMap[base], f.Path)
		if extByBase[base] == nil {
			extByBase[base] = map[string]bool{}
		}
		extByBase[base][ext] = true
	}

	// Pre-compute the largest files in the tree for the "large" category.
	type sz struct{ path string; size int64 }
	all := make([]sz, 0, len(files))
	for _, f := range files {
		all = append(all, sz{f.Path, f.Size})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].size > all[j].size })
	largeSet := map[string]bool{}
	for i, s := range all {
		if i >= opts.LargeTopN {
			break
		}
		if s.size >= opts.LargeMinBytes {
			largeSet[s.path] = true
		}
	}

	const cap = 2000
	out := make([]Suggestion, 0, 64)
	for _, f := range files {
		fi, err := os.Stat(f.Path)
		if err != nil {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Path))
		mt := fi.ModTime()
		at := accessTime(fi)

		cat, reason := classify(ext, f.Size, mt, dupCount[f.Path], opts)
		if cat == "" && largeSet[f.Path] {
			cat = "large"
			reason = "体积较大的文件（" + human(f.Size) + "），若不再需要可归档或删除"
		}
		if cat == "" && opts.Redundant {
			base, _ := splitBaseExt(f.Path)
			if len(extByBase[base]) > 1 {
				cat = "redundant"
				reason = "存在同名不同格式的文件，可能为冗余副本"
			}
		}
		if cat == "" {
			continue
		}
		if isSystemPath(f.Path) {
			reason += "（位于系统目录，删除前请确认不影响系统功能）"
		}

		rel := map[string]bool{}
		for _, p := range siblings[f.Path] {
			rel[p] = true
		}
		base, _ := splitBaseExt(f.Path)
		for _, p := range baseMap[base] {
			if p != f.Path {
				rel[p] = true
			}
		}
		related := make([]string, 0, len(rel))
		for p := range rel {
			related = append(related, p)
		}
		sort.Strings(related)

		out = append(out, Suggestion{
			Path:     f.Path,
			Size:     f.Size,
			Ext:      ext,
			ModTime:  mt.UnixMilli(),
			Atime:    at.UnixMilli(),
			Func:     funcDesc(ext),
			Category: cat,
			Reason:   reason,
			Related:  related,
		})
		if len(out) >= cap {
			break
		}
	}

	// Sort so the most actionable categories (and biggest wastes) come first.
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := catRank[out[i].Category], catRank[out[j].Category]
		if ri != rj {
			return ri < rj
		}
		return out[i].Size > out[j].Size
	})
	return out
}

// classify decides whether a file should be recommended, returning the category
// and an explanation. An empty category means "keep".
func classify(ext string, size int64, mt time.Time, dup int, opts Options) (string, string) {
	if size == 0 {
		return "empty", "空文件（0 字节），通常无实际内容，可直接删除"
	}
	if junkExts[ext] {
		return "junk", "临时/缓存文件（" + ext + "），一般由程序自动生成，可安全清理"
	}
	if dup > 0 {
		return "dup", "与其他 " + itoa(dup) + " 份文件内容完全相同，属于重复文件"
	}
	if daysSince(mt) > opts.OldDays && size > opts.OldMinBytes {
		return "old", "超过 " + itoa(opts.OldDays) + " 天未修改且体积较大（" + human(size) + "），长期未使用"
	}
	// 以"修改时间"作为"长期未使用"的判断依据：atime 在 Windows 上常被禁用、
	// 在非 Windows 上标准库又取不到，跨平台一致性差；而修改时间始终可用且对
	// 静态文件（如错误页、文档）能真实反映"多久没动过"。
	if daysSince(mt) > opts.StaleDays {
		return "stale", "超过 " + itoa(opts.StaleDays) + " 天未修改，确认无用后可清理"
	}
	return "", ""
}

// splitBaseExt returns the lower-cased base name (without extension) and the
// lower-cased extension (including the dot) of a path.
func splitBaseExt(p string) (string, string) {
	base := strings.ToLower(filepath.Base(p))
	ext := strings.ToLower(filepath.Ext(p))
	base = strings.TrimSuffix(base, ext)
	return base, ext
}

// daysSince returns the number of whole days between t and now.
func daysSince(t time.Time) int {
	return int(time.Since(t).Hours() / 24)
}

// isSystemPath reports whether a path sits under a well-known operating-system
// directory. Matches are deliberately conservative (surrounding separators) so
// ordinary user folders named like "windows" are not flagged.
func isSystemPath(p string) bool {
	lp := strings.ToLower(filepath.ToSlash(p))
	for _, d := range []string{
		"/windows/", "/inetpub/", "/program files/", "/programdata/", "/$recycle.bin/",
	} {
		if strings.Contains(lp, d) {
			return true
		}
	}
	return false
}

// funcDesc returns a short Chinese description of what a file with the given
// extension is typically used for.
func funcDesc(ext string) string {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".tif", ".tiff":
		return "图片文件"
	case ".mp4", ".mov", ".avi", ".mkv", ".webm":
		return "视频文件"
	case ".mp3", ".wav", ".flac", ".aac", ".ogg":
		return "音频文件"
	case ".pdf":
		return "PDF 文档"
	case ".doc", ".docx":
		return "Word 文档"
	case ".xls", ".xlsx":
		return "Excel 表格"
	case ".ppt", ".pptx":
		return "PowerPoint 演示"
	case ".txt", ".md", ".rtf":
		return "文本/笔记文件"
	case ".zip", ".rar", ".7z", ".tar", ".gz":
		return "压缩归档文件"
	case ".exe", ".msi":
		return "可执行安装程序"
	case ".dll":
		return "动态链接库（程序运行依赖）"
	case ".log":
		return "日志文件（记录程序运行状态）"
	case ".tmp", ".temp", ".part", ".crdownload":
		return "临时/下载缓存文件"
	case ".bak", ".old":
		return "备份文件"
	case ".cache":
		return "缓存文件"
	case ".json", ".yaml", ".yml", ".toml", ".ini", ".conf", ".cfg":
		return "配置文件"
	case ".go", ".py", ".js", ".ts", ".java", ".c", ".cpp", ".h", ".rs":
		return "源代码文件"
	case ".html", ".css", ".xml", ".htm":
		return "网页/标记文件"
	case ".db", ".sqlite", ".sqlite3":
		return "数据库文件"
	case ".iso", ".img":
		return "磁盘镜像文件"
	case ".lnk":
		return "快捷方式"
	case ".ds_store", ".thumbs.db", ".desktop.ini":
		return "系统隐藏元数据文件"
	default:
		if ext == "" {
			return "无扩展名文件"
		}
		return "其他类型文件（" + ext + "）"
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func human(b int64) string {
	if b < 1024 {
		return itoa(int(b)) + " B"
	}
	sizes := []string{"KB", "MB", "GB", "TB", "PB"}
	f := float64(b) / 1024
	i := 0
	for f >= 1024 && i < len(sizes)-1 {
		f /= 1024
		i++
	}
	return trimFloat(f) + " " + sizes[i]
}

func trimFloat(f float64) string {
	s := strings.TrimRight(strconv.FormatFloat(f, 'f', 2, 64), "0")
	s = strings.TrimRight(s, ".")
	if s == "" {
		return "0"
	}
	return s
}
