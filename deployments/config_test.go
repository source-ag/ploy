package deployments

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFromFile_WithSettings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")
	err := os.WriteFile(configPath, []byte(`
settings:
  post-deploy-commands:
    - [./notify.sh, arg1]
    - [./other.sh]
deployments: []
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigFromFile(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Settings.PostDeployCommands) != 2 {
		t.Fatalf("expected 2 global commands, got %d", len(cfg.Settings.PostDeployCommands))
	}
	if cfg.Settings.PostDeployCommands[0][0] != "./notify.sh" {
		t.Errorf("cmd[0][0]: got %q, want %q", cfg.Settings.PostDeployCommands[0][0], "./notify.sh")
	}
	if cfg.Settings.PostDeployCommands[0][1] != "arg1" {
		t.Errorf("cmd[0][1]: got %q, want %q", cfg.Settings.PostDeployCommands[0][1], "arg1")
	}
	if cfg.Settings.PostDeployCommands[1][0] != "./other.sh" {
		t.Errorf("cmd[1][0]: got %q, want %q", cfg.Settings.PostDeployCommands[1][0], "./other.sh")
	}
}

func TestLoadConfigFromFile_WithoutSettings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")
	err := os.WriteFile(configPath, []byte(`deployments: []`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigFromFile(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Settings.PostDeployCommands) != 0 {
		t.Errorf("expected no global commands, got %d", len(cfg.Settings.PostDeployCommands))
	}
}
