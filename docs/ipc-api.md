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
```

Error codes: `bad_request`, `invalid_config`, `not_configured`, `already_running`, `not_running`, `ping_failed`, `internal`.

## Endpoints

### `GET /v1/version`
```json
{ "service_version": "0.1.0", "nebula_version": "1.9.x", "api_version": 1 }
```
The GUI calls this first. If `api_version` doesn't match, it shows an "update required" message.

### `GET /v1/status`
```json
{
  "state": "running",             // stopped | starting | running | stopping | error
  "error": "",                    // last error when state == "error"
  "overlay_ips": ["192.168.100.3/24"],
  "cert": { "name": "laptop", "groups": ["users"], "not_after": "2027-01-01T00:00:00Z", "fingerprint": "…" },
  "listen_port": 4242,
  "started_at": "2026-11-10T13:00:00Z",
  "configured": true
}
```

### `PUT /v1/config`
Body: `{ "yaml": "<full nebula config with inline PEM>" }`

The service rejects the config with `invalid_config` if:
- it doesn't parse;
- any `pki.*` value is a file path instead of inline PEM;
- `nebula.Main` fails the dry run (`configTest=true`).

If Nebula is running, the response includes `"restart_required": true`. The service does **not** restart on its own.

### `GET /v1/config`
Returns the stored YAML with `pki.key` **redacted**.

### `DELETE /v1/config`
Stops Nebula if it is running, then deletes the stored config.

### `POST /v1/connect` / `POST /v1/disconnect`
No body. Returns the new `status`. Connecting while already connected returns `already_running`.

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
Ring buffer of the last ~2000 entries.

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
