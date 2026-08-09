package contextpack

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/0disoft/relaydock/internal/core"
)

type Selector interface {
	Select(context.Context, BuildRequest) (Selection, error)
}
type GitAwareSelector struct {
	MaximumFileBytes int64
	MaximumFiles     int
}

func NewGitAwareSelector() GitAwareSelector {
	return GitAwareSelector{MaximumFileBytes: 128 << 10, MaximumFiles: 80}
}
func (s GitAwareSelector) Select(ctx context.Context, req BuildRequest) (Selection, error) {
	root, err := filepath.Abs(req.RepositoryRoot)
	if err != nil {
		return Selection{}, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return Selection{}, err
	}
	if !info.IsDir() {
		return Selection{}, fmt.Errorf("%w: repository root is not a directory", core.ErrInvalidArgument)
	}
	if s.MaximumFileBytes <= 0 {
		s.MaximumFileBytes = 128 << 10
	}
	if s.MaximumFiles <= 0 {
		s.MaximumFiles = 80
	}
	max := req.MaximumBytes
	if max <= 0 {
		max = 512 << 10
	}
	keywords := keywords(req.Objective)
	paths := []string{}
	if len(req.CandidatePaths) > 0 {
		for _, candidate := range req.CandidatePaths {
			resolved, err := safeJoin(root, candidate)
			if err != nil {
				return Selection{}, err
			}
			st, err := os.Stat(resolved)
			if err != nil {
				continue
			}
			if st.IsDir() {
				_ = filepath.WalkDir(resolved, func(path string, d fs.DirEntry, walkErr error) error {
					if walkErr != nil {
						return nil
					}
					if err := ctx.Err(); err != nil {
						return err
					}
					if d.IsDir() && excludedDir(d.Name()) {
						return filepath.SkipDir
					}
					if !d.IsDir() {
						paths = append(paths, path)
					}
					return nil
				})
			} else {
				paths = append(paths, resolved)
			}
		}
	} else {
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if d.IsDir() && path != root && excludedDir(d.Name()) {
				return filepath.SkipDir
			}
			if !d.IsDir() {
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			return Selection{}, err
		}
	}
	type ranked struct{ e Evidence }
	rankedFiles := []ranked{}
	excluded := 0
	seen := map[string]bool{}
	for _, path := range paths {
		if seen[path] {
			continue
		}
		seen[path] = true
		st, err := os.Lstat(path)
		if err != nil || st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() || st.Size() == 0 || st.Size() > s.MaximumFileBytes {
			excluded++
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil || bytes.IndexByte(content, 0) >= 0 || !mostlyText(content) {
			excluded++
			continue
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			excluded++
			continue
		}
		rel = filepath.ToSlash(rel)
		score := scoreFile(rel, string(content), keywords)
		if len(req.CandidatePaths) == 0 && score <= 0 {
			excluded++
			continue
		}
		rankedFiles = append(rankedFiles, ranked{e: Evidence{Reference: rel, Digest: Digest(content), Reason: reasonFor(score, keywords, rel), Content: string(content), Bytes: int64(len(content)), Score: score}})
	}
	sort.Slice(rankedFiles, func(i, j int) bool {
		if rankedFiles[i].e.Score != rankedFiles[j].e.Score {
			return rankedFiles[i].e.Score > rankedFiles[j].e.Score
		}
		return rankedFiles[i].e.Reference < rankedFiles[j].e.Reference
	})
	selection := Selection{ExcludedFiles: excluded}
	for _, r := range rankedFiles {
		if len(selection.Evidence) >= s.MaximumFiles || selection.SelectedBytes+r.e.Bytes > max {
			selection.ExcludedFiles++
			continue
		}
		selection.Evidence = append(selection.Evidence, r.e)
		selection.SelectedBytes += r.e.Bytes
	}
	if len(selection.Evidence) == 0 {
		return selection, fmt.Errorf("%w: no eligible text files found", core.ErrNotFound)
	}
	return selection, nil
}
func safeJoin(root, candidate string) (string, error) {
	if strings.TrimSpace(candidate) == "" {
		return root, nil
	}
	path := candidate
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: path escapes repository root", core.ErrForbidden)
	}
	return abs, nil
}
func excludedDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", "node_modules", "vendor", "target", "dist", "build", ".svelte-kit", ".next", "coverage", "bin", "obj", ".idea", ".vscode":
		return true
	}
	return false
}
func keywords(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' })
	seen := map[string]bool{}
	out := []string{}
	for _, f := range fields {
		if len([]rune(f)) < 3 || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}
func scoreFile(path, content string, keywords []string) float64 {
	lowerPath := strings.ToLower(path)
	sample := strings.ToLower(content)
	if len(sample) > 64<<10 {
		sample = sample[:64<<10]
	}
	score := 0.0
	for _, k := range keywords {
		if strings.Contains(lowerPath, k) {
			score += 5
		}
		score += float64(min(strings.Count(sample, k), 8)) * 0.5
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go", ".ts", ".tsx", ".svelte", ".rs", ".py", ".md", ".sql", ".proto":
		score += 1
	}
	if strings.Contains(lowerPath, "test") {
		score += 0.5
	}
	return score
}
func reasonFor(score float64, keywords []string, path string) string {
	if len(keywords) == 0 {
		return "explicitly selected repository evidence"
	}
	return fmt.Sprintf("relevance score %.1f for objective; path %s", score, path)
}
func mostlyText(v []byte) bool {
	if len(v) == 0 {
		return true
	}
	bad := 0
	for _, b := range v {
		if b < 9 || (b > 13 && b < 32) {
			bad++
		}
	}
	return float64(bad)/float64(len(v)) < 0.01
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
