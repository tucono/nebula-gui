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
| The private key leaks through the API or logs | `GET /v1/config` redacts `pki.key`. Never log the config. The state directory is `0700` and the files are `0600`. |
| Ping is used as a scanning or flooding tool | Limit `count` and `timeout_ms`, rate-limit requests, and accept only IPs inside the overlay subnets. |
| An old or tampered service binary | Sign releases. The GUI checks `api_version` and shows the service version. |
| **The updater is used to install malicious code as root/SYSTEM** | The manifest is signed with a minisign key whose public half is built into the binary. The artifact's SHA-256 must match the signed manifest. Platform code signatures are checked. Downgrades are refused. The feed URL can't be set over IPC, and the install endpoint takes only a version string ([updates.md](updates.md#verification-all-must-pass-before-installing)). |
| A downloaded installer is swapped before it runs | Downloads go to the service's `0700` state directory. The hash is checked again immediately before running the installer. |
| Privacy of update checks | The check sends only a plain HTTPS request to GitHub, with no identifiers or telemetry. Automatic checks can be turned off. |

## Rules for contributors
1. New service endpoints must validate every input. Request bodies are capped (1 MiB, which is plenty for a config).
2. The service never opens a path that came from a client.
3. Secrets do not go in logs, error messages, or panics. Note that Nebula's config test (`nebula.Main` with `configTest=true`) logs the whole config, private key included, at info level. `nebulactl.Check` gives it a logger that discards everything.
4. Keep the `nebula` dependency up to date for security fixes, and follow Nebula's security advisories.
