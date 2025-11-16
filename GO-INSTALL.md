# Local Go Toolchains (project-local)

This repo vendors Go toolchains under `.tooling/` so we can build/test even if the host lacks Go or has an older version.

## Installing Go 1.23.4 locally

1. Download and unpack:

```bash
curl -L https://go.dev/dl/go1.23.4.linux-amd64.tar.gz -o /tmp/go1.23.4.tar.gz
mkdir -p .tooling
tar -C .tooling -xzf /tmp/go1.23.4.tar.gz
mv .tooling/go .tooling/go1.23.4
```

2. Use the toolchain for commands:

```bash
GOTOOLCHAIN=local \
GOCACHE=$(pwd)/.tooling/gocache \
GOPATH=$(pwd)/.tooling/gopath \
GOMODCACHE=$(pwd)/.tooling/gopath/pkg/mod \
.tooling/go1.23.4/bin/go test ./...
```

3. Add to PATH (optional):

```bash
export PATH="$(pwd)/.tooling/go1.23.4/bin:$PATH"
```

Notes:
- We pin `go.mod` to `go 1.23` and `toolchain go1.23.4`.
- Cache/module paths stay inside the repo to avoid permission issues in hosts with locked-down home dirs.
