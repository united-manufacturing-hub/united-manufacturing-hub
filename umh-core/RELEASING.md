# Releasing umh-core

## Overview

All development happens on `staging`. To release, merge `staging` into `main` via PR, then create a GitHub Release on `main`. Creating the Release also creates the git tag, which triggers all downstream automation.

## Changelog convention

During development, all changelog entries go under `## Unreleased` at the top of `CHANGELOG.md`. At release time, this section is renamed to the version number (e.g., `## [0.44.12]`) and a fresh empty `## Unreleased` is added above it. This keeps it clear during PR review which entries are pending release, and defers the version number decision to release time.

## Pre-release (nightly)

1. Create a PR from `staging` → `main`, review and merge
2. Go to [Releases > Draft a new release](https://github.com/united-manufacturing-hub/united-manufacturing-hub/releases/new)
3. Create a **new tag** with the format `v0.X.Y-pre.N` (e.g., `v0.44.10-pre.1`)
4. Target: `main`
5. Check **"Set as a pre-release"**
6. Body can be minimal — pre-releases don't get changelog sync or release notes automation
7. Click **Publish release**

### What happens automatically

| Workflow | Trigger | What it does |
|----------|---------|-------------|
| `build-umh-core.yml` | tag push | Builds Docker image, pushes to GHCR, verifies binaries, notifies MC webhook with `channel: nightly` |

## Stable release

1. In `CHANGELOG.md`, rename `## Unreleased` to the version being released (e.g., `## [0.44.12]`) and add a fresh empty `## Unreleased` section above it. Review all entries.
2. Create a PR from `staging` → `main`, review and merge
3. Go to [Releases > Draft a new release](https://github.com/united-manufacturing-hub/united-manufacturing-hub/releases/new)
4. Create a **new tag** with the format `v0.X.Y` (e.g., `v0.44.10`) — no `-pre.` suffix
5. Target: `main`
6. Body (optional): paste the CHANGELOG.md section for this version as a fallback. Automation will overwrite it with a formatted version including the changelog.umh.app link. If automation fails, the original body is preserved.
7. Do NOT check "Set as a pre-release"
8. Click **Publish release**

### What happens automatically

| Workflow | Trigger | What it does |
|----------|---------|-------------|
| `build-umh-core.yml` | tag push | Builds Docker image, pushes to GHCR, verifies binaries, notifies MC webhook (see promotion logic below) |
| `sync-changelog.yml` | tag push | Extracts version section from CHANGELOG.md, creates PR on [changelog.umh.app](https://github.com/united-manufacturing-hub/changelog.umh.app) |
| `update-github-release.yml` | tag push | Extracts version section from CHANGELOG.md, overwrites the GitHub Release body, appends changelog.umh.app link |

### MC webhook promotion logic

When notifying Management Console of a stable release:

- If the current nightly's base version matches the stable tag (e.g., nightly `v0.44.10-pre.3` → stable `v0.44.10`), the nightly is **promoted** to stable
- Otherwise, a new stable version entry is created directly

Both staging and production MC instances are notified.

## Post-release checklist

- [ ] Verify the Docker image is available: `docker pull ghcr.io/united-manufacturing-hub/umh-core:v0.X.Y`
- [ ] Verify the GitHub Release body was updated with CHANGELOG.md content
- [ ] Merge the changelog.umh.app PR created by `sync-changelog.yml` (the changelog link in the GitHub Release is a 404 until this PR is merged)
- [ ] Verify MC shows the new version (check both staging and production)
- [ ] Bump the version section on `staging`: ensure `## Unreleased` exists at the top of `CHANGELOG.md` for the next cycle

## Troubleshooting

### GitHub Release body is empty or wrong
Check the `update-github-release.yml` run in the Actions tab. Common causes: missing `## [X.Y.Z]` section in CHANGELOG.md (shows as warning in the workflow summary). Re-run via Actions > Update GitHub Release > Run workflow > enter the tag name.

### changelog.umh.app entry was not created
Re-trigger via Actions > Sync Changelog > Run workflow > enter the version. Supports `dry_run` mode for testing.

### Docker image was not pushed
Check the `build-umh-core.yml` run. If the build job failed, the tag still exists. Fix the issue and re-run the workflow from the Actions tab.

### MC was not notified of new version
Check the `notify-management-console` job in the `build-umh-core.yml` run. The webhook can be re-sent by re-running that job.

## Workflow files

| File | Purpose |
|------|---------|
| `.github/workflows/build-umh-core.yml` | Build, verify, notify MC |
| `.github/workflows/sync-changelog.yml` | Sync CHANGELOG.md to changelog.umh.app |
| `.github/workflows/update-github-release.yml` | Populate GitHub Release body from CHANGELOG.md |
| `.github/workflows/pull-request.yml` | PR CI (calls build-umh-core.yml as reusable workflow) |
