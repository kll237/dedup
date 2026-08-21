// Package imageph implements perceptual hashing (dHash) for finding
// visually similar images. It depends only on the Go standard library.
package imageph

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"runtime"
	"sort"
	"sync"

	"dedup/result"
)

// PerceptualHash computes a 64-bit dHash fingerprint of an image file.
func PerceptualHash(path string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return 0, err
	}
	return HashImage(img), nil
}

// HashImage converts an image to grayscale, downscales it to 9x8 and
// produces a 64-bit difference hash: each bit is 1 when the left pixel of
// a horizontal pair is brighter than the right one.
func HashImage(img image.Image) uint64 {
	const w, h = 9, 8
	gray := resizeGray(img, w, h)
	var hash uint64
	for y := 0; y < h; y++ {
		for x := 0; x < w-1; x++ {
			hash <<= 1
			if gray[x][y] > gray[x+1][y] {
				hash |= 1
			}
		}
	}
	return hash
}

func resizeGray(src image.Image, w, h int) [][]float64 {
	b := src.Bounds()
	bw, bh := b.Dx(), b.Dy()
	out := make([][]float64, w)
	for x := 0; x < w; x++ {
		out[x] = make([]float64, h)
		sx := int(float64(x) * float64(bw) / float64(w))
		if sx >= bw {
			sx = bw - 1
		}
		for y := 0; y < h; y++ {
			sy := int(float64(y) * float64(bh) / float64(h))
			if sy >= bh {
				sy = bh - 1
			}
			r, g, bl, _ := src.At(b.Min.X+sx, b.Min.Y+sy).RGBA()
			out[x][y] = 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(bl>>8)
		}
	}
	return out
}

// Hamming returns the number of differing bits between two hashes.
func Hamming(a, b uint64) int {
	x := a ^ b
	c := 0
	for x != 0 {
		c++
		x &= x - 1
	}
	return c
}

// FindSimilar groups images whose perceptual hashes differ by at most
// threshold bits. It uses a worker pool to hash files, then a union-find
// over pairwise Hamming distances (O(n^2), fine for typical photo sets).
func FindSimilar(files []result.FileRef, threshold int, workers int) []result.SimilarGroup {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	type job struct {
		i   int
		ref result.FileRef
	}
	type out struct {
		i    int
		ref  result.ImageRef
		hash uint64
		ok   bool
	}

	jobs := make(chan job)
	results := make(chan out, len(files))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				h, err := PerceptualHash(j.ref.Path)
				if err != nil {
					results <- out{i: j.i, ok: false}
					continue
				}
				results <- out{i: j.i, ref: result.ImageRef{Path: j.ref.Path, Size: j.ref.Size, Hash: h}, hash: h, ok: true}
			}
		}()
	}
	go func() {
		defer close(results)
		wg.Wait()
	}()
	go func() {
		for i, f := range files {
			jobs <- job{i: i, ref: f}
		}
		close(jobs)
	}()

	refs := make([]result.ImageRef, len(files))
	hashes := make([]uint64, len(files))
	ok := make([]bool, len(files))
	for r := range results {
		if r.ok {
			refs[r.i] = r.ref
			hashes[r.i] = r.hash
			ok[r.i] = true
		}
	}

	parent := make([]int, len(files))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}
	for i := 0; i < len(files); i++ {
		if !ok[i] {
			continue
		}
		for j := i + 1; j < len(files); j++ {
			if !ok[j] {
				continue
			}
			if Hamming(hashes[i], hashes[j]) <= threshold {
				union(i, j)
			}
		}
	}

	grouped := make(map[int][]result.ImageRef)
	for i := range files {
		if !ok[i] {
			continue
		}
		root := find(i)
		grouped[root] = append(grouped[root], refs[i])
	}
	groups := make([]result.SimilarGroup, 0, len(grouped))
	for _, fs := range grouped {
		if len(fs) > 1 {
			sort.Slice(fs, func(a, b int) bool { return fs[a].Path < fs[b].Path })
			groups = append(groups, result.SimilarGroup{Representative: fs[0].Hash, Files: fs})
		}
	}
	sort.Slice(groups, func(a, b int) bool { return len(groups[a].Files) > len(groups[b].Files) })
	return groups
}
