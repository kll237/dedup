// Package report renders a scan Report in text, JSON or HTML form.
package report

import (
	"encoding/json"
	"fmt"
	"io"

	"dedup/result"
)

// Report is the full payload rendered by the tool.
type Report struct {
	Exact   []result.ExactGroup
	Similar []result.SimilarGroup
	Stats   result.Stats
}

// Format enumerates supported output formats.
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
	FormatHTML Format = "html"
)

// Render writes the report to w in the requested format.
func Render(w io.Writer, r Report, format Format) error {
	switch format {
	case FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	case FormatHTML:
		return renderHTML(w, r)
	case FormatText, "":
		return renderText(w, r)
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
}

func renderText(w io.Writer, r Report) error {
	s := r.Stats
	fmt.Fprintf(w, "扫描文件数: %d\n", s.FilesScanned)
	fmt.Fprintf(w, "精确重复组: %d  (可节省 %s)\n", s.ExactGroups, human(s.WastedBytes))
	fmt.Fprintf(w, "相似图片组: %d\n", s.SimilarGroups)
	fmt.Fprintln(w)

	if len(r.Exact) > 0 {
		fmt.Fprintln(w, "===== 精确重复 (按内容哈希) =====")
		for gi, g := range r.Exact {
			fmt.Fprintf(w, "\n[%d] 哈希 %s  单文件 %s  副本 %d  可节省 %s\n",
				gi+1, shortHash(g.Hash), human(g.Size), len(g.Files),
				human(g.Size*int64(len(g.Files)-1)))
			for _, f := range g.Files {
				fmt.Fprintf(w, "    - %s\n", f.Path)
			}
		}
	}

	if len(r.Similar) > 0 {
		fmt.Fprintln(w, "\n===== 相似图片 (感知哈希) =====")
		for gi, g := range r.Similar {
			fmt.Fprintf(w, "\n[%d] 代表指纹 %016x  张数 %d\n", gi+1, g.Representative, len(g.Files))
			for _, f := range g.Files {
				fmt.Fprintf(w, "    - %s  (%s)  %016x\n", f.Path, human(f.Size), f.Hash)
			}
		}
	}
	return nil
}

func shortHash(h string) string {
	if len(h) <= 16 {
		return h
	}
	return h[:8] + "…" + h[len(h)-8:]
}

func human(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	sizes := []string{"KB", "MB", "GB", "TB", "PB"}
	f := float64(b)
	i := 0
	for f >= 1024 && i < len(sizes)-1 {
		f /= 1024
		i++
	}
	return fmt.Sprintf("%.2f %s", f, sizes[i])
}
