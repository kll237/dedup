package main

import (
	"fmt"
	"os"
	"strings"
)

// progressBar renders a compact progress indicator on stderr. It is safe to
// use even when stdout is redirected to a file, because the bar is always
// written to stderr.
type progressBar struct {
	label string
	total int
	done  int
	last  int
}

func newProgressBar(total int, label string) *progressBar {
	return &progressBar{label: label, total: total}
}

// tick should be called once per completed unit of work (the total is fixed at
// construction time).
func (p *progressBar) tick(done, total int) {
	p.done++
	p.render()
}

// finish forces the bar to 100% and prints a trailing newline.
func (p *progressBar) finish() {
	p.done = p.total
	p.render()
}

func (p *progressBar) render() {
	if p.total <= 0 {
		return
	}
	pct := int(float64(p.done) / float64(p.total) * 100)
	if pct < p.last && p.done != p.total {
		return
	}
	p.last = pct
	const width = 28
	filled := int(float64(pct) / 100 * width)
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
	fmt.Fprintf(os.Stderr, "\r%-20s [%s] %d/%d %3d%%", p.label, bar, p.done, p.total, pct)
	if p.done >= p.total {
		fmt.Fprintln(os.Stderr)
	}
}
