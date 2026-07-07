# Releasing okashi

Releases are automated by [GoReleaser](https://goreleaser.com) when you push a version
tag. Each release builds prebuilt **macOS + Linux** binaries (`arm64` + `amd64`, pure-Go,
no cgo) and uploads them plus a `checksums.txt` to the GitHub Release.

> **Homebrew is not wired yet** (see below). Current releases are **binaries + checksums only**.

## Cutting a release

```sh
# 1. Validate the config and do a local dry run (recommended):
goreleaser check
goreleaser release --snapshot --clean   # builds into ./dist, uploads nothing

# 2. Tag and push — the `release` workflow does the rest:
git tag v0.1.0
git push origin v0.1.0
```

The workflow runs `go vet` + `go test` as a release gate (via GoReleaser's `before` hooks),
then builds and publishes. `okashi --version` reports the tag. Users install by downloading
the archive for their OS/arch from the Releases page and putting `okashi` on their `PATH`
(see the README's Install section; on macOS, clear the Gatekeeper quarantine flag once).

## Notes

- The version is injected via `-ldflags "-X main.version=<tag>"` (see `.goreleaser.yaml`).
- The optional Apple grammar backend (`make apple`, cgo) is **not** shipped in releases —
  the distributed binaries are the pure-Go default build. It gates at runtime anyway.
- `go.mod` sets the Go version the CI toolchain uses (`actions/setup-go` reads it).

## Planned: Homebrew (not yet wired)

GoReleaser deprecated the old `brews:` (formula) support in favor of `homebrew_casks:`, so
adding `brew install` is a deliberate follow-up, not a flip of a switch. When we do it:

1. **Migrate `.goreleaser.yaml`** — add a `homebrew_casks:` block (`repository`,
   `url.verified`, `hooks.post.install`/`test`). Casks install the **unsigned** binary, which
   macOS Gatekeeper quarantines — so include a `postflight`/hook that runs
   `xattr -dr com.apple.quarantine` on the installed binary, or `brew install` produces a
   binary that won't launch. Dry-run with `goreleaser release --snapshot --clean` first.
2. **Create the tap repo** — a **public** repo named exactly `homebrew-okashi` under the
   account (e.g. `snackztime/homebrew-okashi`); an empty repo is fine, GoReleaser writes the
   cask into it.
3. **Add the token** — a GitHub personal access token with **write access to the tap repo's
   contents**, added to *this* repo's **Settings → Secrets and variables → Actions** as
   `HOMEBREW_TAP_GITHUB_TOKEN` (the release job already passes it through).

Then a tagged release also publishes the cask, and `brew install snackztime/okashi/okashi`
works. Until then, the README lists Homebrew as "coming soon" and leads with the binary
download.
