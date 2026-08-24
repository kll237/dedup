// Package suggest analyses a set of scanned files and produces a list of
// "useless file" recommendations: temporary/cache files, empty files, exact
// duplicates and large files that have not been touched for a long time. For
// each flagged file it reports the file's function (by extension), its last
// used time, related files (siblings / same-base-name variants) and a human
// readable reason, so the user can decide what to clean up safely.
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

// Suggestion describes one file that the tool recommends removing.
type Suggestion struct {
	Path     string   // absolute/relative path on disk
	Size     int64    // bytes
	Ext      string   // lower-cased extension including the dot, e.g. ".tmp"
	ModTime  int64    // last modified time, unix milliseconds
	Atime    int64    // last used (access) time, unix milliseconds
	Func     string   // human description of what the file type is for
	Category string   // empty | junk | dup | old
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

// Analyze scans the given files and returns recommendations. exact carries the
// duplicate groups produced by hash.FindExact; duplicates are flagged as well.
func Analyze(files []result.FileRef, exact []result.ExactGroup) []Suggestion {
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

	// Group files that share the same base name (without extension) so we can
	// surface related files such as "photo.jpg" <-> "photo.png".
	baseMap := map[string][]string{}
	for _, f := range files {
		base := strings.ToLower(filepath.Base(f.Path))
		base = strings.TrimSuffix(base, filepath.Ext(base))
		baseMap[base] = append(baseMap[base], f.Path)
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

		cat, reason := classify(ext, f.Size, mt, dupCount[f.Path])
		if cat == "" {
			continue
		}

		rel := map[string]bool{}
		for _, p := range siblings[f.Path] {
			rel[p] = true
		}
		base := strings.ToLower(filepath.Base(f.Path))
		base = strings.TrimSuffix(base, filepath.Ext(base))
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
	return out
}

// classify decides whether a file should be recommended for removal, returning
// the category and an explanation. An empty category means "keep".
func classify(ext string, size int64, mt time.Time, dup int) (string, string) {
	if size == 0 {
		return "empty", "空文件（0 字节），通常无实际内容，可直接删除"
	}
	if junkExts[ext] {
		return "junk", "临时/缓存文件（" + ext + "），一般由程序自动生成，可安全清理"
	}
	if dup > 0 {
		return "dup", "与其他 " + itoa(dup) + " 份文件内容完全相同，属于重复文件"
	}
	ageDays := int(time.Since(mt).Hours() / 24)
	if ageDays > 365 && size > 50*1024*1024 {
		return "old", "超过 1 年未修改且体积较大（" + human(size) + "），长期未使用"
	}
	return "", ""
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
	case ".html", ".css", ".xml":
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
