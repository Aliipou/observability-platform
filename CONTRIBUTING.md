# Contributing to observability-platform

## Setup

```bash
git clone https://github.com/Aliipou/observability-platform.git
cd observability-platform
go mod download
docker compose up -d  # Start dependencies
```

## Running Tests

```bash
go test ./... -v -race
```

## Code Style

- `gofmt` and `golangci-lint` must pass
- All exported types and functions need GoDoc comments
- Error wrapping with `fmt.Errorf("context: %w", err)` required

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):
- `feat:` new feature
- `fix:` bug fix
- `docs:` documentation only
- `chore:` tooling or config
- `test:` tests only

## Pull Requests

1. Fork the repo and create a feature branch
2. Write tests for any new behavior
3. Run `golangci-lint run` and fix any issues
4. Open a PR with a clear description of what and why

## Adding a New Alert Rule

Add your rule to `config/prometheus/rules/` as a YAML file following the existing format. Include a comment explaining the threshold choice.
