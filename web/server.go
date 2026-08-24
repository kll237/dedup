// Package web serves a zero-dependency, browser-based dashboard for dedup.
// It reuses the existing scan/hash/imageph/report/trash packages and is
// shipped as the `dedup serve` subcommand. The frontend is embedded with
// go:embed so the binary is fully self-contained.
package web

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"dedup/hash"
	"dedup/imageph"
	"dedup/report"
	"dedup/result"
	"dedup/scan"
	"dedup/suggest"
	"dedup/trash"
)

//go:embed static
var staticFS embed.FS

// Server wires up the HTTP routes.
type Server struct{}

// NewServer returns a ready-to-use Server.
func NewServer() *Server { return &Server{} }

// Run parses the `serve` subcommand flags and starts the server. It blocks
// until the server stops.
func Run(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "监听地址，例如 :8080 或 127.0.0.1:9000")
	open := fs.Bool("open", true, "启动后自动打开浏览器")
	fs.Parse(args)

	url := "http://localhost" + *addr
	if *open {
		openBrowser(url)
	}
	fmt.Printf("dedup 可视化已启动: %s  (Ctrl+C 退出)\n", url)
	if err := Serve(*addr); err != nil {
		fmt.Fprintln(os.Stderr, "serve error:", err)
		os.Exit(1)
	}
}

// Serve starts the HTTP server on the given address.
func Serve(addr string) error {
	srv := &http.Server{Addr: addr, Handler: NewServer().routes()}
	return srv.ListenAndServe()
}

func (s *Server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/scan", s.handleScan)
	mux.HandleFunc("/api/thumb", s.handleThumb)
	mux.HandleFunc("/api/delete", s.handleDelete)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// ---- Scan (Server-Sent Events) ----

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	root := q.Get("path")
	if root == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	mode := q.Get("mode")
	if mode == "" {
		mode = "both"
	}
	threshold, _ := strconv.Atoi(q.Get("threshold"))
	if threshold <= 0 {
		threshold = 10
	}
	minB := parseSize(q.Get("min"))
	maxB := parseSize(q.Get("max"))
	skipHidden := q.Get("skipHidden") != "false"
	ignore := q.Get("ignore")
	if ignore == "" {
		ignore = ".git,.node_modules"
	}
	ignoreDirs := map[string]bool{}
	for _, d := range strings.Split(ignore, ",") {
		d = strings.TrimSpace(d)
		if d != "" {
			ignoreDirs[d] = true
		}
	}

	// 清理建议阈值（均带默认值，缺省时使用 suggest.Defaults）
	sopts := suggest.Defaults()
	if n, err := strconv.Atoi(q.Get("staleDays")); err == nil && n > 0 {
		sopts.StaleDays = n
	}
	if n, err := strconv.Atoi(q.Get("oldDays")); err == nil && n > 0 {
		sopts.OldDays = n
	}
	if n, err := strconv.Atoi(q.Get("oldMinMB")); err == nil && n > 0 {
		sopts.OldMinBytes = int64(n) * 1024 * 1024
	}
	if n, err := strconv.Atoi(q.Get("largeMinMB")); err == nil && n > 0 {
		sopts.LargeMinBytes = int64(n) * 1024 * 1024
	}
	if n, err := strconv.Atoi(q.Get("largeTopN")); err == nil && n > 0 {
		sopts.LargeTopN = n
	}
	if q.Get("redundant") == "false" {
		sopts.Redundant = false
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	f.Flush()

	imageExts := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".tiff"}
	opts := scan.Options{
		Roots:      []string{root},
		MinSize:    minB,
		MaxSize:    maxB,
		SkipHidden: skipHidden,
		IgnoreDirs: ignoreDirs,
	}
	if mode == "image" {
		opts.IncludeExt = imageExts
	}

	ctx := r.Context()
	send := func(event string, v any) bool {
		if ctx.Err() != nil {
			return false
		}
		sendSSE(w, f, event, v)
		return true
	}

	files, err := scan.Walk(opts)
	if err != nil {
		send("error", map[string]string{"message": err.Error()})
		return
	}
	if !send("progress", map[string]any{"phase": "scanned", "files": len(files)}) {
		return
	}

	rep := report.Report{}
	if mode != "image" {
		if !send("progress", map[string]any{"phase": "hash", "done": 0, "total": len(files)}) {
			return
		}
		rep.Exact = hash.FindExact(files, 0, func(done, total int) {
			send("progress", map[string]any{"phase": "hash", "done": done, "total": total})
		})
	}
	if mode != "exact" {
		imgs := onlyImages(files, imageExts)
		if !send("progress", map[string]any{"phase": "phash", "done": 0, "total": len(imgs)}) {
			return
		}
		rep.Similar = imageph.FindSimilar(imgs, threshold, 0, func(done, total int) {
			send("progress", map[string]any{"phase": "phash", "done": done, "total": total})
		})
	}
	rep.Stats = report.BuildStats(files, rep.Exact, rep.Similar)
	rep.Suggest = suggest.Analyze(files, rep.Exact, sopts)
	send("result", toWeb(rep))
}

// ---- Thumbnails ----

func (s *Server) handleThumb(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	if p == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	info, err := os.Stat(p)
	if err != nil || info.IsDir() {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, p)
}

// ---- Delete (into recycle bin) ----

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	existing := make([]string, 0, len(req.Paths))
	for _, p := range req.Paths {
		if _, err := os.Stat(p); err == nil {
			existing = append(existing, p)
		}
	}
	resp := map[string]any{"deleted": []string{}, "failed": map[string]string{}}
	if len(existing) > 0 {
		if err := trash.MoveToTrash(existing); err != nil {
			failed := map[string]string{}
			for _, p := range existing {
				failed[p] = err.Error()
			}
			resp["failed"] = failed
		} else {
			deleted := []string{}
			for _, p := range existing {
				if _, err := os.Stat(p); err != nil {
					deleted = append(deleted, p)
				}
			}
			resp["deleted"] = deleted
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ---- helpers ----

func sendSSE(w http.ResponseWriter, f http.Flusher, event string, v any) {
	data, _ := json.Marshal(v)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	f.Flush()
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

// webReport is the JSON shape sent to the browser. Hashes are hex strings so
// that 64-bit perceptual hashes survive JSON number parsing in JavaScript.
type webReport struct {
	Stats    result.Stats  `json:"stats"`
	Exact    []webExact    `json:"exact"`
	Similar  []webSimilar  `json:"similar"`
	Suggest  []webSuggest  `json:"suggest"`
}

type webExact struct {
	Hash  string    `json:"hash"`
	Size  int64     `json:"size"`
	Files []webFile `json:"files"`
}

type webSimilar struct {
	Rep   string    `json:"rep"`
	Files []webFile `json:"files"`
}

type webFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Hash string `json:"hash,omitempty"`
}

type webSuggest struct {
	Path     string   `json:"path"`
	Size     int64    `json:"size"`
	Ext      string   `json:"ext"`
	ModTime  int64    `json:"modTime"`
	Atime    int64    `json:"atime"`
	Func     string   `json:"func"`
	Category string   `json:"category"`
	Reason   string   `json:"reason"`
	Related  []string `json:"related"`
}

func toWeb(rep report.Report) webReport {
	out := webReport{Stats: rep.Stats}
	for _, g := range rep.Exact {
		wg := webExact{Hash: g.Hash, Size: g.Size}
		for _, f := range g.Files {
			wg.Files = append(wg.Files, webFile{Path: f.Path, Size: f.Size})
		}
		out.Exact = append(out.Exact, wg)
	}
	for _, g := range rep.Similar {
		ws := webSimilar{Rep: fmt.Sprintf("%016x", g.Representative)}
		for _, f := range g.Files {
			ws.Files = append(ws.Files, webFile{Path: f.Path, Size: f.Size, Hash: fmt.Sprintf("%016x", f.Hash)})
		}
		out.Similar = append(out.Similar, ws)
	}
	for _, s := range rep.Suggest {
		out.Suggest = append(out.Suggest, webSuggest{
			Path:     s.Path,
			Size:     s.Size,
			Ext:      s.Ext,
			ModTime:  s.ModTime,
			Atime:    s.Atime,
			Func:     s.Func,
			Category: s.Category,
			Reason:   s.Reason,
			Related:  s.Related,
		})
	}
	return out
}

func parseSize(s string) int64 {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" || s == "0" {
		return 0
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
		return 0
	}
	return n * mult
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
