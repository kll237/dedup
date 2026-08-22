package report

import (
	"encoding/base64"
	"html"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"strings"

	"dedup/result"
)

func renderHTML(w io.Writer, r Report) error {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"zh\"><head><meta charset=\"utf-8\">")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">")
	b.WriteString("<title>文件去重报告</title>")
	b.WriteString("<style>")
	b.WriteString("body{font-family:system-ui,Segoe UI,Arial,sans-serif;margin:2rem;color:#222;background:#fafafa}")
	b.WriteString(".group{border:1px solid #ddd;border-radius:8px;padding:1rem;margin:1rem 0;background:#fff}")
	b.WriteString(".exact{border-left:4px solid #c0392b}.similar{border-left:4px solid #2980b9}")
	b.WriteString(".file{font-size:13px;color:#555;margin:2px 0;word-break:break-all}")
	b.WriteString("img.thumb{height:80px;margin:4px;border:1px solid #ccc;border-radius:4px;vertical-align:middle}")
	b.WriteString("</style></head><body>")
	b.WriteString("<h1>文件去重报告</h1>")
	b.WriteString(formatStats(r.Stats))

	if len(r.Exact) > 0 {
		b.WriteString("<h2>精确重复</h2>")
		for _, g := range r.Exact {
			b.WriteString("<div class=\"group exact\"><h3>")
			b.WriteString(html.EscapeString(shortHash(g.Hash)))
			b.WriteString(" · 副本 ")
			b.WriteString(itoa(len(g.Files)))
			b.WriteString(" · 可节省 ")
			b.WriteString(human(g.Size * int64(len(g.Files)-1)))
			b.WriteString("</h3>")
			for _, f := range g.Files {
				b.WriteString("<div class=\"file\">")
				b.WriteString(html.EscapeString(f.Path))
				b.WriteString("</div>")
			}
			b.WriteString("</div>")
		}
	}

	if len(r.Similar) > 0 {
		b.WriteString("<h2>相似图片</h2>")
		for gi, g := range r.Similar {
			b.WriteString("<div class=\"group similar\"><h3>组 ")
			b.WriteString(itoa(gi + 1))
			b.WriteString(" · 张数 ")
			b.WriteString(itoa(len(g.Files)))
			b.WriteString("</h3>")
			for _, f := range g.Files {
				if uri := thumbnailDataURI(f.Path); uri != "" {
					b.WriteString("<img class=\"thumb\" src=\"")
					b.WriteString(uri)
					b.WriteString("\" alt=\"thumb\">")
				}
				b.WriteString("<div class=\"file\">")
				b.WriteString(html.EscapeString(f.Path))
				b.WriteString("</div>")
			}
			b.WriteString("</div>")
		}
	}
	b.WriteString("</body></html>")

	_, err := io.WriteString(w, b.String())
	return err
}

func formatStats(s result.Stats) string {
	var b strings.Builder
	b.WriteString("<p>扫描文件数: <b>")
	b.WriteString(itoa(s.FilesScanned))
	b.WriteString("</b> (共 ")
	b.WriteString(human(s.BytesScanned))
	b.WriteString(") · 精确重复组: <b>")
	b.WriteString(itoa(s.ExactGroups))
	b.WriteString("</b> · 可节省: <b>")
	b.WriteString(human(s.WastedBytes))
	b.WriteString("</b> · 相似图片组: <b>")
	b.WriteString(itoa(s.SimilarGroups))
	b.WriteString("</b></p>")
	if len(s.ExtStats) > 0 {
		b.WriteString("<details><summary>按扩展名统计</summary><table><tr><th>扩展名</th><th>文件数</th><th>总大小</th></tr>")
		for _, e := range s.ExtStats {
			b.WriteString("<tr><td>")
			b.WriteString(html.EscapeString(e.Ext))
			b.WriteString("</td><td>")
			b.WriteString(itoa(e.Count))
			b.WriteString("</td><td>")
			b.WriteString(human(e.Bytes))
			b.WriteString("</td></tr>")
		}
		b.WriteString("</table></details>")
	}
	return b.String()
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

// thumbnailDataURI decodes an image, scales it down and returns a PNG
// data URI. On any error an empty string is returned (the path is still
// shown as text).
func thumbnailDataURI(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return ""
	}
	thumb := scaleToMax(img, 80)
	var buf strings.Builder
	buf.WriteString("data:image/png;base64,")
	enc := base64.NewEncoder(base64.StdEncoding, &buf)
	if err := png.Encode(enc, thumb); err != nil {
		return ""
	}
	enc.Close()
	return buf.String()
}

// scaleToMax downscales an image so its largest dimension is at most maxDim,
// preserving aspect ratio. Uses nearest-neighbour sampling — sufficient for
// tiny thumbnails.
func scaleToMax(src image.Image, maxDim int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxDim && h <= maxDim {
		return src
	}
	scale := float64(maxDim) / float64(max(w, h))
	nw, nh := int(float64(w)*scale), int(float64(h)*scale)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			sx := b.Min.X + int(float64(x)/scale)
			sy := b.Min.Y + int(float64(y)/scale)
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}
