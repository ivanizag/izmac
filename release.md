# Releasing izmac

Releases are triggered by pushing a git tag, followed by one manual review step.

1. Pick the next version, following the existing `vX.Y.Z` tags (`git tag --sort=-creatordate | head`).
2. Tag and push:

   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```

3. GitHub Actions (`.github/workflows/release.yml`) builds the windowed frontend for Linux, Windows and macOS and creates a **draft** GitHub Release with the archives attached and an auto-generated changelog (from commit/PR titles since the last tag).
4. Review the draft release on GitHub, then click **Publish release**.
5. Publishing fires `.github/workflows/homebrew.yml`, which pushes an updated formula to [ivanizag/homebrew-tap](https://github.com/ivanizag/homebrew-tap), so `brew install ivanizag/tap/izmac` picks it up. Nothing is pushed to Homebrew while the release stays a draft.

That's it — no version bump in code, no changelog file to edit.

## What is built

Only the windowed frontend, `frontend/macebiten`, as `izmac`. The headless one
is a debugging tool and is not shipped.

| Archive | How it is built |
|---|---|
| `izmac-linux-amd64.tar.gz` | in an `ubuntu:22.04` container, so it runs on any distro with glibc 2.35 or newer. Ebitengine needs cgo here, so it is a native build. |
| `izmac-windows-amd64.zip` | cross compiled from Linux with `CGO_ENABLED=0`. Ebitengine reaches the Windows APIs without cgo, so there is no toolchain to install. |
| `izmac-macos-universal.tar.gz` | built on macOS for both arm64 and amd64 and joined with `lipo`. Ebitengine needs cgo here too. |

Each archive holds the binary, `README.md` and `LICENSE`, flat.

The ROM is copyrighted and is never packaged. A machine without one downloads
it on the first run, which is what `-rom` and `default.rom` are about.

## Requirements

- The `TAP_GITHUB_TOKEN` repo secret must be set (a PAT with push access to `ivanizag/homebrew-tap`), or the homebrew.yml step will fail.
- `update-izmac.sh` must be present in the tap repository. It is what homebrew.yml runs to rewrite `Formula/izmac.rb`.
