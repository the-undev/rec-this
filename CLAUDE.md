# CLAUDE.md

## Commands

| What | Command |
|---|---|
| Full check | `make check`: gofmt, golangci-lint, `go test ./...` and the build |
| Dev instance | The installed `rec-this` in `/usr/local/bin`, writing to `~/Recordings`. Nothing touches it. |
| Isolated run | `./rec-this -o "$(mktemp -d)" -t 5`. No UI and no port. It writes only to its own temporary directory and reads the live PipeWire monitor without changing it. |

## Workflow

No PRs. A ticket or integration branch merges into main locally with a merge
commit, then main is pushed to `origin`, which pushes to Forgejo and GitHub.
