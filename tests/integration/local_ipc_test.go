package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	expertapp "github.com/your-org/ai-runtime-gateway/internal/expert/app"
	"github.com/your-org/ai-runtime-gateway/internal/idgen"
	"github.com/your-org/ai-runtime-gateway/internal/localipc"
	"github.com/your-org/ai-runtime-gateway/internal/localruntime"
	"github.com/your-org/ai-runtime-gateway/internal/mcpcontract"
)

func TestLocalIPCRoundTrip(t *testing.T) {
	application, err := expertapp.NewInMemory()
	if err != nil {
		t.Fatal(err)
	}
	repositoryRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repositoryRoot, "ledger.go"), []byte("package ledger\n\nfunc Capture() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtimeCore, err := localruntime.New(application, repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := filepath.Join(t.TempDir(), "runtime.sock")
	if runtime.GOOS == "windows" {
		endpoint = `\\.\pipe\ai-runtime-test-` + idgen.New("")
	}
	server := localipc.NewServer(endpoint, runtimeCore.Handler())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- server.Run(ctx) }()
	deadline := time.Now().Add(2 * time.Second)
	for !server.Ready() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !server.Ready() {
		t.Fatal("local IPC server did not become ready")
	}

	var status struct {
		Ready               bool   `json:"ready"`
		ActiveConsultations int    `json:"activeConsultations"`
		RepositoryRoot      string `json:"repositoryRoot"`
	}
	if err := localipc.NewClient(endpoint).Call(context.Background(), "runtime.status", nil, &status); err != nil {
		t.Fatalf("IPC call: %v", err)
	}
	if !status.Ready || status.ActiveConsultations != 0 || status.RepositoryRoot == "" {
		t.Fatalf("unexpected IPC status: %+v", status)
	}

	client := localipc.NewClient(endpoint)
	var created mcpcontract.ConsultationCreateOutput
	if err := client.Call(context.Background(), "consultation.create", mcpcontract.ConsultationCreateInput{
		RepositoryRoot: status.RepositoryRoot,
		Objective:      "verify that a pending consultation can be cancelled through MCP IPC",
	}, &created); err != nil {
		t.Fatalf("create consultation over IPC: %v", err)
	}
	if created.ConsultationID == "" {
		t.Fatal("IPC create omitted consultation ID")
	}
	var cancelled mcpcontract.ConsultationCancelOutput
	if err := client.Call(context.Background(), "consultation.cancel", mcpcontract.ConsultationCancelInput{
		ConsultationID: created.ConsultationID,
	}, &cancelled); err != nil {
		t.Fatalf("cancel consultation over IPC: %v", err)
	}
	if cancelled.State != "cancelled" {
		t.Fatalf("unexpected cancelled state: %+v", cancelled)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("IPC server stopped with error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("IPC server did not stop after cancellation")
	}
}
