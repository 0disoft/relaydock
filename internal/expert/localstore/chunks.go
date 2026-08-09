package localstore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/expert/contextpack"
	"github.com/0disoft/relaydock/internal/persistence/atomicfile"
)

type chunkStore struct {
	root      string
	chunkSize int
}

func newChunkStore(statePath string, chunkSize int) chunkStore {
	if chunkSize <= 0 || chunkSize > defaultContentChunk {
		chunkSize = defaultContentChunk
	}
	return chunkStore{root: statePath + ".chunks", chunkSize: chunkSize}
}

func (s chunkStore) put(content []byte) ([]chunkReference, error) {
	if len(content) == 0 {
		return nil, nil
	}
	references := make([]chunkReference, 0, (len(content)+s.chunkSize-1)/s.chunkSize)
	for offset := 0; offset < len(content); offset += s.chunkSize {
		end := offset + s.chunkSize
		if end > len(content) {
			end = len(content)
		}
		chunk := content[offset:end]
		digest := digestBytes(chunk)
		path, err := s.pathForDigest(digest)
		if err != nil {
			return nil, err
		}
		existing, err := atomicfile.Read(path, int64(s.chunkSize+1))
		if err != nil {
			return nil, err
		}
		if len(existing) > 0 {
			if !bytes.Equal(existing, chunk) {
				return nil, fmt.Errorf("%w: content chunk %s does not match its digest", core.ErrInvalidConfiguration, digest)
			}
		} else if err := atomicfile.Write(path, chunk, 0o600); err != nil {
			return nil, fmt.Errorf("write content chunk %s: %w", digest, err)
		}
		references = append(references, chunkReference{Digest: digest, Size: len(chunk)})
	}
	return references, nil
}

func (s chunkStore) get(references []chunkReference, expectedBytes int64, expectedDigest string) ([]byte, error) {
	if expectedBytes == 0 && len(references) == 0 {
		return nil, nil
	}
	if expectedBytes < 0 || expectedBytes > maximumStateBytes*8 {
		return nil, fmt.Errorf("%w: invalid content byte count", core.ErrInvalidConfiguration)
	}
	var buffer bytes.Buffer
	if expectedBytes > 0 {
		buffer.Grow(int(expectedBytes))
	}
	for _, reference := range references {
		if reference.Size <= 0 || reference.Size > s.chunkSize {
			return nil, fmt.Errorf("%w: invalid content chunk size", core.ErrInvalidConfiguration)
		}
		path, err := s.pathForDigest(reference.Digest)
		if err != nil {
			return nil, err
		}
		raw, err := atomicfile.Read(path, int64(s.chunkSize+1))
		if err != nil {
			return nil, err
		}
		if len(raw) == 0 {
			return nil, fmt.Errorf("%w: missing content chunk %s", core.ErrInvalidConfiguration, reference.Digest)
		}
		if len(raw) != reference.Size || digestBytes(raw) != reference.Digest {
			return nil, fmt.Errorf("%w: corrupt content chunk %s", core.ErrInvalidConfiguration, reference.Digest)
		}
		_, _ = buffer.Write(raw)
	}
	content := buffer.Bytes()
	if int64(len(content)) != expectedBytes {
		return nil, fmt.Errorf("%w: reconstructed content length mismatch", core.ErrInvalidConfiguration)
	}
	if strings.TrimSpace(expectedDigest) != "" && contextpack.Digest(content) != expectedDigest {
		return nil, fmt.Errorf("%w: reconstructed evidence digest mismatch", core.ErrInvalidConfiguration)
	}
	return append([]byte(nil), content...), nil
}

func (s chunkStore) verifyReferences(references []chunkReference) error {
	for _, reference := range references {
		if reference.Size <= 0 || reference.Size > s.chunkSize {
			return fmt.Errorf("%w: invalid content chunk size", core.ErrInvalidConfiguration)
		}
		path, err := s.pathForDigest(reference.Digest)
		if err != nil {
			return err
		}
		raw, err := atomicfile.Read(path, int64(s.chunkSize+1))
		if err != nil {
			return fmt.Errorf("read content chunk %s: %w", reference.Digest, err)
		}
		if len(raw) == 0 {
			return fmt.Errorf("%w: missing content chunk %s", core.ErrInvalidConfiguration, reference.Digest)
		}
		if len(raw) != reference.Size || digestBytes(raw) != reference.Digest {
			return fmt.Errorf("%w: corrupt content chunk %s", core.ErrInvalidConfiguration, reference.Digest)
		}
	}
	return nil
}

func (s chunkStore) garbageCollect(live map[string]struct{}, olderThan time.Time, dryRun bool) (int, int64, error) {
	removed := 0
	var removedBytes int64
	err := filepath.WalkDir(s.root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		digest := chunkDigestPrefix + strings.ReplaceAll(filepath.ToSlash(relative), "/", "")
		if _, exists := live[digest]; exists {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !olderThan.IsZero() && info.ModTime().After(olderThan) {
			return nil
		}
		removed++
		removedBytes += info.Size()
		if !dryRun {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("garbage collect content chunks: %w", err)
	}
	if !dryRun {
		s.removeEmptyDirectories()
	}
	return removed, removedBytes, nil
}

func (s chunkStore) stats() (int, int64, error) {
	count := 0
	var bytesTotal int64
	err := filepath.WalkDir(s.root, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		count++
		bytesTotal += info.Size()
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, nil
	}
	return count, bytesTotal, err
}

func (s chunkStore) pathForDigest(digest string) (string, error) {
	digest = strings.TrimSpace(digest)
	if !strings.HasPrefix(digest, chunkDigestPrefix) {
		return "", fmt.Errorf("%w: malformed content chunk digest", core.ErrInvalidConfiguration)
	}
	hexValue := strings.TrimPrefix(digest, chunkDigestPrefix)
	if len(hexValue) != sha256.Size*2 {
		return "", fmt.Errorf("%w: malformed content chunk digest", core.ErrInvalidConfiguration)
	}
	if _, err := hex.DecodeString(hexValue); err != nil {
		return "", fmt.Errorf("%w: malformed content chunk digest", core.ErrInvalidConfiguration)
	}
	return filepath.Join(s.root, hexValue[:2], hexValue[2:]), nil
}

func (s chunkStore) removeEmptyDirectories() {
	var directories []string
	_ = filepath.WalkDir(s.root, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && entry.IsDir() && path != s.root {
			directories = append(directories, path)
		}
		return nil
	})
	sort.Slice(directories, func(i, j int) bool { return len(directories[i]) > len(directories[j]) })
	for _, directory := range directories {
		_ = os.Remove(directory)
	}
}

func digestBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return chunkDigestPrefix + hex.EncodeToString(sum[:])
}
