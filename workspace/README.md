# workspace

A CLI that runs a given command in every module of the Go workspace (as defined in `go.work`). Useful for running linters, tests, or any script across all modules in parallel or sequentially

## Requirements

- Run from any directory: `go.work` is looked up from the current directory up to the git repo root (or set `-workfile` to point to a `go.work` file)

## Usage

```bash
go run ./workspace [options] <command> [args...]
```

Positional arguments after flags are the command run in each module (e.g. `golangci-lint run`). If none are given, selected module paths are printed

### Options

| Flag | Description |
|------|-------------|
| `-workfile=<path>` | Path to `go.work`. Default: looked up in current directory and parents until repo root |
| `-parallel=N` | Max concurrent modules: 0=auto (min(NumCPU, modules), default), 1=sequential, 2+=N-way. Must be >= 0 |
| `-only=<patterns>` | Comma-separated **suffix regexes** to include. Module directory path (with `/`) is matched from the end. E.g. `service,common` or `libs/pkg/.*` |
| `-exclude=<patterns>` | Comma-separated **suffix regexes** to exclude (same matching as `-only`) |
| `-override=<modules:command>` | Override the command for a list of modules: `module1,module2,...:command` (e.g. `service,common:golangci-lint run`). Module directory paths are suffix-matched |

**Suffix matching:** Patterns are anchored at the end of the module directory path. So `service` matches `apps/service`, `common` matches `libs/pkg/common` and `libs/pkg/shared-common`, and `libs/pkg/.*` matches any module under `libs/pkg/` (e.g. `libs/pkg/config`, `libs/pkg/logging`).

## Examples

Run `golangci-lint run` in all workspace modules:

```bash
go run ./workspace golangci-lint run
```

Same, but use a custom command for one module and skip another:

```bash
go run ./workspace -override="service:golangci-lint run --config .golangci.service.yml" -exclude=examples golangci-lint run
```

Run only in service and common modules (suffix matching):

```bash
go run ./workspace -only=service,common -parallel=1 go build ./...
```

Run only in all `libs/pkg/` modules (regex from the end):

```bash
go run ./workspace -only=libs/pkg/.* -parallel=1 go build ./...
```

Run tests in selected modules with one override:

```bash
go run ./workspace -override="service:go test ./... -tags=integration" -exclude=examples,tools go test ./...
```
