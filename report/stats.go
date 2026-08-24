package report

import (
	"path/filepath"
	"sort"
	"strings"

	"dedup/result"
)

// BuildStats computes summary statistics from the scanned files and the
// duplicate/similar groups found. It is shared by the CLI renderer and the
// web dashboard so both report identical numbers.
func BuildStats(files []result.FileRef, exact []result.ExactGroup, similar []result.SimilarGroup) result.Stats {
	s := result.Stats{}
	s.FilesScanned = len(files)
	s.ExtStats = collectExtStats(files, &s.BytesScanned)
	for _, g := range exact {
		s.ExactGroups++
		s.WastedBytes += g.Size * int64(len(g.Files)-1)
	}
	s.SimilarGroups = len(similar)
	return s
}

// collectExtStats tallies file counts and bytes per extension and records the
// grand total into totalBytes. The returned slice is sorted by total bytes
// descending.
func collectExtStats(files []result.FileRef, totalBytes *int64) []result.ExtStat {
	type acc struct {
		count int
		bytes int64
	}
	m := map[string]*acc{}
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f.Path))
		if ext == "" {
			ext = "(无扩展名)"
		}
		a, ok := m[ext]
		if !ok {
			a = &acc{}
			m[ext] = a
		}
		a.count++
		a.bytes += f.Size
		*totalBytes += f.Size
	}
	out := make([]result.ExtStat, 0, len(m))
	for ext, a := range m {
		out = append(out, result.ExtStat{Ext: ext, Count: a.count, Bytes: a.bytes})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	return out
}
