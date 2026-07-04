# Releasing okashi

Releases are automated by [GoReleaser](https://goreleaser.com) when you push a version
tag. Each release builds prebuilt **macOS + Linux** binaries (`arm64` + `amd64`, pure-Go,
no cgo), uploads them plus a `checksums.txt` to the GitHub Release, and updates the
Homebrew formula in the tap.

## One-time setup (for the Homebrew tap)

Binaries are published with zero setup. To also enable `brew install`:

1. Create a **public** tap repo named exactly `homebrew-okashi` under your account
   (e.g. `snackztime/homebrew-okashi`). An empty repo is fine — GoReleaser writes the
   formula into it.
2. Create a GitHub **personal access token** (classic or fine-grained) with **write access
   to that tap repo's contents**, and add it to *this* repo's
   **Settings → Secrets and variables → Actions** as `HOMEBREW_TAP_GITHUB_TOKEN`.

(If you skip this, delete the `brews:` block in `.goreleaser.yaml` — the release will still
publish binaries + checksums.)

## Cutting a release

```sh
# 1. Validate the config and do a local dry run (optional but recommended):
goreleaser check
goreleaser release --snapshot --clean   # builds into ./dist, uploads nothing

# 2. Tag and push — the `release` workflow does the rest:
git tag v0.1.0
git push origin v0.1.0
```

The workflow runs `go vet` + `go test` as a release gate (via GoReleaser's `before` hooks),
then builds and publishes. `okashi --version` reports the tag.

## Notes

- The version is injected via `-ldflags "-X main.version=<tag>"` (see `.goreleaser.yaml`).
- The optional Apple grammar backend (`make apple`, cgo) is **not** shipped in releases —
  the distributed binaries are the pure-Go default build. It gates at runtime anyway.
- `go.mod` sets the Go version the CI toolchain uses (`actions/setup-go` reads it).
