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

func TestRunGlobalCommands_AllCommandsRun(t *testing.T) {
	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "cmd1.txt")
	file2 := filepath.Join(tmpDir, "cmd2.txt")

	commands := [][]string{
		{"sh", "-c", fmt.Sprintf("touch %s", file1)},
		{"sh", "-c", fmt.Sprintf("touch %s", file2)},
	}

	p := func(string, ...any) {}
	runGlobalCommands(commands, "my-service", "v1.0.0", p)

	if _, err := os.Stat(file1); os.IsNotExist(err) {
		t.Error("first command did not run: file1 not created")
	}
	if _, err := os.Stat(file2); os.IsNotExist(err) {
		t.Error("second command did not run: file2 not created")
	}
}

func TestRunGlobalCommands_FailureDoesNotBlockOthers(t *testing.T) {
	tmpDir := t.TempDir()
	file2 := filepath.Join(tmpDir, "cmd2.txt")

	commands := [][]string{
		{"false"},
		{"sh", "-c", fmt.Sprintf("touch %s", file2)},
	}

	p := func(string, ...any) {}
	runGlobalCommands(commands, "my-service", "v1.0.0", p)

	if _, err := os.Stat(file2); os.IsNotExist(err) {
		t.Error("second command did not run after first command failure")
	}
}

func TestRunGlobalCommands_EmptyListIsNoop(t *testing.T) {
	p := func(string, ...any) {}
	runGlobalCommands([][]string{}, "my-service", "v1.0.0", p)
}
