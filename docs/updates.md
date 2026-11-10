# Updates (service branch)

On this branch there are **two independent update streams**:

| Stream | What changes | How often | How it's installed | Tunnel downtime |
|---|---|---|---|---|
| **Nebula binary** | `<state>/nebula/<ver>/nebula` | Each upstream Nebula release | Download, verify, switch, health check, automatic rollback | About 1–3 s (process restart) |
| **App** | `nebula-gui` + `nebula-guid` | Our releases | Platform installer (MSI, pkg, deb/rpm), as on the `monolithic` branch | A few seconds (service restart) |

## Nebula binary updates

### Channels
The user picks one in Settings → Updates. **`curated` is the default.**

| Channel | Source of truth | Trust anchor | Reaches users |
|---|---|---|---|
| `curated` | **Our signed channel manifest**, which lists upstream Nebula versions we've tested, their download URLs and SHA-256s | Our minisign key, built into the service | After we've tested and approved the version. Updating the manifest doesn't need an app release. |
| `upstream` | The latest release on `api.github.com/repos/slackhq/nebula/releases` | HTTPS to GitHub + upstream's `SHASUM256.txt` (not separately signed) | As soon as upstream publishes |
| `pinned` | The user stays on one version | — | Never; it only shows a notice |
| *(custom binary)* | Admin-set path | The admin | Updates are turned off |

`upstream` is an **advanced** option. It only allows versions inside the service's supported range, and warns before installing a version newer than the newest one we've tested.

### Curated channel manifest
It's hosted as a file that can be updated, separately from app releases, for example as an asset on a fixed `nebula-channel` GitHub release, or on GitHub Pages:

```json
{
  "schema": 1,
  "generated_at": "2026-12-02T00:00:00Z",
  "sequence": 42,                       // increases with every update; older ones are rejected (replay protection)
  "versions": [
    {
      "version": "1.10.0",
      "security": false,
      "notes_url": "https://github.com/slackhq/nebula/releases/tag/v1.10.0",
      "min_service_version": "0.3.0",   // older services must update the app first
      "artifacts": {
        "linux-amd64":   { "url": "https://github.com/slackhq/nebula/releases/download/v1.10.0/nebula-linux-amd64.tar.gz", "sha256": "…", "binary_sha256": "…" },
        "linux-arm64":   { "url": "…", "sha256": "…", "binary_sha256": "…" },
        "darwin":        { "url": "…/nebula-darwin.zip", "sha256": "…", "binary_sha256": "…" },
        "windows-amd64": { "url": "…/nebula-windows-amd64.zip", "sha256": "…", "binary_sha256": "…", "wintun_sha256": "…" }
      }
    }
  ],
  "revoked": ["1.9.1"]                  // versions that are known bad; switched away from on the next check
}
```

The downloads **come straight from upstream GitHub releases**. We don't host Nebula binaries. Our signature confirms which upstream files we've tested. Signature file: `nebula-channel.json.minisig`.

Release process for us: a CI job watches upstream releases, runs the version matrix tests ([testing.md](testing.md)), and on success opens a PR that adds the version to the manifest. A maintainer reviews, merges, and signs the manifest.

### Verification
1. **Curated:** the manifest's minisign signature is valid and `sequence` is newer than the last one seen.
   **Upstream:** the release is fetched over HTTPS and `SHASUM256.txt` is downloaded from the same release.
2. The archive's SHA-256 matches. After extracting, the binary's SHA-256 also matches (curated only).
3. **Platform signature, where upstream provides one.** For example, on macOS run `codesign --verify --strict` and check the Team ID. Record whether a signature was present. A signature that was present before and is missing now is a hard failure.
4. Run `<new>/nebula -version` and check that its output matches the expected version.
5. Run `<new>/nebula -test -config run/config.yml`. Nebula's own config test must pass with the user's current config.

Archives are extracted with path-traversal protection. Only the expected file names are extracted: `nebula`, `nebula.exe`, and `wintun.dll` for the right architecture.

### Switch, health check, rollback
1. Install into `nebula/<new>/` with restricted permissions (see [security.md](security.md)).
2. If connected: stop the old process, write `active.json` (`version=new`, `previous=old`) atomically, and spawn the new one. If disconnected: just update `active.json`.
3. **Health check** within 30 s: the process is still running, the SSH console answers `version` with the new version, and `print-cert` succeeds. If the lighthouse was reachable before the update, it must be reachable again within 60 s. **That last check only produces a warning**, because network problems shouldn't trigger a rollback.
4. If the health check fails, **roll back automatically**: switch to `previous`, mark the new version `bad` (it won't be offered again unless the user asks), and tell the GUI.
5. Keep the active and previous versions plus the bundled one. Delete older ones.

Users can also roll back manually from the GUI (`POST /v1/nebula/rollback`).

### Scheduling
- Check at startup and every 24 h ± 2 h.
- Auto-install settings: `off`, `security-only` (the default), or `all`. "Install when disconnected" works the same as on the monolithic branch.

## App updates
Same as the `monolithic` branch: a signed `manifest.json` attached to each nebula-gui release, the platform installer, and the same verification rules. The one difference: **the installer doesn't replace managed Nebula binaries**. It only updates the `bundled` fallback copy. After an app update, the service checks that the active Nebula version is still inside its supported range. If not, it switches to the newest supported installed version and tells the user.

## Linux package-manager installs
If the app itself was installed from a distro repo, app updates only notify, as before. **Managed Nebula binary updates still work**, because they live in the service's own state directory and not in package-manager paths. If the admin set a custom binary like `/usr/bin/nebula`, Nebula updates are left to the package manager.

## IPC endpoints
See [ipc-api.md](ipc-api.md#updates).
