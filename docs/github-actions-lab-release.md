# GitHub Actions — signed lab release (one-time setup)

This document describes the **one-time** GitHub configuration for fully automated lab releases after P1D merges. **Do not run a release workflow until you intend to publish a new version.**

Public site base URL (unchanged): `https://faridze.github.io/pve-zfs-monitor`

## What changes for operators

- **Before:** build and sign on a maintainer machine, push to `gh-pages`, wait for Pages rebuild.
- **After:** trigger **Lab release (signed)** from GitHub Actions on `main`; build, Ed25519 sign, verify, and deploy run in CI.
- **Trade-off:** GitHub holds a protected copy of the lab signing private key (`PVE_ZFS_MONITOR_LAB_SIGNING_KEY_PEM`). You lose strict offline-key isolation in exchange for autonomous releases. The approved public fingerprint remains:

  `sha256:1307dc2b092cd88dde7f27461f3c5f48620889f870b25fe5515f427e54e14c8a`

Never paste the private key into issues, PRs, or this repository.

## One-time GitHub settings

### 1. Protected environment `lab-release-signing`

1. Repository **Settings → Environments → New environment**
2. Name: **`lab-release-signing`**
3. **Deployment branches:** limit to **`main`** (or your protected default branch)
4. **Required reviewers:** enable and add the **repository owner** (manual approval before sign/deploy jobs run)

### 2. Environment secret

In environment **`lab-release-signing`**, add:

| Name | Value |
|------|--------|
| **`PVE_ZFS_MONITOR_LAB_SIGNING_KEY_PEM`** | Full PKCS#8 PEM for the approved Ed25519 lab key (same material as offline signing) |

The workflow writes this to a short-lived file on the runner, verifies the fingerprint, signs, then discards the runner. The secret is **not** available to `pull_request` workflows or forks.

### 3. GitHub Pages source → GitHub Actions

When you are ready to switch from branch-based Pages:

1. **Settings → Pages**
2. **Build and deployment → Source:** select **GitHub Actions** (not “Deploy from a branch”)

Until you change this, the workflow can still sign and verify, but **`actions/deploy-pages` will not update the live site**. Existing content on `gh-pages` (including signed **`1.0.0-rc1`**) remains served until the first successful Actions deployment.

### 4. Branch protection on `main`

Ensure `main` matches your review policy (required reviews, no force-push). The workflow only accepts:

- `workflow_dispatch` triggered **from** `main`
- `git_sha` that is an ancestor of current `main`
- versions not already present at `{RELEASE_BASE}/lab/{version}/manifest.json`

## How to trigger a lab release (after setup)

1. Merge reviewed release tooling and application changes to **`main`**.
2. Note the exact commit SHA to ship.
3. **Actions → Lab release (signed) → Run workflow**
4. Branch: **`main`**
5. Inputs:
   - **version:** semver (e.g. `1.0.0-rc2`)
   - **git_sha:** full 40-character commit on `main`
   - **allow_migration_bootstrap:** set to **true** only for the **first** Actions migration while live `lab/latest` is still **`1.0.0-rc1`** and `pages-deploy-manifest.txt` is absent. Leave **false** for all later releases.
6. Approve the **`lab-release-signing`** environment when prompted.
7. Confirm deployment in the workflow summary and verify:
   - `https://faridze.github.io/pve-zfs-monitor/lab/{version}/manifest.json`
   - `https://faridze.github.io/pve-zfs-monitor/lab/latest/manifest.json`

The **`stable`** channel is not modified by this workflow.

## Migration notes

- The deploy job uses **`prepare-pages-site-baseline.sh`**: every release after the first migration loads the **currently deployed live site** via **`pages-deploy-manifest.txt`** (strict fetch + inventory verification). **`gh-pages`** is used only for the one-time migration when the live inventory is absent (HTTP 404), with byte-level checks against the live trusted snapshot.
- After merge, **`materialize-public-pages-site.sh`** builds a clean static tree (no `.git/`, repo internals, or symlinks). The uploaded artifact and inventory include **only** allowlisted public paths (`scripts/`, `lab/`, `stable/`, etc.).
- **`lab/latest`** is promoted only from a verified **`lab/<version>/`** directory whose basename matches the manifest `version` field and contains every artifact listed in the manifest.
- The workflow derives the release version from the **signed manifest** and requires an exact match to the `version` workflow input (artifact directories named `unsigned/` are supported).
- Duplicate-version checks accept only explicit **HTTP 404** as “not published”; TLS, network, and 5xx errors abort validation.
- Immutable version directories are not overwritten with different content.
- `lab/.published-versions` and `pages-deploy-manifest.txt` track published lab versions and the full deploy file list.

## Security limitations

- Anyone who can approve `lab-release-signing` and dispatch the workflow on `main` can sign and publish lab builds for approved SHAs.
- GitHub-hosted runners briefly hold the private key in memory; this is not HSM-backed offline signing.
- Compromised maintainer approval or stolen environment secret could publish malicious lab binaries—same class of risk as compromised offline key, but with a larger attack surface (GitHub account, Actions, secret store).

See also: `docs/p1d-lab-release-runbook.md` (offline flow), `SECURITY.md`, `.github/workflows/lab-release.yml`.
