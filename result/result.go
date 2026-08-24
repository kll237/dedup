// Package result defines the data structures shared across the dedup tool.
package result

// FileRef is a lightweight reference to a file on disk.
type FileRef struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// ExactGroup groups files that have identical content (same hash).
type ExactGroup struct {
	Hash  string    `json:"hash"`
	Size  int64     `json:"size"` // size of a single file (all share the same content)
	Files []FileRef `json:"files"`
}

// ImageRef is a file reference annotated with its perceptual hash.
type ImageRef struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Hash uint64 `json:"hash"`
}

// SimilarGroup groups images that are perceptually similar.
type SimilarGroup struct {
	Representative uint64     `json:"representative"`
	Files          []ImageRef `json:"files"`
}

// ExtStat summarises files sharing the same extension.
type ExtStat struct {
	Ext   string `json:"ext"`
	Count int    `json:"count"`
	Bytes int64  `json:"bytes"`
}

// Stats summarises a scan run.
type Stats struct {
	FilesScanned  int      `json:"filesScanned"`
	BytesScanned  int64    `json:"bytesScanned"`
	ExactGroups   int      `json:"exactGroups"`
	WastedBytes   int64    `json:"wastedBytes"`
	SimilarGroups int      `json:"similarGroups"`
	ExtStats      []ExtStat `json:"extStats"`
}
