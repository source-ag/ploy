package deployments

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDeploymentScript_InjectsEnvVars(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "env.txt")

	p := func(string, ...any) {}
	err := runDeploymentScript(
		"post",
		[]string{"sh", "-c", fmt.Sprintf("echo VERSION=$VERSION DEPLOYMENT_ID=$DEPLOYMENT_ID > %s", outFile)},
		"v1.2.3",
		"my-service",
		p,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	got := strings.TrimSpace(string(content))
	want := "VERSION=v1.2.3 DEPLOYMENT_ID=my-service"
	if got != want {
		t.Errorf("env vars: got %q, want %q", got, want)
	}
}
