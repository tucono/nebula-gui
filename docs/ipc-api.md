<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# GUI ↔ Service API (v1)

HTTP/1.1 with JSON bodies, over a local-only transport. No TCP listener, ever.

| Platform | Transport | Default address |
|---|---|---|
| Linux | Unix socket | `/run/nebula-gui/nebula-guid.sock` (mode `0660`, group `nebula-gui`) |
| macOS | Unix socket | `/var/run/nebula-guid.sock` (mode `0660`, group `nebula-gui`; see [platforms.md](platforms.md#macos)) |
| Windows | Named pipe | `\\.\pipe\nebula-guid` (DACL: SYSTEM, Administrators, and the configured user group) |

In development the address can be changed with `--listen` on the service and `NEBULA_GUI_SOCKET` on the GUI.

All types live in `internal/api`. Errors use this shape:

```json
{ "error": { "code": "not_configured", "message": "no config has been imported" } }
{ "error": { "code": "invalid_config", "reason": "cert_expired", "message": "nebula rejected the config: …" } }
```

`reason` is optional and refines the code (see [Error reasons](#error-reasons)). `message` is for logs and the details view, not for matching.

| Code | HTTP status | Meaning |
|---|---|---|
| `bad_request` | 400 | Malformed request: bad JSON, a body over 1 MiB, a missing field, a bad query parameter |
| `invalid_config` | 422 | The config failed validation or Nebula's dry run |
| `not_configured` | 409 | No config has been imported |
| `already_running` | 409 | Connect while Nebula is running |
| `not_running` | 409 | Disconnect while Nebula is stopped |
| `ping_failed` | 500 | The ping could not be sent |
| `internal` | 500 | Anything else, including Nebula failing to start |

### Error reasons

The service sets `reason` when it knows why something failed, so the GUI can show a specific message without parsing English. Clients must treat an unknown or missing reason like no reason: new reasons can be added without changing `api_version`.

| Reason | Code | Where | Meaning |
|---|---|---|---|
| `yaml_syntax` | `invalid_config` | `PUT /v1/config` | The config isn't valid YAML, or isn't a mapping |
| `pki_missing` | `invalid_config` | `PUT /v1/config` | The `pki` section, or `pki.ca`, `pki.cert` or `pki.key`, is missing |
| `sshd_enabled` | `invalid_config` | `PUT /v1/config` | `sshd.enabled` isn't `false` |
| `cert_expired` | `invalid_config` | `PUT /v1/config`, connect | The host cert's `not_after` has passed |
| `ca_expired` | `invalid_config` | `PUT /v1/config`, connect | Every CA in `pki.ca` has expired, or the CA that signed the host cert has (even if others in the bundle are valid) |
| `key_mismatch` | `invalid_config` | `PUT /v1/config`, connect | `pki.key` isn't the key for `pki.cert` |
| `wrong_ca` | `invalid_config` | `PUT /v1/config`, connect | `pki.cert` wasn't signed by any CA in `pki.ca` |
| `key_encrypted` | `invalid_config` | `PUT /v1/config`, connect | `pki.key` is passphrase-protected |
| `tun_unavailable` | `internal` | connect, `error_reason` | Nebula couldn't create the tun device |
| `port_in_use` | `internal` | connect, `error_reason` | Nebula's UDP port is taken |
| `nebula_stopped` | | `error_reason` | Nebula stopped on its own while running |

An `invalid_config` without a reason is any other dry-run failure, or a `pki.*` value that isn't inline PEM.

The service finds reasons in this order: its own checks of the certs (expiry, signer, encrypted key), then Nebula's error values (`errors.Is`), and only then Nebula's messages. Tests against a real Nebula pin each reason, so an upgrade that changes a message fails a test.

## Endpoints

### `GET /v1/version`
```json
{ "service_version": "0.1.0", "nebula_version": "1.12.0", "api_version": 1 }
```
The GUI calls this first. If `api_version` doesn't match, it shows an "update required" message.

### `GET /v1/status`
```json
{
  "state": "running",             // stopped | starting | running | stopping | error
  "error": "",                    // last error when state == "error"
  "error_reason": "",             // optional; see Error reasons
  "overlay_ips": ["192.168.100.3/24"],
  "cert": {
    "name": "laptop", "groups": ["users"], "not_after": "2027-01-01T00:00:00Z", "fingerprint": "…",
    "expired": false,
    "ca_not_after": "2030-01-01T00:00:00Z", // the CA that signed the cert; omitted if it isn't in pki.ca
    "ca_expired": false
  },
  "listen_port": 4242,
  "started_at": "2026-11-10T13:00:00Z",
  "configured": true
}
```

`expired` and `ca_expired` are computed on every call, so a cert that runs out after the import shows up before the user clicks Connect. The cert is described even when the stored config no longer passes the dry run, for example after a service restart with an expired cert.

### `PUT /v1/config`
Body: `{ "yaml": "<full nebula config with inline PEM>" }`

Response: `{ "restart_required": false }`

The service rejects the config with `invalid_config` if:
- it doesn't parse;
- `pki.ca`, `pki.cert` or `pki.key` is missing, or is a file path instead of inline PEM;
- `sshd.enabled` is anything other than `false`;
- the service's own cert checks fail (see [Error reasons](#error-reasons));
- `nebula.Main` fails the dry run (`configTest=true`).

A rejected config is not stored. If Nebula is running, `restart_required` is `true`: the new config is stored, but the service does **not** restart on its own. Until the next connect, `/v1/status` keeps describing the running config.

### `GET /v1/config`
Response: `{ "yaml": "…" }`, the stored YAML with `pki.key`, and any other value containing a private key, replaced by `<redacted>`. Returns `not_configured` if there is none.

### `DELETE /v1/config`
Stops Nebula if it is running, then deletes the stored config. Returns the new `status`.

### `POST /v1/connect` / `POST /v1/disconnect`
No body. Returns the new `status`.

- Connect: `not_configured` without a config, `already_running` if running. The cert checks from `PUT /v1/config` run again first: if the cert or CA has expired since the import, the response is `invalid_config` with the reason, Nebula isn't started, and the state stays `stopped`. If Nebula fails to start (for example, no permission to create the tun device), the response is `internal` with Nebula's error and a reason if known, and the state becomes `error` with the same `error_reason`.
- Disconnect: `not_running` if stopped. From `error`, disconnect clears the error and returns to `stopped`.
- If Nebula stops on its own, the state becomes `error` with Nebula's error in `error` and `error_reason` `nebula_stopped`. Connect starts it again.
- Requests that change state are serialized. A connect sent while another is starting waits for it, then returns `already_running`.

### `GET /v1/peers`
```json
{
  "peers": [
    {
      "vpn_ips": ["192.168.100.5"],
      "name": "desktop",
      "groups": ["users"],
      "state": "established",       // established | pending
      "current_remote": "203.0.113.7:4242",
      "remote_addrs": ["203.0.113.7:4242", "10.0.0.5:4242"],
      "relayed": false,
      "cert_not_after": "2027-01-01T00:00:00Z",
      "last_ping_rtt_ms": 23.4      // null if this peer has never been pinged
    }
  ]
}
```
The data comes from `Control.ListHostmapHosts`. Nebula has no per-peer byte or latency counters, so `last_ping_rtt_ms` is the only latency figure.

### `POST /v1/peers/{vpn_ip}/close`
Closes the tunnel to that peer (`Control.CloseTunnel`).

### `POST /v1/ping`
Request: `{ "vpn_ip": "192.168.100.5", "count": 3, "timeout_ms": 5000 }`
`count` is 1–10 and `timeout_ms` is at most 15000. The service rejects IPs outside the overlay network's subnets.

Response:
```json
{ "vpn_ip": "192.168.100.5", "sent": 3, "received": 3, "rtt_min_ms": 21.0, "rtt_avg_ms": 23.4, "rtt_max_ms": 27.9 }
```
The first ping often times out while the handshake runs, so the GUI should present it as "connecting…" rather than "failed".

### `GET /v1/logs?since=<seq>`
```json
{ "next": 1042, "lines": [ { "seq": 1041, "time": "…", "level": "info", "msg": "Handshake message sent", "fields": { … } } ] }
```
Ring buffer of the last 2000 entries at info level and above, from the service and from Nebula (Nebula's lines have `"component": "nebula"` in `fields`). Returns lines with `seq >= since`, oldest first. Pass `next` as `since` on the next call to get only new lines. If `since` is older than the buffer, the response starts at the oldest line kept.

## Updates

See [updates.md](updates.md) for how update checks and installs work.

### `GET /v1/update`
```json
{
  "current":   { "version": "0.3.0", "nebula_version": "1.9.3" },
  "available": { "version": "0.4.0", "nebula_version": "1.10.0", "security": false,
                 "notes_url": "…", "released_at": "…" },          // null if up to date
  "upstream_nebula": { "version": "1.10.1", "url": "…" },          // null if we embed the latest
  "state": "idle",            // idle | checking | downloading | verifying | ready | installing | error
  "progress": 0.0,            // 0–1 while downloading
  "last_checked": "2026-11-10T13:00:00Z",
  "error": "",
  "self_update_supported": true,   // false for package-manager installs on Linux
  "settings": { "auto_check": true, "install_when_disconnected": false, "skipped_version": "" }
}
```

### `POST /v1/update/check`
Runs a check now and returns the result as `GET /v1/update` does. Rate-limited to one check per 60 s.

### `POST /v1/update/install`
Request: `{ "version": "0.4.0", "when": "now" }`. `when` is `now` or `disconnected`.

The `version` must match the update the service already found and verified. **Clients can't provide URLs, file paths, or hashes.** The response is `202 Accepted`. The GUI then polls `GET /v1/update`, and during the install the service restarts.

### `PUT /v1/update/settings`
Request: `{ "auto_check": true, "install_when_disconnected": false, "skipped_version": "0.4.0" }`

Error codes for these endpoints: `update_unavailable`, `update_not_supported`, `update_verification_failed`, `update_in_progress`.

## Versioning
- Breaking changes increase the path prefix (`/v2`) and `api_version`.
- New fields can be added at any time. Clients must ignore fields they don't recognize.
