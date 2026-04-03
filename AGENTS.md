# Ploy

Terminal-based deployment tool for AWS services. Defines desired service versions in YAML config files and can verify/deploy them.

## Stack

- Go 1.23, modules
- CLI framework: Cobra
- AWS SDK v2 (Lambda, ECS, EventBridge)
- Config format: YAML

## Project Structure

- `cmd/` - CLI commands (deploy, verify, update)
- `engine/` - Deployment engines (Lambda, ECS, ECS Scheduled Tasks)
- `deployments/` - Config parsing, deploy/verify/update orchestration
- `utils/` - Shared helpers

## Development

No Makefile. Use standard Go commands:

```bash
go build ./...
go test ./...
go install .  # install locally
```

## CI/CD

- **PR**: `go build ./...` (`.github/workflows/pr-build.yml`)
- **Release**: Tag `v*.*.*` triggers GoReleaser (`.github/workflows/release.yml`). Create releases via GitHub UI. `source-deployments` picks up the latest release automatically.

## Architecture Notes

- Each deployment engine implements the `DeploymentEngine` interface in `engine/core.go` and registers itself via `RegisterDeploymentEngine` in an `init()` function.
- Config files are per-environment YAML (e.g. `development.yml`, `production.yml`).
- The `update` command only modifies the YAML file; it does not deploy. Run `deploy` separately.
