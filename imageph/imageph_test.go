package imageph

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"dedup/result"
)

func makeImg(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func savePNG(t *testing.T, path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestPerceptualHashIdentical(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "1.png")
	p2 := filepath.Join(dir, "2.png")
	savePNG(t, p1, makeImg(64, 64, color.RGBA{120, 80, 200, 255}))
	savePNG(t, p2, makeImg(64, 64, color.RGBA{120, 80, 200, 255}))
	h1, err := PerceptualHash(p1)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := PerceptualHash(p2)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("identical images should have equal hash, got %d vs %d", h1, h2)
	}
}

func TestPerceptualHashSimilar(t *testing.T) {
	dir := t.TempDir()
	base := makeImg(64, 64, color.RGBA{120, 80, 200, 255})
	mod := makeImg(64, 64, color.RGBA{125, 85, 205, 255}) // tiny modification
	p1 := filepath.Join(dir, "a.png")
	p2 := filepath.Join(dir, "b.png")
	savePNG(t, p1, base)
	savePNG(t, p2, mod)
	h1, _ := PerceptualHash(p1)
	h2, _ := PerceptualHash(p2)
	if d := Hamming(h1, h2); d > 10 {
		t.Fatalf("small modification should stay near-identical, hamming=%d", d)
	}
	files := []result.FileRef{{Path: p1, Size: 1}, {Path: p2, Size: 1}}
	groups := FindSimilar(files, 10, 2, nil)
	if len(groups) != 1 {
		t.Fatalf("expected 1 similar group, got %d", len(groups))
	}
}

func TestFindSimilarDistinct(t *testing.T) {
	// 左→右渐变 (dHash 全 0) vs 右→左渐变 (dHash 全 1)，汉明距离 64，
	// 不应被归为相似。
	dir := t.TempDir()
	lr := image.NewRGBA(image.Rect(0, 0, 64, 64))
	rl := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			lr.Set(x, y, color.RGBA{uint8(x * 4), 0, 0, 255})        // 左→右
			rl.Set(x, y, color.RGBA{uint8((63 - x) * 4), 0, 0, 255}) // 右→左
		}
	}
	p1 := filepath.Join(dir, "lr.png")
	p2 := filepath.Join(dir, "rl.png")
	savePNG(t, p1, lr)
	savePNG(t, p2, rl)
	files := []result.FileRef{{Path: p1, Size: 1}, {Path: p2, Size: 1}}
	h1, _ := PerceptualHash(p1)
	h2, _ := PerceptualHash(p2)
	t.Logf("h1=%016x h2=%016x hamming=%d", h1, h2, Hamming(h1, h2))
	groups := FindSimilar(files, 5, 2, nil)
	if len(groups) != 0 {
		t.Fatalf("left->right vs right->left gradient should not be similar, got %d groups", len(groups))
	}
}
