package contextpack

/* llmnav/1 module
id=relaydock.contextpack.compile
role=Select, redact, hash, and package repository evidence before expert consultation data leaves the local boundary.
owns=ContextPack assembly|evidence redaction|content-addressed pack identity
excludes=expert model invocation|consultation persistence
search=build redacted context pack|select repository evidence|context pack manifest
invariant=Evidence content is redacted before its digest, size, and token estimate are recorded.
invariant=Preview and build share the same selection and redaction path.
stability=architecture
*/

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/expert/redaction"
)

type Compiler struct {
	selector Selector
	redactor redaction.Redactor
}

func NewCompiler(selector Selector, redactor redaction.Redactor) *Compiler {
	return &Compiler{selector: selector, redactor: redactor}
}
func (c *Compiler) Preview(ctx context.Context, req BuildRequest) (Pack, error) {
	pack, err := c.compile(ctx, req, false)
	return pack, err
}
func (c *Compiler) Build(ctx context.Context, req BuildRequest) (Pack, error) {
	return c.compile(ctx, req, true)
}
func (c *Compiler) compile(ctx context.Context, req BuildRequest, includeContent bool) (Pack, error) {
	if c == nil || c.selector == nil || c.redactor == nil {
		return Pack{}, fmt.Errorf("%w: ContextPack dependencies", core.ErrInvalidConfiguration)
	}
	root, err := filepath.Abs(req.RepositoryRoot)
	if err != nil {
		return Pack{}, err
	}
	req.RepositoryRoot = root
	req.Objective = strings.TrimSpace(req.Objective)
	if req.Objective == "" {
		return Pack{}, fmt.Errorf("%w: objective is required", core.ErrInvalidArgument)
	}
	selection, err := c.selector.Select(ctx, req)
	if err != nil {
		return Pack{}, err
	}
	createdAt := time.Now().UTC()
	ttl := req.TTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	pack := Pack{
		TenantID: strings.TrimSpace(req.TenantID), ProjectID: strings.TrimSpace(req.ProjectID),
		RepositoryRoot: root, Objective: req.Objective, SuccessCriteria: append([]string(nil), req.SuccessCriteria...),
		Attempts: append([]Attempt(nil), req.Attempts...), OpenQuestions: append([]string(nil), req.OpenQuestions...),
		ExcludedFiles: selection.ExcludedFiles, CreatedAt: createdAt, ExpiresAt: createdAt.Add(ttl),
	}
	pack.Revision = gitOutput(ctx, root, "rev-parse", "HEAD")
	status := gitOutput(ctx, root, "status", "--porcelain=v1")
	pack.WorkingTreeHash = Digest([]byte(status))
	for _, e := range selection.Evidence {
		result, err := c.redactor.Redact(ctx, e.Reference, []byte(e.Content))
		if err != nil {
			return Pack{}, err
		}
		if string(result.Sanitized) == "[REDACTED_FILE]" {
			pack.RedactionReport.RemovedFiles++
			pack.ExcludedFiles++
			continue
		}
		e.Content = string(result.Sanitized)
		e.Digest = Digest(result.Sanitized)
		e.Bytes = int64(len(result.Sanitized))
		pack.EstimatedBytes += e.Bytes
		pack.EstimatedTokens += (e.Bytes + 3) / 4
		for _, f := range result.Findings {
			pack.RedactionReport.RemovedSegments++
			pack.RedactionReport.Findings = append(pack.RedactionReport.Findings, RedactionFinding{Rule: f.Rule, Reference: f.Reference, Severity: f.Severity})
		}
		if !includeContent {
			e.Content = ""
		}
		pack.Evidence = append(pack.Evidence, e)
	}
	sort.Slice(pack.Evidence, func(i, j int) bool { return pack.Evidence[i].Reference < pack.Evidence[j].Reference })
	manifest := pack
	manifest.ID = ""
	manifest.CreatedAt = time.Time{}
	manifest.ExpiresAt = time.Time{}
	payload, _ := json.Marshal(manifest)
	pack.ID = "ctx_" + strings.TrimPrefix(Digest(payload), "sha256:")[:26]
	return pack, nil
}
func gitOutput(ctx context.Context, root string, args ...string) string {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "unavailable"
	}
	return strings.TrimSpace(string(out))
}
