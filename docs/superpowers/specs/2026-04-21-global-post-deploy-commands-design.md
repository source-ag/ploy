# Global Post-Deploy Commands

**Date:** 2026-04-21

## Summary

Add support for a list of global post-deploy commands that run after every successful service deployment, before any per-service `post-deploy-command`. Commands run sequentially. Failures are logged and ignored -- they do not block subsequent global commands or the per-service command.

## YAML Config Structure

A new optional top-level `settings:` block holds the global commands. Configs without it continue to work unchanged.

```yaml
settings:
  post-deploy-commands:
    - [./scripts/notify.sh, arg1]
    - [./scripts/other.sh]
deployments:
  - id: my-service
    type: lambda
    version: v1.0.0
    post-deploy-command: [./scripts/service-specific.sh]  # still works
```

`post-deploy-commands` is a list of commands. Each command is a list of strings (binary + arguments). Type: `[][]string`.

## Config Parsing (`deployments/config.go`)

- Add `Settings` struct:
  ```go
  type Settings struct {
      PostDeployCommands [][]string `yaml:"post-deploy-commands,omitempty"`
  }
  ```
- Extend the internal `Deployments` YAML struct with `Settings Settings` (mapped to `settings,omitempty`).
- Change `LoadDeploymentsFromFile` signature to return `([]engine.Deployment, Settings, error)`.
- Update callers:
  - `deployments/deploy.go`: capture settings
  - `deployments/verify.go`: discard settings with `_`

## Execution (`deployments/deploy.go`)

`Deploy` passes `settings.PostDeployCommands` into `doDeployment`. After a successful deploy, the order is:

1. For each command in `settings.PostDeployCommands` (sequential):
   - Run command
   - On failure: log error, continue to next command
2. Run per-service `post-deploy-command` if set (existing behavior, failure propagates as error)

## Environment Variables

Both global and per-service commands receive:

| Variable        | Value                        |
|-----------------|------------------------------|
| `VERSION`       | The version just deployed    |
| `DEPLOYMENT_ID` | The `id` of the deployment   |

`runDeploymentScript` is updated to accept and inject `DEPLOYMENT_ID`. The per-service command gains `DEPLOYMENT_ID` as well for consistency.

## Error Handling

| Command              | On failure          |
|----------------------|---------------------|
| Global command       | Log, continue       |
| Per-service command  | Return error (existing behavior) |

## Out of Scope

- A global `pre-deploy-commands` (can be added later via `settings:`)
- Per-service `post-deploy-commands` (plural list) -- not changed
