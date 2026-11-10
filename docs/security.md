<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# Security (service branch)

The service runs with high privileges and accepts requests from local processes. It also **runs downloaded executables** and **talks to Nebula over a localhost SSH listener**. Treat the IPC API, the binary store, and the SSH console as privilege boundaries.

## Threat model

| Threat | Mitigation |
|---|---|
| A local user without permission controls the VPN | Restrict the socket or pipe to a dedicated group (Linux/macOS) or a DACL (Windows). No TCP listener for the API. |
| **The config is used to read arbitrary files as root** (`pki.key: /etc/shadow`) | Accept only inline PEM in `pki.*` (ADR-005). The service generates the runtime config, and it never contains client-supplied paths. |
| The config sets up harmful routing | Show `tun.routes` and `unsafe_routes` before connecting, and ask for confirmation of default routes and `unsafe_routes`. |
| Unexpected `listen.port` or `tun.dev` | Validate them, or limit them to an allowlist. |
| **A local user connects to the SSH debug console**, which listens on 127.0.0.1 and is reachable by any local process | Only one authorized key is allowed: the service's own ed25519 key, stored `0600` in the state directory. No password auth. A random port each start. The service pins the host key, so a local process that grabs the port first is detected. The user's own `sshd` settings are always replaced. |
| Someone fakes a Nebula process on the console port | Host-key pinning (see above). The supervisor also checks that the expected child PID is the one listening, where the OS allows it. |
| **The binary store is tampered with**, swapping `nebula` for a malicious executable that then runs as root | The state directory is owned by root/SYSTEM, and directories are `0700`. On Windows, set an explicit ACL on `%ProgramData%\nebula-gui` (SYSTEM + Administrators only). **The default ProgramData ACL lets Users create files and must not be inherited.** The hash of the active binary is checked again before every spawn. |
| **Malicious or compromised upstream release** | The `curated` channel (the default) only installs builds we've tested and whose hashes we've signed. `upstream` is opt-in with a warning. Platform code signatures are checked when present, and a missing signature that used to be present is a hard failure. The `revoked` list lets us switch users away from a bad version. |
| Downgrade or replay of an old manifest | `sequence` must only increase, and old versions aren't offered as updates. Manual rollback only allows installed versions that aren't marked bad. |
| **Executing a client-chosen binary** | There's no API to set a custom binary path. Admins set it in the service config file, which requires admin rights. |
| Archive extraction attacks (zip slip) | Extract only the expected file names into a new temporary directory, then move them into place atomically. |
| The updater is used to install malicious code (app updates) | Same as the `monolithic` branch: a signed manifest, hash, platform signature, and no downgrades. |
| The runtime config on disk leaks the private key | `run/config.yml` and the stored config are `0600` in a `0700` directory. `GET /v1/config` redacts `pki.key`. Configs are never logged. |
| Nebula log output contains sensitive data | The log buffer is only readable through the authorized IPC socket. It keeps a fixed number of entries and isn't written to disk by default. |
| Ping or `create-tunnel` used for scanning | Rate limits, and only IPs inside the overlay subnets are accepted. |
| Privacy of update checks | Only plain HTTPS GETs to GitHub, with no identifiers. Automatic checks can be turned off. |

## Rules for contributors
1. New service endpoints must validate every input. Request bodies are capped at 1 MiB.
2. The service never opens or runs a path that came from a client.
3. Secrets (the Nebula private key and the SSH client key) do not go in logs, error messages, or panics.
4. Before **every** spawn, check the binary's hash against the record in `active.json`.
5. Changes to the SSH console parsers must pass the full Nebula version matrix ([testing.md](testing.md)).
