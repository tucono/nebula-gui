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
| Ping is used as a scanning or flooding tool | Limit `count` and `timeout_ms`, rate-limit requests, and accept only IPs inside the overlay subnets. |
| An old or tampered service binary | Sign releases. The GUI checks `api_version` and shows the service version. |
| **The updater is used to install malicious code as root/SYSTEM** | The manifest is signed with a minisign key whose public half is built into the binary. The artifact's SHA-256 must match the signed manifest. Platform code signatures are checked. Downgrades are refused. The feed URL can't be set over IPC, and the install endpoint takes only a version string ([updates.md](updates.md#verification-all-must-pass-before-installing)). |
| A downloaded installer is swapped before it runs | Downloads go to the service's `0700` state directory. The hash is checked again immediately before running the installer. |
| Privacy of update checks | The check sends only a plain HTTPS request to GitHub, with no identifiers or telemetry. Automatic checks can be turned off. |

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
- **Names are collected, not guessed.** They come from the status, the config summary, known log fields and remembered names. In free text only whole words of 4 or more characters that aren't on a stop list are replaced. A name that appears nowhere else, or a very short one, can stay in free text; every IP address is masked by range regardless.
- **Remembered names** are kept in `os.UserConfigDir()/nebula-gui/mask-names.json` (`0600`), so a report made while the service can't be reached still masks the GUI log. They're deleted when the config is removed.
- **Never in a report:** keys, certificates or the config. Certificates in log lines are shortened to name, groups, networks, expiry and issuer; public keys and signatures are dropped. Every input line containing `PRIVATE KEY` or `BEGIN NEBULA` is dropped and counted, and a finished report that still contains either is refused.
- **Saved reports** are written with mode `0600`. Nothing is sent anywhere by the app: the user copies or saves the report.

## Rules for contributors
1. New service endpoints must validate every input. Request bodies are capped (1 MiB, which is plenty for a config).
2. The service never opens a path that came from a client.
3. Secrets do not go in logs, error messages, or panics. Note that Nebula's config test (`nebula.Main` with `configTest=true`) logs the whole config, private key included, at info level. `nebulactl.Check` gives it a logger that discards everything.
4. Keep the `nebula` dependency up to date for security fixes, and follow Nebula's security advisories.
