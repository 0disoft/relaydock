package releasepack

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func Audit(root string, maximumFileBytes int64) (VerifyReport, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return VerifyReport{}, err
	}
	policy, err := LoadSizePolicy(root, maximumFileBytes)
	if err != nil {
		return VerifyReport{}, err
	}
	records, err := scanFiles(root, true)
	if err != nil {
		return VerifyReport{}, err
	}
	if err := enforceSizePolicy(records, policy); err != nil {
		return VerifyReport{}, err
	}
	return summarize(records, 0), nil
}

func Verify(root string) (VerifyReport, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return VerifyReport{}, err
	}
	indexRaw, err := os.ReadFile(filepath.Join(root, rootManifestPath))
	if err != nil {
		return VerifyReport{}, fmt.Errorf("read root manifest: %w", err)
	}
	var index ManifestIndex
	if err := decodeStrictJSON(indexRaw, &index, "root manifest"); err != nil {
		return VerifyReport{}, err
	}
	if index.Version != ManifestVersion || index.HashAlgorithm != "sha256" {
		return VerifyReport{}, fmt.Errorf("unsupported manifest contract")
	}
	if index.FileCount <= 0 || index.TotalBytes < 0 || index.MaximumFileBytes <= 0 || len(index.Chunks) == 0 {
		return VerifyReport{}, fmt.Errorf("invalid manifest index metadata")
	}
	seenChunkPaths := make(map[string]struct{}, len(index.Chunks))
	var expected []FileRecord
	previousLastPath := ""
	for _, reference := range index.Chunks {
		if reference.Path == "" || reference.FileCount <= 0 || reference.Size <= 0 || reference.FirstPath == "" || reference.LastPath == "" {
			return VerifyReport{}, fmt.Errorf("invalid manifest chunk reference")
		}
		if normalizeRelativePath(reference.Path) != reference.Path || !strings.HasPrefix(reference.Path, manifestChunkDirectory+"/") {
			return VerifyReport{}, fmt.Errorf("invalid manifest chunk path %q", reference.Path)
		}
		if _, exists := seenChunkPaths[reference.Path]; exists {
			return VerifyReport{}, fmt.Errorf("duplicate manifest chunk %s", reference.Path)
		}
		seenChunkPaths[reference.Path] = struct{}{}
		if previousLastPath != "" && reference.FirstPath <= previousLastPath {
			return VerifyReport{}, fmt.Errorf("manifest chunk ranges overlap or are unsorted at %s", reference.Path)
		}
		path := filepath.Join(root, filepath.FromSlash(reference.Path))
		raw, err := os.ReadFile(path)
		if err != nil {
			return VerifyReport{}, fmt.Errorf("read manifest chunk %s: %w", reference.Path, err)
		}
		if int64(len(raw)) != reference.Size {
			return VerifyReport{}, fmt.Errorf("manifest chunk size mismatch for %s", reference.Path)
		}
		hash := sha256.Sum256(raw)
		if hex.EncodeToString(hash[:]) != reference.SHA256 {
			return VerifyReport{}, fmt.Errorf("manifest chunk hash mismatch for %s", reference.Path)
		}
		var chunk ManifestChunk
		if err := decodeStrictJSON(raw, &chunk, "manifest chunk "+reference.Path); err != nil {
			return VerifyReport{}, err
		}
		if chunk.Version != ManifestVersion || len(chunk.Files) != reference.FileCount {
			return VerifyReport{}, fmt.Errorf("manifest chunk metadata mismatch for %s", reference.Path)
		}
		if len(chunk.Files) == 0 || chunk.Files[0].Path != reference.FirstPath || chunk.Files[len(chunk.Files)-1].Path != reference.LastPath {
			return VerifyReport{}, fmt.Errorf("manifest chunk range mismatch for %s", reference.Path)
		}
		for index, record := range chunk.Files {
			if normalizeRelativePath(record.Path) != record.Path || record.Path == "" || record.Size < 0 || len(record.SHA256) != sha256.Size*2 || record.Mode == "" {
				return VerifyReport{}, fmt.Errorf("invalid manifest record in %s", reference.Path)
			}
			if index > 0 && chunk.Files[index-1].Path >= record.Path {
				return VerifyReport{}, fmt.Errorf("manifest records are not strictly sorted in %s", reference.Path)
			}
		}
		expected = append(expected, chunk.Files...)
		previousLastPath = reference.LastPath
	}
	if len(expected) != index.FileCount {
		return VerifyReport{}, fmt.Errorf("manifest file count mismatch: index=%d chunks=%d", index.FileCount, len(expected))
	}
	if aggregateRecords(expected) != index.AggregateSHA256 {
		return VerifyReport{}, fmt.Errorf("manifest aggregate hash mismatch")
	}
	actual, err := scanFiles(root, false)
	if err != nil {
		return VerifyReport{}, err
	}
	if err := compareRecords(expected, actual); err != nil {
		return VerifyReport{}, err
	}
	policy, err := LoadSizePolicy(root, index.MaximumFileBytes)
	if err != nil {
		return VerifyReport{}, err
	}
	allFiles, err := scanFiles(root, true)
	if err != nil {
		return VerifyReport{}, err
	}
	if err := enforceSizePolicy(allFiles, policy); err != nil {
		return VerifyReport{}, err
	}
	report := summarize(actual, len(index.Chunks))
	if report.TotalBytes != index.TotalBytes {
		return VerifyReport{}, fmt.Errorf("manifest total byte count mismatch: index=%d actual=%d", index.TotalBytes, report.TotalBytes)
	}
	return report, nil
}

func compareRecords(expected, actual []FileRecord) error {
	sort.Slice(expected, func(i, j int) bool { return expected[i].Path < expected[j].Path })
	sort.Slice(actual, func(i, j int) bool { return actual[i].Path < actual[j].Path })
	if len(expected) != len(actual) {
		return fmt.Errorf("manifest content count mismatch: expected=%d actual=%d", len(expected), len(actual))
	}
	for index := range expected {
		if expected[index] != actual[index] {
			return fmt.Errorf("manifest mismatch for %s", expected[index].Path)
		}
	}
	return nil
}

func summarize(records []FileRecord, chunks int) VerifyReport {
	report := VerifyReport{Files: len(records), ManifestChunks: chunks}
	for _, record := range records {
		report.TotalBytes += record.Size
		if record.Size > report.LargestFileBytes {
			report.LargestFileBytes = record.Size
			report.LargestFilePath = record.Path
		}
	}
	return report
}
