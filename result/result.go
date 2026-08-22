// Package result defines the data structures shared across the dedup tool.
package result

// FileRef is a lightweight reference to a file on disk.
type FileRef struct {
	Path string
	Size int64
}

// ExactGroup groups files that have identical content (same hash).
type ExactGroup struct {
	Hash  string
	Size  int64 // size of a single file (all share the same content)
	Files []FileRef
}

// ImageRef is a file reference annotated with its perceptual hash.
type ImageRef struct {
	Path string
	Size int64
	Hash uint64
}

// SimilarGroup groups images that are perceptually similar.
type SimilarGroup struct {
	Representative uint64
	Files          []ImageRef
}

// ExtStat summarises files sharing the same extension.
type ExtStat struct {
	Ext   string
	Count int
	Bytes int64
}

// Stats summarises a scan run.
type Stats struct {
	FilesScanned  int
	BytesScanned  int64
	ExactGroups   int
	WastedBytes   int64
	SimilarGroups int
	ExtStats      []ExtStat
}
