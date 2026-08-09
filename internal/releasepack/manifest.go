package releasepack

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func WriteManifest(root string, records []FileRecord, generatedAt time.Time, maximumFileBytes int64, maximumChunkBytes int) (ManifestIndex, error) {
	if maximumChunkBytes <= 0 {
		maximumChunkBytes = DefaultManifestChunkBytes
	}
	if maximumChunkBytes >= int(maximumFileBytes) {
		return ManifestIndex{}, fmt.Errorf("manifest chunk target must be smaller than maximum file size")
	}
	manifestRoot := filepath.Join(root, manifestDirectory)
	if err := os.RemoveAll(manifestRoot); err != nil {
		return ManifestIndex{}, fmt.Errorf("remove previous manifest chunks: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(manifestChunkDirectory)), 0o755); err != nil {
		return ManifestIndex{}, fmt.Errorf("create manifest chunk directory: %w", err)
	}

	chunks, err := splitManifestRecords(records, maximumChunkBytes)
	if err != nil {
		return ManifestIndex{}, err
	}
	index := ManifestIndex{
		Version: ManifestVersion, GeneratedAt: generatedAt.UTC(), HashAlgorithm: "sha256",
		FileCount: len(records), MaximumFileBytes: maximumFileBytes,
		Excluded: []string{rootManifestPath, manifestDirectory + "/**"},
	}
	for _, record := range records {
		index.TotalBytes += record.Size
	}
	index.AggregateSHA256 = aggregateRecords(records)

	for chunkIndex, chunk := range chunks {
		path := fmt.Sprintf("%s/files-%04d.json", manifestChunkDirectory, chunkIndex+1)
		raw, err := marshalIndented(chunk)
		if err != nil {
			return ManifestIndex{}, err
		}
		if len(raw) > maximumChunkBytes {
			return ManifestIndex{}, fmt.Errorf("manifest chunk %s is %d bytes; target=%d", path, len(raw), maximumChunkBytes)
		}
		if err := writeTextFile(filepath.Join(root, filepath.FromSlash(path)), raw, 0o644); err != nil {
			return ManifestIndex{}, fmt.Errorf("write manifest chunk %s: %w", path, err)
		}
		hash := sha256.Sum256(raw)
		index.Chunks = append(index.Chunks, ChunkReference{
			Path: path, FileCount: len(chunk.Files), FirstPath: chunk.Files[0].Path,
			LastPath: chunk.Files[len(chunk.Files)-1].Path, Size: int64(len(raw)),
			SHA256: hex.EncodeToString(hash[:]),
		})
	}
	indexRaw, err := marshalIndented(index)
	if err != nil {
		return ManifestIndex{}, err
	}
	if int64(len(indexRaw)) > maximumFileBytes {
		return ManifestIndex{}, fmt.Errorf("root manifest is %d bytes; limit=%d", len(indexRaw), maximumFileBytes)
	}
	if err := writeTextFile(filepath.Join(root, rootManifestPath), indexRaw, 0o644); err != nil {
		return ManifestIndex{}, fmt.Errorf("write root manifest: %w", err)
	}
	return index, nil
}

func splitManifestRecords(records []FileRecord, maximumBytes int) ([]ManifestChunk, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("cannot create an empty manifest")
	}
	sorted := append([]FileRecord(nil), records...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	var chunks []ManifestChunk
	current := ManifestChunk{Version: ManifestVersion}
	for _, record := range sorted {
		candidate := ManifestChunk{Version: ManifestVersion, Files: append(append([]FileRecord(nil), current.Files...), record)}
		raw, err := marshalIndented(candidate)
		if err != nil {
			return nil, err
		}
		if len(raw) <= maximumBytes {
			current = candidate
			continue
		}
		if len(current.Files) == 0 {
			return nil, fmt.Errorf("manifest record %s cannot fit in a %d-byte chunk", record.Path, maximumBytes)
		}
		chunks = append(chunks, current)
		current = ManifestChunk{Version: ManifestVersion, Files: []FileRecord{record}}
		raw, err = marshalIndented(current)
		if err != nil {
			return nil, err
		}
		if len(raw) > maximumBytes {
			return nil, fmt.Errorf("manifest record %s cannot fit in a %d-byte chunk", record.Path, maximumBytes)
		}
	}
	if len(current.Files) > 0 {
		chunks = append(chunks, current)
	}
	return chunks, nil
}

func marshalIndented(value any) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
