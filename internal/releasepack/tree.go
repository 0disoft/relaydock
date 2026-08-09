package releasepack

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type treeNode struct {
	name      string
	directory bool
	children  []*treeNode
}

func WriteTree(root string) error {
	root = filepath.Clean(root)
	top := &treeNode{name: filepath.Base(root), directory: true}
	nodes := map[string]*treeNode{"": top}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = normalizeRelativePath(relative)
		if entry.IsDir() {
			if shouldExcludeDirectory(relative, entry.Name()) {
				return filepath.SkipDir
			}
			if relative == manifestDirectory {
				return filepath.SkipDir
			}
		} else {
			if !entry.Type().IsRegular() {
				return fmt.Errorf("repository contains unsupported non-regular file %s", relative)
			}
			if _, excluded := excludedFileNames[entry.Name()]; excluded {
				return nil
			}
		}
		if relative == "TREE.md" || relative == rootManifestPath {
			return nil
		}
		parentPath := normalizeRelativePath(filepath.Dir(relative))
		if parentPath == "." {
			parentPath = ""
		}
		parent := nodes[parentPath]
		if parent == nil {
			return fmt.Errorf("tree parent %q missing for %q", parentPath, relative)
		}
		node := &treeNode{name: entry.Name(), directory: entry.IsDir()}
		parent.children = append(parent.children, node)
		if entry.IsDir() {
			nodes[relative] = node
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("build repository tree: %w", err)
	}
	var output strings.Builder
	output.WriteString("# Repository Tree\n\n```text\n")
	output.WriteString(top.name)
	output.WriteByte('\n')
	writeTreeNode(&output, top, "")
	output.WriteString("```\n")
	return writeTextFile(filepath.Join(root, "TREE.md"), []byte(output.String()), 0o644)
}

func writeTreeNode(output *strings.Builder, node *treeNode, prefix string) {
	sort.Slice(node.children, func(i, j int) bool {
		if node.children[i].directory != node.children[j].directory {
			return node.children[i].directory
		}
		return strings.ToLower(node.children[i].name) < strings.ToLower(node.children[j].name)
	})
	for index, child := range node.children {
		last := index == len(node.children)-1
		branch := "├── "
		nextPrefix := prefix + "│   "
		if last {
			branch = "└── "
			nextPrefix = prefix + "    "
		}
		output.WriteString(prefix)
		output.WriteString(branch)
		output.WriteString(child.name)
		if child.directory {
			output.WriteByte('/')
		}
		output.WriteByte('\n')
		if child.directory {
			writeTreeNode(output, child, nextPrefix)
		}
	}
}

func writeTextFile(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := replaceFile(temporaryPath, path); err != nil {
		return err
	}
	removeTemporary = false
	return nil
}
