// Package hash provides content-based (cryptographic) deduplication.
package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"runtime"
	"sort"
	"sync"

	"dedup/result"
)

// SHA256File returns the hex-encoded SHA-256 of a file's content.
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// FindExact groups files by content hash using a bounded worker pool.
// Files that cannot be read are skipped (and therefore never reported as
// duplicates). onProgress, if non-nil, is called once per hashed file with
// (done, total) so callers can render a progress bar.
func FindExact(files []result.FileRef, workers int, onProgress func(done, total int)) []result.ExactGroup {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	type job struct {
		f result.FileRef
	}
	type hashed struct {
		f    result.FileRef
		hash string
	}

	jobs := make(chan job)
	results := make(chan hashed, len(files))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				h, err := SHA256File(j.f.Path)
				if err != nil {
					continue
				}
				results <- hashed{f: j.f, hash: h}
			}
		}()
	}
	go func() {
		defer close(results)
		wg.Wait()
	}()
	go func() {
		for _, f := range files {
			jobs <- job{f: f}
		}
		close(jobs)
	}()

	type acc struct {
		size  int64
		files []result.FileRef
	}
	m := make(map[string]*acc)
	done := 0
	for r := range results {
		done++
		if onProgress != nil {
			onProgress(done, len(files))
		}
		a, ok := m[r.hash]
		if !ok {
			a = &acc{size: r.f.Size}
			m[r.hash] = a
		}
		a.files = append(a.files, r.f)
	}

	groups := make([]result.ExactGroup, 0, len(m))
	for h, a := range m {
		sort.Slice(a.files, func(i, j int) bool { return a.files[i].Path < a.files[j].Path })
		if len(a.files) > 1 {
			groups = append(groups, result.ExactGroup{Hash: h, Size: a.size, Files: a.files})
		}
	}
	// Sort by reclaimable space, largest first.
	sort.Slice(groups, func(i, j int) bool {
		wi := groups[i].Size * int64(len(groups[i].Files)-1)
		wj := groups[j].Size * int64(len(groups[j].Files)-1)
		return wi > wj
	})
	return groups
}
