package releasepack

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func Build(options BuildOptions) (BuildReport, error) {
	root, err := filepath.Abs(strings.TrimSpace(options.Root))
	if err != nil {
		return BuildReport{}, err
	}
	if strings.TrimSpace(options.OutputZIP) == "" {
		return BuildReport{}, fmt.Errorf("output ZIP path is required")
	}
	outputZIP, err := filepath.Abs(options.OutputZIP)
	if err != nil {
		return BuildReport{}, err
	}
	insideRoot, err := pathInside(root, outputZIP)
	if err != nil {
		return BuildReport{}, err
	}
	if insideRoot {
		return BuildReport{}, fmt.Errorf("output ZIP must be outside the repository root")
	}
	maximum := options.MaximumFileBytes
	if maximum <= 0 {
		maximum = DefaultMaximumFileBytes
	}
	generatedAt := options.GeneratedAt.UTC()
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC().Truncate(time.Second)
	}
	prefix := strings.Trim(strings.TrimSpace(options.ArchivePrefix), "/")
	if prefix == "" {
		prefix = defaultArchivePrefix
	}
	if err := WriteTree(root); err != nil {
		return BuildReport{}, err
	}
	policy, err := LoadSizePolicy(root, maximum)
	if err != nil {
		return BuildReport{}, err
	}
	records, err := scanFiles(root, false)
	if err != nil {
		return BuildReport{}, err
	}
	if err := enforceSizePolicy(records, policy); err != nil {
		return BuildReport{}, err
	}
	index, err := WriteManifest(root, records, generatedAt, policy.MaxBytes, options.ManifestChunkBytes)
	if err != nil {
		return BuildReport{}, err
	}
	if _, err := Verify(root); err != nil {
		return BuildReport{}, fmt.Errorf("verify generated manifest: %w", err)
	}
	allRecords, err := scanFiles(root, true)
	if err != nil {
		return BuildReport{}, err
	}
	if err := enforceSizePolicy(allRecords, policy); err != nil {
		return BuildReport{}, err
	}
	if err := writeDeterministicZIP(root, outputZIP, prefix, generatedAt, allRecords); err != nil {
		return BuildReport{}, err
	}
	archiveRecord, err := inspectFile(outputZIP, filepath.Base(outputZIP))
	if err != nil {
		return BuildReport{}, err
	}
	report := BuildReport{
		Root: root, OutputZIP: outputZIP, ArchiveSHA256: archiveRecord.SHA256,
		ArchiveBytes: archiveRecord.Size, Files: index.FileCount, TotalBytes: index.TotalBytes,
		ManifestChunks: len(index.Chunks),
	}
	for _, record := range allRecords {
		if record.Size > report.LargestFileBytes {
			report.LargestFileBytes = record.Size
			report.LargestFilePath = record.Path
		}
	}
	return report, nil
}

func writeDeterministicZIP(root, output, prefix string, generatedAt time.Time, records []FileRecord) error {
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(output), "."+filepath.Base(output)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	temporary := file.Name()
	if err := file.Chmod(0o644); err != nil {
		_ = file.Close()
		_ = os.Remove(temporary)
		return fmt.Errorf("set archive permissions: %w", err)
	}
	writer := zip.NewWriter(file)
	sorted := append([]FileRecord(nil), records...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	for _, record := range sorted {
		if err := addZIPFile(writer, root, prefix, generatedAt, record); err != nil {
			_ = writer.Close()
			_ = file.Close()
			_ = os.Remove(temporary)
			return err
		}
	}
	if err := writer.Close(); err != nil {
		_ = file.Close()
		_ = os.Remove(temporary)
		return fmt.Errorf("finalize archive: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(temporary)
		return fmt.Errorf("sync archive: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err := replaceFile(temporary, output); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("publish archive: %w", err)
	}
	return nil
}

func pathInside(root, candidate string) (bool, error) {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false, fmt.Errorf("compare archive path with repository root: %w", err)
	}
	relative = filepath.Clean(relative)
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))), nil
}

func addZIPFile(writer *zip.Writer, root, prefix string, generatedAt time.Time, record FileRecord) error {
	path := filepath.Join(root, filepath.FromSlash(record.Path))
	source, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open archive file %s: %w", record.Path, err)
	}
	defer source.Close()
	mode, err := parseMode(record.Mode)
	if err != nil {
		return err
	}
	before, err := source.Stat()
	if err != nil {
		return fmt.Errorf("stat archive file %s: %w", record.Path, err)
	}
	if !before.Mode().IsRegular() || before.Size() != record.Size || canonicalFileMode(record.Path, before.Mode()) != mode.Perm() {
		return fmt.Errorf("source metadata changed before archiving %s", record.Path)
	}
	header := &zip.FileHeader{
		Name: filepath.ToSlash(filepath.Join(prefix, record.Path)), Method: zip.Deflate,
	}
	header.SetMode(mode)
	header.Modified = generatedAt
	destination, err := writer.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create archive entry %s: %w", record.Path, err)
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(destination, hash), source)
	if err != nil {
		return fmt.Errorf("archive %s: %w", record.Path, err)
	}
	after, statErr := source.Stat()
	if statErr != nil {
		return fmt.Errorf("restat archive file %s: %w", record.Path, statErr)
	}
	if written != record.Size || hex.EncodeToString(hash.Sum(nil)) != record.SHA256 || after.Size() != record.Size || canonicalFileMode(record.Path, after.Mode()) != mode.Perm() {
		return fmt.Errorf("source changed while archiving %s", record.Path)
	}
	return nil
}

func parseMode(value string) (os.FileMode, error) {
	var parsed uint32
	if _, err := fmt.Sscanf(value, "%o", &parsed); err != nil {
		return 0, fmt.Errorf("parse file mode %q: %w", value, err)
	}
	return os.FileMode(parsed), nil
}
