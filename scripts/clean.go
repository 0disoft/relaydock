package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const fallbackHTML = `<!doctype html>
<html lang="ko">
  <head><meta charset="UTF-8"><meta name="viewport" content="width=device-width"><title>RelayDock</title></head>
  <body><p>프론트엔드 번들이 없다. frontend에서 빌드를 실행해야 한다.</p></body>
</html>
`

func main() {
	paths := []string{
		"bin",
		"gen/go",
		"internal/persistence/postgres/sqlcgen",
		"frontend/dist",
		"frontend/bindings",
		"frontend/node_modules",
		"web/control-console/.svelte-kit",
		"web/control-console/build",
	}
	for _, path := range paths {
		if err := os.RemoveAll(path); err != nil {
			fatal("remove %s: %v", path, err)
		}
	}
	if err := os.MkdirAll(filepath.Join("frontend", "dist"), 0o755); err != nil {
		fatal("recreate frontend dist: %v", err)
	}
	if err := os.WriteFile(filepath.Join("frontend", "dist", "index.html"), []byte(fallbackHTML), 0o644); err != nil {
		fatal("write frontend fallback: %v", err)
	}
	fmt.Println("generated outputs removed; frontend fallback restored")
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
