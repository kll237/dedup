// Command dedup finds duplicate files (by content hash) and visually similar
// images (by perceptual hash) on your disk, and can safely move duplicates
// into the recycle bin.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"dedup/hash"
	"dedup/imageph"
	"dedup/report"
	"dedup/result"
	"dedup/scan"
	"dedup/trash"
)

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func main() {
	var (
		paths      multiFlag
		mode       = flag.String("mode", "both", "扫描模式: exact(精确去重) | image(相似图片) | both")
		format     = flag.String("format", "text", "输出格式: text | json | html")
		outPath    = flag.String("out", "", "报告输出文件，默认 stdout")
		minSize    = flag.String("min-size", "0", "最小文件大小，如 1KB / 2MB")
		maxSize    = flag.String("max-size", "0", "最大文件大小，如 10MB")
		threshold  = flag.Int("threshold", 10, "相似图片汉明距离阈值(0-64)，越小越严格")
		workers    = flag.Int("workers", 0, "并发数，默认等于 CPU 核数")
		skipHidden = flag.Bool("skip-hidden", true, "跳过隐藏文件和目录")
		ignore     = flag.String("ignore", ".git,.node_modules", "跳过的目录名(逗号分隔)")
		deleteDup  = flag.Bool("delete", false, "把精确重复的额外副本移入回收站(保留每组一个)")
		dryRun     = flag.Bool("dry-run", false, "只预览将要删除的文件，不实际删除")
		showVer    = flag.Bool("version", false, "打印版本并退出")
	)
	flag.Var(&paths, "path", "要扫描的路径(可多次指定)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: dedup [选项] [-path DIR]... [DIRS...]\n\n选项:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVer {
		fmt.Println("dedup 0.1.0")
		return
	}

	roots := append([]string(paths), flag.Args()...)
	if len(roots) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 至少需要一个扫描路径 (用 -path 或位置参数)")
		flag.Usage()
		os.Exit(2)
	}

	minB, err := parseSize(*minSize)
	if err != nil {
		fatal(err)
	}
	maxB, err := parseSize(*maxSize)
	if err != nil {
		fatal(err)
	}

	ignoreDirs := map[string]bool{}
	for _, d := range strings.Split(*ignore, ",") {
		d = strings.TrimSpace(d)
		if d != "" {
			ignoreDirs[d] = true
		}
	}

	imageExts := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".tiff"}

	opts := scan.Options{
		Roots:      roots,
		MinSize:    minB,
		MaxSize:    maxB,
		SkipHidden: *skipHidden,
		IgnoreDirs: ignoreDirs,
	}
	if *mode == "image" {
		opts.IncludeExt = imageExts
	}

	files, err := scan.Walk(opts)
	if err != nil {
		fatal(err)
	}

	rep := report.Report{}
	rep.Stats.FilesScanned = len(files)

	switch *mode {
	case "exact":
		rep.Exact = hash.FindExact(files, *workers)
	case "image":
		rep.Similar = imageph.FindSimilar(onlyImages(files, imageExts), *threshold, *workers)
	case "both":
		rep.Exact = hash.FindExact(files, *workers)
		rep.Similar = imageph.FindSimilar(onlyImages(files, imageExts), *threshold, *workers)
	default:
		fatal(fmt.Errorf("未知模式: %s (应为 exact|image|both)", *mode))
	}

	for _, g := range rep.Exact {
		rep.Stats.ExactGroups++
		rep.Stats.WastedBytes += g.Size * int64(len(g.Files)-1)
	}
	rep.Stats.SimilarGroups = len(rep.Similar)

	// Deletion (safe: into recycle bin, keep one copy per exact group).
	if *deleteDup {
		var toDelete []string
		for _, g := range rep.Exact {
			for i := 1; i < len(g.Files); i++ { // index 0 is kept (sorted by path)
				toDelete = append(toDelete, g.Files[i].Path)
			}
		}
		// Only keep files that still exist, so a missing path can't abort the
		// whole batch (SHFileOperation fails entirely if any source is missing).
		existing := toDelete[:0]
		for _, p := range toDelete {
			if _, err := os.Stat(p); err == nil {
				existing = append(existing, p)
			}
		}
		toDelete = existing
		if len(toDelete) == 0 {
			fmt.Fprintln(os.Stderr, "没有需要删除的重复文件。")
		} else if *dryRun {
			fmt.Fprintln(os.Stderr, "[dry-run] 以下重复文件将被移入回收站(保留每组第一个):")
			for _, p := range toDelete {
				fmt.Fprintf(os.Stderr, "  - %s\n", p)
			}
		} else {
			fmt.Fprintf(os.Stderr, "即将把 %d 个文件移入回收站...\n", len(toDelete))
			if err := trash.MoveToTrash(toDelete); err != nil {
				fatal(fmt.Errorf("删除失败: %w", err))
			}
			fmt.Fprintf(os.Stderr, "已移入回收站 %d 个文件。\n", len(toDelete))
		}
	}

	var w = os.Stdout
	if *outPath != "" {
		f, err := os.Create(*outPath)
		if err != nil {
			fatal(err)
		}
		defer f.Close()
		w = f
	}
	if err := report.Render(w, rep, report.Format(*format)); err != nil {
		fatal(err)
	}
}

func onlyImages(files []result.FileRef, exts []string) []result.FileRef {
	set := make(map[string]bool, len(exts))
	for _, e := range exts {
		set[strings.ToLower(e)] = true
	}
	out := make([]result.FileRef, 0, len(files))
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f.Path))
		if set[ext] {
			out = append(out, f)
		}
	}
	return out
}

func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" || s == "0" {
		return 0, nil
	}
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "KB"):
		mult, s = 1024, strings.TrimSuffix(s, "KB")
	case strings.HasSuffix(s, "MB"):
		mult, s = 1024*1024, strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "GB"):
		mult, s = 1024*1024*1024, strings.TrimSuffix(s, "GB")
	case strings.HasSuffix(s, "K"):
		mult, s = 1024, strings.TrimSuffix(s, "K")
	case strings.HasSuffix(s, "M"):
		mult, s = 1024*1024, strings.TrimSuffix(s, "M")
	case strings.HasSuffix(s, "G"):
		mult, s = 1024*1024*1024, strings.TrimSuffix(s, "G")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("无法解析大小 %q: %w", s, err)
	}
	return n * mult, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "错误:", err)
	os.Exit(1)
}
