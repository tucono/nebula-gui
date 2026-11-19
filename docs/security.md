<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# Security

The service runs with high privileges and accepts requests from local processes. **Anyone who can talk to the service can change this machine's networking.** Treat the IPC API as a privilege boundary.

## Threat model

| Threat | Mitigation |
|---|---|
| A local user without permission controls the VPN | Restrict the socket or pipe to a dedicated group (Linux/macOS) or a DACL (Windows). No TCP listener. |
| **The config is used to read arbitrary files as root.** For example `pki.key: /etc/shadow`: Nebula would read it as root and the error message could leak its contents. | The service **accepts only inline PEM** in `pki.*` and refuses file paths. The GUI reads files as the user and inlines them. |
| The config sets up harmful routing | `tun.routes` and `tun.unsafe_routes` can redirect any traffic. Show them clearly in the GUI before connecting, and consider a confirmation step for `unsafe_routes` and default routes (`0.0.0.0/0`). |
| The config makes Nebula listen on privileged or unexpected ports, or names the tun device after an existing interface | Validate `listen.port` and `tun.dev`, or limit them to an allowlist. |
| `sshd` debug console enabled by an imported config | `PUT /v1/config` rejects any `sshd.enabled` other than `false`. The console gives deep control over Nebula. |
| The private key leaks through the API or logs | `GET /v1/config` redacts `pki.key`. Never log the config. The state directory is `0700` and the files are `0600`, log files included. A test runs a real Nebula with the log file at debug level and scans it for key material. |
| Ping is used as a scanning or flooding tool | `count` 1–10, `timeout_ms` at most 15000, at most 4 pings in flight (one per peer) and 60 per minute, and only IPs inside this host's cert networks, never its own. |
| An old or tampered service binary | The GUI checks `api_version` and shows the service version. Until code signing (M5), each release carries `SHA256SUMS` ([Releases and installers](#releases-and-installers)). |
| A second copy of the GUI, or another program, drives the running GUI | The single-instance socket accepts one message, `show`, from the same user only ([One copy of the GUI](#one-copy-of-the-gui)). |
| **The updater is used to install malicious code as root/SYSTEM** | The manifest is signed with a minisign key whose public half is built into the binary. The artifact's SHA-256 must match the signed manifest. Platform code signatures are checked. Downgrades are refused. The feed URL can't be set over IPC, and the install endpoint takes only a version string ([updates.md](updates.md#verification-all-must-pass-before-installing)). |
| A downloaded installer is swapped before it runs | Downloads go to the service's `0700` state directory. The hash is checked again immediately before running the installer. |
| Privacy of update checks | The check sends only a plain HTTPS request to GitHub, with no identifiers or telemetry. Automatic checks can be turned off. |

## The installed service

### Privileges

| | Runs as | Can |
|---|---|---|
| Linux | the system user `nebula-guid`, not root | only `CAP_NET_ADMIN` and `CAP_NET_RAW` (ambient and bounding set), with `NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp` and only `/dev/net/tun` among devices. It can change networking and send ICMP, but can't read users' files, write system files outside its own directories, or install software. |
| macOS | root (launchd daemon) | everything; `utun` needs root. |
| Windows | LocalSystem | everything; Wintun needs SYSTEM. |

Nebula handles UDP from the internet and the API takes requests from every member of `nebula-gui`, so on Linux a bug in either reaches a process that can't take over the machine. The price is that the Linux service can't install updates (ADR-007 in [decisions.md](decisions.md)).

### Who can use the socket or pipe

| | Address | Access |
|---|---|---|
| Linux | `/run/nebula-gui/nebula-guid.sock` | owner `nebula-guid`, group `nebula-gui`, mode `0660` |
| macOS | `/var/run/nebula-guid.sock` | owner root, group `nebula-gui`, mode `0660` |
| Windows | `\\.\pipe\nebula-guid` | a protected DACL: SYSTEM and Administrators full access; the `nebula-gui` group and the installing user read and write |

Being in `nebula-gui` means being able to connect, disconnect, import or delete the config, change **Connect automatically**, ping and close tunnels: everything the API offers. It doesn't give access to the private key, which `GET /v1/config` redacts.

- **Who is added:** only a fresh install adds anyone, and only the person installing: on Linux `$SUDO_USER`, the user behind `$PKEXEC_UID`, or the active session's user on `seat0`; on macOS the console user; on Windows the person signed in to the installer's session. An upgrade never grants access, since it's often run by someone else. Everyone else is added by an administrator ([platforms.md](platforms.md)).
- **Autoconnect** applies to every user of the computer and connects before anyone signs in. It's off after every install and upgrade; only a `PUT /v1/settings` turns it on.
- **The state directory** (`/var/lib/nebula-gui`, `/Library/Application Support/nebula-gui`, `%ProgramData%\nebula-gui`) is `0700` with `0600` files, or SYSTEM and Administrators only on Windows. It holds the config with its private key, the settings and the service log. Uninstalling keeps it unless asked to remove it, so a removed app can leave a key behind; the README and [platforms.md](platforms.md) say how to remove it.

### One copy of the GUI

The GUI listens on a per-user socket so a second copy can ask the first to show its window: `$XDG_RUNTIME_DIR/nebula-gui/gui.sock` (Linux) or `os.UserCacheDir()/nebula-gui/gui.sock` (macOS) in a `0700` directory with a `0600` socket, or on Windows `\\.\pipe\nebula-gui-<user SID>` with a DACL for that user only, not SYSTEM or Administrators. It reads one line: `show` shows the window; anything else is ignored. It can't be used to connect, read status or reach the service.

### Releases and installers

- **Built in CI from a tag**, by `.github/workflows/release.yml` with only `contents: write` and no secrets. It makes a **draft** release; a person publishes it.
- **Not signed until M5.** Windows (Edge, SmartScreen) and macOS (Gatekeeper) warn, and the README walks people past the warnings. On macOS the app and service are signed ad hoc, which Apple silicon requires, but that proves nothing about who built them. Windows builds keep their symbols and carry version details, which stopped Defender's machine-learning scan from quarantining the installer as `Program:Win32/Wacapew.A!ml`; signing is the lasting fix.
- **`SHA256SUMS`** lists every file in the release. It shows a download wasn't damaged or swapped on the way, but it sits on the same page as the files, so it doesn't help against someone who controls the release. For that, the person who runs the network can send the hash another way.
- **Wintun** isn't committed. CI downloads it from wintun.net and refuses it unless its SHA-256 matches `packaging/windows/wintun.sha256` (version 0.14.1).
- **Dependencies' licenses** ship as `THIRD_PARTY_NOTICES.txt`, generated from the modules actually built in ([development.md](development.md#packages)).

## Service log file

The service keeps a persistent log so a troubleshooting report can include what happened before the GUI was opened.

- **Where:** `<state dir>/logs/nebula-guid.jsonl`, for example `/var/lib/nebula-gui/logs/` on Linux. It rotates at 2 MiB and keeps `.1` and `.2`, so it takes at most about 6 MiB.
- **Who can read it:** the service only. The directory is `0700` and the files are `0600`. On Windows, the log directory gets the same SYSTEM and Administrators DACL as the state directory. Users read it through `GET /v1/diagnostics`, behind the socket permissions.
- **What's in it:** the service's lines and Nebula's, at debug level, as JSON Lines. That includes overlay and underlay IPs, host names and cert names, which is why the GUI masks reports before they're shared. It never includes certificates, keys or the config: rule 3 below applies to every log line. Successful `GET` requests aren't logged, because the GUI polls them every few seconds.
- **If it can't be opened**, the service prints a warning and runs without it. A broken log must never stop the VPN.

## GUI log and troubleshooting reports

### GUI log
- **Where:** `os.UserConfigDir()/nebula-gui/logs/nebula-gui.jsonl` (for example `~/.config/nebula-gui/logs/` on Linux), rotated at 1 MiB with one old file kept. Directory `0700`, files `0600`.
- **What's in it:** what the user did and what happened, with file names but never config contents or PEM. Paths in the home folder are written as `~/…`, and only the file name of any other path. A line containing `PRIVATE KEY` or `BEGIN NEBULA` is replaced with `[removed for safety]`.

### Troubleshooting reports
**Get help…** makes a report meant to be pasted into a chat, often a public AI assistant, so it's built to be safe to share:

- **Masked by default.** Public, LAN and link-local IPv6 addresses, overlay addresses, host and cert names, lighthouse DNS names, cert groups and fingerprints are replaced with consistent placeholders (`PUBLIC-1`, `HOST-2`, …). The home folder becomes `~` and the login name `USER`. **Include network details** turns this off, for reports sent to the network's admin.
- **Names are collected, not guessed.** They come from the status, the config summary, the peers' cert names and groups, known log fields and remembered names. In free text only whole words of 4 or more characters that aren't on a stop list are replaced. A name that appears nowhere else, or a very short one, can stay in free text; every IP address is masked by range regardless.
- **Saved peers** are kept in `os.UserConfigDir()/nebula-gui/peers.json` (`0600`): a name and an overlay IP per peer. They stay when the config is removed.
- **Remembered names** are kept in `os.UserConfigDir()/nebula-gui/mask-names.json` (`0600`), so a report made while the service can't be reached still masks the GUI log. Peer cert names and groups are added from every peer poll. They're deleted when the config is removed.
- **Never in a report:** keys, certificates or the config. Certificates in log lines are shortened to name, groups, networks, expiry and issuer; public keys and signatures are dropped. Every input line containing `PRIVATE KEY` or `BEGIN NEBULA` is dropped and counted, and a finished report that still contains either is refused.
- **Saved peer names** (M3) are never in a report, masked or not. They're personal ("Mom's laptop"), they don't help troubleshooting, and free-form names can't be masked reliably. The GUI log records peer actions by IP only. The report's Peers section uses the cert names and addresses from `GET /v1/peers`, which are masked like everything else.
- **Saved reports** are written with mode `0600`. Nothing is sent anywhere by the app: the user copies or saves the report.

## Rules for contributors
1. New service endpoints must validate every input. Request bodies are capped (1 MiB, which is plenty for a config).
2. The service never opens a path that came from a client.
3. Secrets do not go in logs, error messages, or panics. Note that Nebula's config test (`nebula.Main` with `configTest=true`) logs the whole config, private key included, at info level. `nebulactl.Check` gives it a logger that discards everything.
4. Keep the `nebula` dependency up to date for security fixes, and follow Nebula's security advisories.
