# Updates

The app can check for updates and install them. Nebula is **built into** `nebula-guid` (ADR-002), so a "Nebula update" means a new nebula-gui release with a newer `github.com/slackhq/nebula` pinned. There is no separate Nebula binary to replace. See ADR-006 in [decisions.md](decisions.md).

## What the user sees

- **Settings → Updates:**
  - "Check for updates automatically" (on by default) and a "Check now" button
  - Current versions: app, embedded Nebula, service API
- **When an update is available:** a banner and tray notification such as *"nebula-gui 0.4.0 is available (Nebula 1.9.3 → 1.10.0)"*, with release notes and buttons for **Install now**, **Install when disconnected**, and **Skip this version**.
- **Upstream notice (informational only):** if slackhq/nebula has a newer release than the one we embed, and no nebula-gui release includes it yet, show *"Nebula 1.10.1 was released upstream; waiting for a nebula-gui update."* If the upstream release is marked as a security fix, highlight the notice.

## Who does what

| Step | Process | Why |
|---|---|---|
| Scheduled check (startup + every 24 h ± jitter) | Service | Works when the GUI is closed. One check per machine, not one per user. |
| Download + verification | Service | Writes to its own `0700` state directory, so other users can't swap the file. |
| Install | Service | Needs privileges to replace system binaries and restart itself. |
| Prompt, release notes, consent | GUI | The service never installs without a GUI request, except with the optional "auto-install" setting (M5+). |

## Update feed

The service reads a JSON **manifest** that is attached to each GitHub release of nebula-gui:

```
https://github.com/<owner>/nebula-gui/releases/latest/download/manifest.json
https://github.com/<owner>/nebula-gui/releases/latest/download/manifest.json.minisig
```

```json
{
  "version": "0.4.0",
  "nebula_version": "1.10.0",
  "api_version": 1,
  "released_at": "2026-12-01T00:00:00Z",
  "security": false,
  "notes_url": "https://github.com/<owner>/nebula-gui/releases/tag/v0.4.0",
  "min_upgrade_from": "0.1.0",
  "artifacts": {
    "windows-amd64": { "url": "…/nebula-gui-0.4.0-windows-amd64.msi", "sha256": "…", "size": 12345678 },
    "windows-arm64": { "url": "…", "sha256": "…", "size": 0 },
    "darwin-universal": { "url": "…/nebula-gui-0.4.0.pkg", "sha256": "…", "size": 0 },
    "linux-amd64-deb": { "url": "…", "sha256": "…", "size": 0 },
    "linux-amd64-rpm": { "url": "…", "sha256": "…", "size": 0 }
  }
}
```

- The feed URL is a build-time constant. Packagers and forks can override it with a service config setting. It can **never** be set through the IPC API.
- The upstream notice reads `https://api.github.com/repos/slackhq/nebula/releases/latest`, without authentication. Unauthenticated requests are limited to 60/hour per IP, which is far more than one call per day needs. Cache the response with its ETag.

## Verification (all must pass before installing)

1. **Manifest signature:** `manifest.json.minisig` is checked against the **ed25519 public key built into the binary** (minisign format, e.g. `github.com/jedisct1/go-minisign`). HTTPS alone isn't enough, because it doesn't protect against a compromised GitHub account or release.
2. **No downgrades:** `manifest.version` must be newer than the running version (semver). A replayed old manifest is ignored.
3. **Artifact hash:** the SHA-256 of the downloaded file matches the signed manifest. The size is checked before downloading the whole file.
4. **Platform signature (defense in depth):**
   - Windows: the MSI's Authenticode signature is valid and from our certificate.
   - macOS: `pkgutil --check-signature` shows our Developer ID and the package is notarized.
   - Linux: covered by step 3. Packages can additionally be GPG-signed.

The minisign private key is kept offline or in CI secrets, separate from GitHub write access. Rotating the key needs a release signed with the old key that ships the new public key.

## Install mechanics per platform

The service starts the platform installer. The installer replaces **both** binaries (GUI + service) and restarts the service.

| Platform | Install command | Notes |
|---|---|---|
| Windows | `msiexec /i <file> /qn /norestart` started as a detached process | The MSI stops the service, replaces the files using Restart Manager for the running GUI, and starts the service again. |
| macOS | `installer -pkg <file> -target /` started detached | The package's postinstall step reloads the launchd daemon (`launchctl bootout` / `bootstrap`). |
| Linux, `.deb`/`.rpm` from **our download** | `apt-get install -y ./file.deb` / `dnf install -y ./file.rpm` | Supported. |
| Linux, from a **distro or third-party repo** | — | **Don't self-update.** Only show "Update available. Update through your package manager." The service detects this by checking whether its package came from our repo. |

Because the installer restarts the service, **the tunnel goes down for a few seconds**. With "Install when disconnected", the update waits until the user disconnects or the machine is idle. After restarting, the service reconnects if it was connected before the update. It writes a `resume-after-update` marker before running the installer.

## Failure handling

- Download and verification errors are shown in the GUI and logged. Nothing is installed. Partial files are deleted.
- **After an install, the new service reports its version.** If the GUI can't reach a service with the expected version within 60 s, it shows "Update may have failed. See logs" and the path to the installer log. MSI and pkg roll back on their own if the installer itself fails.
- The **GUI** sees a new `api_version` and asks the user to restart the GUI. On Windows and macOS the installer usually relaunches it.

## IPC endpoints

See [ipc-api.md](ipc-api.md#updates).
