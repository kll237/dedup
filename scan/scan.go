// Package scan walks the filesystem and returns files that match the
// supplied filters. It is intentionally I/O bound and single-threaded;
// hashing (the expensive part) happens concurrently in the hash/imageph
// packages.
package scan

import (
	"io/fs"
	"path/filepath"
	"strings"

	"dedup/result"
)

// Options controls how the filesystem is walked.
type Options struct {
	Roots      []string
	MinSize    int64
	MaxSize    int64
	IncludeExt []string // lowercase, with dot, e.g. ".jpg"; empty = all
	ExcludeExt []string
	SkipHidden bool
	IgnoreDirs map[string]bool
}

func (o *Options) extAllowed(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if len(o.ExcludeExt) > 0 {
		for _, e := range o.ExcludeExt {
			if e == ext {
				return false
			}
		}
	}
	if len(o.IncludeExt) > 0 {
		for _, e := range o.IncludeExt {
			if e == ext {
				return true
			}
		}
		return false
	}
	return true
}

// Walk recursively lists files under every root, applying the filters.
func Walk(opts Options) ([]result.FileRef, error) {
	out := make([]result.FileRef, 0, 1024)
	seen := make(map[string]struct{})
	for _, root := range opts.Roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip unreadable paths
			}
			name := d.Name()
			if d.IsDir() {
				if opts.IgnoreDirs != nil && opts.IgnoreDirs[name] {
					return filepath.SkipDir
				}
				if opts.SkipHidden && name != "." && strings.HasPrefix(name, ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if opts.SkipHidden && strings.HasPrefix(name, ".") {
				return nil
			}
			if !opts.extAllowed(name) {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			sz := info.Size()
			if opts.MinSize > 0 && sz < opts.MinSize {
				return nil
			}
			if opts.MaxSize > 0 && sz > opts.MaxSize {
				return nil
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				abs = path
			}
			if _, ok := seen[abs]; ok {
				return nil
			}
			seen[abs] = struct{}{}
			out = append(out, result.FileRef{Path: abs, Size: sz})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
