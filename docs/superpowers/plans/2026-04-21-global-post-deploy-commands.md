# Global Post-Deploy Commands Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `settings.post-deploy-commands` block to the ploy YAML config that runs a list of shell commands after each successful service deployment, before the per-service `post-deploy-command`.

**Architecture:** Introduce `Settings` and `Config` types in `deployments/config.go`; rename the load/write functions to `LoadConfigFromFile`/`WriteConfigToFile` so settings round-trip through `update`; add `runGlobalCommands` in `deployments/deploy.go` that iterates commands sequentially and logs-but-ignores failures; inject `DEPLOYMENT_ID` alongside `VERSION` into all deployment scripts.

**Tech Stack:** Go 1.23, `gopkg.in/yaml.v3`, `github.com/mitchellh/mapstructure`

---

## Files

| File | Change |
|------|--------|
| `deployments/config.go` | Add `Settings`, `Config` types; rename load/write functions; keep settings through round-trip |
| `deployments/deploy.go` | Update `Deploy`; add `runGlobalCommands`; add `DEPLOYMENT_ID` to `runDeploymentScript`; update `doDeployment` |
| `deployments/verify.go` | Update to use `LoadConfigFromFile` |
| `deployments/update.go` | Update to use `LoadConfigFromFile` and `WriteConfigToFile` |
| `deployments/config_test.go` | New: settings YAML parsing tests |
| `deployments/deploy_test.go` | New: `runGlobalCommands` behavior and env var injection tests |

---

### Task 1: Create feature branch

- [ ] **Step 1: Create and switch to feature branch**

```bash
git checkout -b feature/global-post-deploy-commands
```

Expected: now on branch `feature/global-post-deploy-commands`.

- [ ] **Step 2: Verify the build is clean before starting**

```bash
go build ./...
```

Expected: builds with no errors.

---

### Task 2: Add Settings and Config types, update config.go

**Files:**
- Modify: `deployments/config.go`
- Create: `deployments/config_test.go`

- [ ] **Step 1: Write failing tests for config parsing**

Create `deployments/config_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./deployments/... -run TestLoadConfigFromFile -v
```

Expected: FAIL — `LoadConfigFromFile` undefined.

- [ ] **Step 3: Replace deployments/config.go**

```go
package deployments

import (
	"bytes"
	"os"

	"github.com/mitchellh/mapstructure"
	"github.com/source-ag/ploy/engine"
	"gopkg.in/yaml.v3"
)

type Settings struct {
	PostDeployCommands [][]string `yaml:"post-deploy-commands,omitempty"`
}

type Config struct {
	Settings    Settings
	Deployments []engine.Deployment
}

// deploymentsYAML is the raw YAML shape, used only for marshaling/unmarshaling.
type deploymentsYAML struct {
	Settings    Settings         `yaml:"settings,omitempty"`
	Deployments []map[string]any `yaml:"deployments"`
}

func LoadConfigFromFile(deploymentsConfigPath string) (Config, error) {
	raw := &deploymentsYAML{}
	b, err := os.ReadFile(deploymentsConfigPath)
	if err != nil {
		return Config{}, err
	}
	if err = yaml.Unmarshal(b, raw); err != nil {
		return Config{}, err
	}
	deployments := make([]engine.Deployment, 0, len(raw.Deployments))
	for _, deployment := range raw.Deployments {
		deploymentEngine, err := engine.GetEngine(deployment["type"].(string))
		if err != nil {
			return Config{}, err
		}
		deploymentConfig := deploymentEngine.ResolveConfigStruct()
		if err = mapstructure.Decode(deployment, deploymentConfig); err != nil {
			return Config{}, err
		}
		deployments = append(deployments, deploymentConfig)
	}
	return Config{
		Settings:    raw.Settings,
		Deployments: deployments,
	}, nil
}

func WriteConfigToFile(deploymentsConfigPath string, cfg Config) error {
	deploymentMaps := make([]map[string]any, 0, len(cfg.Deployments))
	for _, deployment := range cfg.Deployments {
		var deploymentMap map[string]any
		if err := mapstructure.Decode(deployment, &deploymentMap); err != nil {
			return err
		}
		deploymentMaps = append(deploymentMaps, deploymentMap)
	}
	raw := deploymentsYAML{
		Settings:    cfg.Settings,
		Deployments: deploymentMaps,
	}
	return marshalYamlToFile(raw, deploymentsConfigPath)
}

func marshalYamlToFile(in interface{}, path string) (err error) {
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	defer func() {
		err = encoder.Close()
	}()
	encoder.SetIndent(2)
	err = encoder.Encode(in)
	if err != nil {
		return
	}
	err = os.WriteFile(path, buffer.Bytes(), 0644)
	return
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./deployments/... -run TestLoadConfigFromFile -v
```

Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add deployments/config.go deployments/config_test.go
git commit -m "feat: add Settings and Config types with post-deploy-commands support"
```

---

### Task 3: Update all callers of the renamed config functions

**Files:**
- Modify: `deployments/verify.go`
- Modify: `deployments/update.go`
- Modify: `deployments/deploy.go`

- [ ] **Step 1: Replace the Verify function in deployments/verify.go**

Replace the `Verify` function body (keep `FailOnVersionMismatch` var and `verifyDeployment` unchanged):

```go
func Verify(_ *cobra.Command, args []string) {
	cfg, err := LoadConfigFromFile(args[0])
	cobra.CheckErr(err)

	errorChan := make(chan error, len(cfg.Deployments))
	var wg sync.WaitGroup
	for _, deployment := range cfg.Deployments {
		wg.Add(1)
		go func(deployment engine.Deployment) {
			defer wg.Done()
			errorChan <- verifyDeployment(deployment, FailOnVersionMismatch)
		}(deployment)
	}
	wg.Wait()
	close(errorChan)
	var result *multierror.Error
	for err := range errorChan {
		result = multierror.Append(result, err)
	}
	cobra.CheckErr(result.ErrorOrNil())
}
```

- [ ] **Step 2: Replace the Update function in deployments/update.go**

Replace the `Update` function body entirely:

```go
func Update(_ *cobra.Command, args []string) {
	deploymentsConfigPath := args[0]
	nrArgs := len(args)
	serviceIds := args[1 : nrArgs-1]
	version := args[nrArgs-1]
	cfg, err := LoadConfigFromFile(deploymentsConfigPath)
	cobra.CheckErr(err)
	for _, serviceId := range serviceIds {
		service := utils.Find(cfg.Deployments, func(d engine.Deployment) bool {
			return d.Id() == serviceId
		})
		if service == nil {
			cobra.CheckErr(fmt.Errorf("there is no service with id '%s'", serviceId))
		}
		(*service).SetVersion(version)
	}
	err = WriteConfigToFile(deploymentsConfigPath, cfg)
	cobra.CheckErr(err)
}
```

- [ ] **Step 3: Update the Deploy function and doDeployment signature in deployments/deploy.go**

Replace the `Deploy` function:

```go
func Deploy(_ *cobra.Command, args []string) {
	cfg, err := LoadConfigFromFile(args[0])
	cobra.CheckErr(err)

	errorChan := make(chan error, len(cfg.Deployments))
	var wg sync.WaitGroup
	for _, deployment := range cfg.Deployments {
		wg.Add(1)
		go func(deployment engine.Deployment) {
			defer wg.Done()
			errorChan <- doDeployment(deployment, cfg.Settings.PostDeployCommands)
		}(deployment)
	}
	wg.Wait()
	close(errorChan)
	var result *multierror.Error
	for err := range errorChan {
		result = multierror.Append(result, err)
	}
	cobra.CheckErr(result.ErrorOrNil())
}
```

Update `doDeployment` to accept the global commands parameter (body stays the same for now — global command logic is added in Task 5):

```go
func doDeployment(deploymentConfig engine.Deployment, globalPostDeployCommands [][]string) error {
```

- [ ] **Step 4: Build to verify no compile errors**

```bash
go build ./...
```

Expected: builds successfully. (`globalPostDeployCommands` is an unused parameter — that is fine in Go.)

- [ ] **Step 5: Commit**

```bash
git add deployments/verify.go deployments/update.go deployments/deploy.go
git commit -m "refactor: update callers to use LoadConfigFromFile and WriteConfigToFile"
```

---

### Task 4: Inject DEPLOYMENT_ID env var into deployment scripts

**Files:**
- Modify: `deployments/deploy.go`
- Create: `deployments/deploy_test.go`

- [ ] **Step 1: Write a failing test for DEPLOYMENT_ID injection**

Create `deployments/deploy_test.go`:

```go
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
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./deployments/... -run TestRunDeploymentScript_InjectsEnvVars -v
```

Expected: FAIL — `runDeploymentScript` called with wrong number of arguments (missing `deploymentId`).

- [ ] **Step 3: Update runDeploymentScript and its call sites in deployments/deploy.go**

Replace `runDeploymentScript`:

```go
func runDeploymentScript(context string, deploymentCommand []string, version string, deploymentId string, p func(string, ...any)) error {
	p("running %s-deployment command %s...", context, strings.Join(deploymentCommand, " "))
	cmd := exec.Command(deploymentCommand[0], deploymentCommand[1:]...)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("VERSION=%s", version))
	cmd.Env = append(cmd.Env, fmt.Sprintf("DEPLOYMENT_ID=%s", deploymentId))
	output, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}
	if err = cmd.Wait(); err != nil {
		return err
	}
	return nil
}
```

Update the two existing call sites in `doDeployment` to pass `deploymentConfig.Id()`:

```go
if err = runDeploymentScript("pre", deploymentConfig.PreDeployCommand(), deploymentConfig.Version(), deploymentConfig.Id(), p); err != nil {
```

```go
if err = runDeploymentScript("post", deploymentConfig.PostDeployCommand(), deploymentConfig.Version(), deploymentConfig.Id(), p); err != nil {
```

- [ ] **Step 4: Run test to confirm it passes**

```bash
go test ./deployments/... -run TestRunDeploymentScript_InjectsEnvVars -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add deployments/deploy.go deployments/deploy_test.go
git commit -m "feat: inject DEPLOYMENT_ID env var into all deployment scripts"
```

---

### Task 5: Add runGlobalCommands and wire it into doDeployment

**Files:**
- Modify: `deployments/deploy.go`
- Modify: `deployments/deploy_test.go`

- [ ] **Step 1: Write failing tests for runGlobalCommands**

Append to `deployments/deploy_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./deployments/... -run TestRunGlobalCommands -v
```

Expected: FAIL — `runGlobalCommands` undefined.

- [ ] **Step 3: Add runGlobalCommands to deployments/deploy.go**

Add this function (place it alongside `runDeploymentScript`):

```go
func runGlobalCommands(commands [][]string, deploymentId string, version string, p func(string, ...any)) {
	for _, cmd := range commands {
		if len(cmd) == 0 {
			continue
		}
		if err := runDeploymentScript("global post", cmd, version, deploymentId, p); err != nil {
			p("global post-deploy command failed (ignored): %v\n", err)
		}
	}
}
```

- [ ] **Step 4: Update doDeployment to call runGlobalCommands**

Replace `doDeployment` with the complete updated version:

```go
func doDeployment(deploymentConfig engine.Deployment, globalPostDeployCommands [][]string) error {
	p := CreateDeploymentPrinter(deploymentConfig.Id())
	deploymentEngine, err := engine.GetEngine(deploymentConfig.Type())
	if err != nil {
		return fmt.Errorf("%s: %w", deploymentConfig.Id(), err)
	}
	p("checking deployed version...")
	version, err := deploymentEngine.CheckVersion(deploymentConfig)
	if err != nil {
		return fmt.Errorf("%s: %w", deploymentConfig.Id(), err)
	}
	if version != deploymentConfig.Version() {
		p("version '%s' does not match expected version '%s'. Deploying new version...", version, deploymentConfig.Version())
		if len(deploymentConfig.PreDeployCommand()) > 0 {
			if err = runDeploymentScript("pre", deploymentConfig.PreDeployCommand(), deploymentConfig.Version(), deploymentConfig.Id(), p); err != nil {
				return fmt.Errorf("%s: %w", deploymentConfig.Id(), err)
			}
		}
		if err = deploymentEngine.Deploy(deploymentConfig, p); err != nil {
			return fmt.Errorf("%s: %w", deploymentConfig.Id(), err)
		}
		runGlobalCommands(globalPostDeployCommands, deploymentConfig.Id(), deploymentConfig.Version(), p)
		if len(deploymentConfig.PostDeployCommand()) > 0 {
			if err = runDeploymentScript("post", deploymentConfig.PostDeployCommand(), deploymentConfig.Version(), deploymentConfig.Id(), p); err != nil {
				return fmt.Errorf("%s: %w", deploymentConfig.Id(), err)
			}
		}
		p("version %s deployed successfully!", deploymentConfig.Version())
		return nil
	}
	p("version '%s' matches expected version '%s'. Skipping...", version, deploymentConfig.Version())
	return nil
}
```

- [ ] **Step 5: Run new tests to confirm they pass**

```bash
go test ./deployments/... -run TestRunGlobalCommands -v
```

Expected: PASS (all three tests).

- [ ] **Step 6: Run all tests**

```bash
go test ./...
```

Expected: all tests pass.

- [ ] **Step 7: Build**

```bash
go build ./...
```

Expected: builds successfully.

- [ ] **Step 8: Commit**

```bash
git add deployments/deploy.go deployments/deploy_test.go
git commit -m "feat: add global post-deploy-commands executed after each successful deployment"
```

---

### Task 6: Push and open PR

- [ ] **Step 1: Push the feature branch**

```bash
git push -u origin feature/global-post-deploy-commands
```

- [ ] **Step 2: Open pull request**

```bash
gh pr create \
  --title "feat: add global post-deploy-commands in settings block" \
  --body "$(cat <<'EOF'
## Summary

- Adds a top-level `settings.post-deploy-commands` block to the ploy YAML config
- Each command in the list runs sequentially after every successful service deployment, before the per-service `post-deploy-command`
- Global command failures are logged and ignored — they do not block subsequent global commands, the per-service command, or cause `ploy deploy` to exit non-zero
- All deployment scripts (pre, post, global post) now receive both `VERSION` and `DEPLOYMENT_ID` env vars

## Usage

\`\`\`yaml
settings:
  post-deploy-commands:
    - [./scripts/notify.sh, arg1]
    - [./scripts/other.sh]
deployments:
  - id: my-service
    type: lambda
    version: v1.0.0
    post-deploy-command: [./scripts/service-specific.sh]  # still works, runs after global commands
\`\`\`

## Test plan

- [ ] Verify `go test ./...` passes
- [ ] Manually test with a config file containing `settings.post-deploy-commands` pointing to `echo` scripts
- [ ] Verify `DEPLOYMENT_ID` and `VERSION` are set in the script environment
- [ ] Verify a failing global command does not prevent the per-service command from running
- [ ] Verify configs without `settings:` continue to work unchanged
EOF
)"
```

---
