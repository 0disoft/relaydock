package releasepack

import "time"

const (
	ManifestVersion           = 2
	DefaultMaximumFileBytes   = int64(40 * 1024)
	DefaultManifestChunkBytes = 30 * 1024
	defaultArchivePrefix      = "ai-runtime-gateway"
	rootManifestPath          = "MANIFEST.json"
	manifestDirectory         = "manifest"
	manifestChunkDirectory    = "manifest/chunks"
	fileSizeExceptionConfig   = "config/file-size-exceptions.json"
)

type FileRecord struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Mode   string `json:"mode"`
}

type ChunkReference struct {
	Path      string `json:"path"`
	FileCount int    `json:"fileCount"`
	FirstPath string `json:"firstPath"`
	LastPath  string `json:"lastPath"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
}

type ManifestIndex struct {
	Version          int              `json:"version"`
	GeneratedAt      time.Time        `json:"generatedAt"`
	HashAlgorithm    string           `json:"hashAlgorithm"`
	FileCount        int              `json:"fileCount"`
	TotalBytes       int64            `json:"totalBytes"`
	MaximumFileBytes int64            `json:"maximumFileBytes"`
	AggregateSHA256  string           `json:"aggregateSha256"`
	Chunks           []ChunkReference `json:"chunks"`
	Excluded         []string         `json:"excluded,omitempty"`
}

type ManifestChunk struct {
	Version int          `json:"version"`
	Files   []FileRecord `json:"files"`
}

type SizeException struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type SizePolicy struct {
	Version    int             `json:"version"`
	MaxBytes   int64           `json:"maxBytes"`
	Exceptions []SizeException `json:"exceptions"`
}

type BuildOptions struct {
	Root               string
	OutputZIP          string
	ArchivePrefix      string
	GeneratedAt        time.Time
	MaximumFileBytes   int64
	ManifestChunkBytes int
}

type BuildReport struct {
	Root             string
	OutputZIP        string
	ArchiveSHA256    string
	ArchiveBytes     int64
	Files            int
	TotalBytes       int64
	ManifestChunks   int
	LargestFilePath  string
	LargestFileBytes int64
}

type VerifyReport struct {
	Files            int
	TotalBytes       int64
	ManifestChunks   int
	LargestFilePath  string
	LargestFileBytes int64
}
