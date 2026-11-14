<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# M2 plan: GUI basics

Detailed plan for the [M2 roadmap items](roadmap.md#m2-gui-basics).

## Who this is for

The people using the app are friends and family with little technical experience. They received a config from the person who runs the network, and they want it to work. So for every error:

- say what happened in plain words;
- say what to do next;
- when the app can fix the problem itself, offer a button that does it;
- when it can't, make it easy to get help, from the person who runs the network or from an AI assistant.

See [Friendly errors](#friendly-errors) and [Getting help](#getting-help).

## What M2 delivers

At the end of M2, one window lets the user do all of this without `curl`:

- import a `config.yml` whose `pki.*` entries are file paths
- connect and disconnect
- see the state, overlay IP and certificate
- read the logs
- understand every error they hit, and fix it directly where possible
- create a troubleshooting report that an AI assistant or a person can read, with private network details masked

**Left for later:** the tray and hide-to-tray (M4), peers and ping (M3), routes and cert-expiry warnings before they happen (M5). Fixes that need an installed service, such as "Start the service" and "Give me permission", come with the installers in M4 (see [Fixes that wait for M4](#fixes-that-wait-for-m4)).

## Starting point

- `internal/api.Client` already has the basic M2 calls: `Status`, `PutConfig`, `Config`, `DeleteConfig`, `Connect`, `Disconnect` and `Logs`.
- `internal/gui` has a single screen: a version check (`gui.go`) plus `describeVersion`, a function with no Fyne dependency that has its own unit test. M2 keeps that split: logic goes in plain functions that can be tested, and the Fyne code stays a thin layer on top.
- The service's errors are correct but technical (`internal: Failed to get a tun/tap device`). Most of Nebula's startup errors are plain strings, so the GUI can't reliably tell them apart. The service can: it has the error values (`cert.ErrPublicPrivateKeyMismatch` and the like) and the parsed certificate.
- The service logs to stderr and to an in-memory ring buffer (`GET /v1/logs`, info and above). Both are lost when the service restarts. The GUI keeps no log at all.

M2 therefore makes two backward-compatible changes to the service: error reasons (step 1) and a persistent log with a diagnostics endpoint (step 2).

## Friendly errors

### Rules for messages

Every problem the GUI shows has the same parts:

| Part | Content | Example |
|---|---|---|
| **Title** | What happened, in plain words. | "Your network certificate has expired" |
| **Explanation** | Why, in one or two sentences, without blaming the user. | "Certificates are only valid for a limited time. Yours ran out on 3 March 2027." |
| **What to do** | Numbered steps or a single sentence. | "Ask the person who set up your network for a new config." |
| **Fix button** | Only when the app can fix it directly. | "Choose the file…" |
| **Details** | Collapsed by default: the error code, the raw message, and the app and service versions. | `invalid_config / cert_expired: nebula certificate for this host is expired` |
| **Get help…** | Opens the [troubleshooting report](#getting-help) with this problem at the top. | |

Wording:

- No jargon in the title, explanation or steps: no "socket", "pipe", "PEM", "YAML", "tun", "API", "IPC", "daemon" or "HTTP". Say "background service", "network adapter", "certificate", "key file" and "config file" instead. Technical terms only appear under Details and in the report.
- Use the names the user knows: the config's file name, and the file name from the config, such as `laptop.key`.
- "The person who set up your network" is the fallback contact. Nothing in the code or docs names a real person.

### Error catalogue

Every row has a unit test (see step 3). "Fix button" means the app does the fix itself. Every problem also has **Get help…**, so the tables don't repeat it.

**Reaching the service**

| Situation | Title | What to do / fix button |
|---|---|---|
| Socket or pipe missing, connection refused | "The background service isn't running" | "The Nebula service runs in the background and needs to be running for this app to work. Restart your computer. If this keeps happening, reinstall the app." **Try again** button. The app also retries on its own and the message disappears once the service answers. |
| Permission denied on the socket or pipe | "You don't have permission to use Nebula on this computer" | Linux/macOS: "Your user account needs to be added to the `nebula-gui` group. Ask whoever manages this computer, or run this command, then log out and back in:" followed by the command with a **Copy** button. Windows: "Reinstall the app and choose your user account during setup." |
| No answer within the timeout | "The background service isn't responding" | **Try again**. "If this keeps happening, restart your computer." |
| Service API older than the app | "The background service needs an update" | "Install the latest version of the app; it updates the service too." **Open download page** button (the public GitHub releases page). |
| Service API newer than the app | "This app needs an update" | Same button. |

**Importing a config**

| Situation | Title | What to do / fix button |
|---|---|---|
| No config yet | "Let's get you connected" (not an error, an empty state) | "You should have received a config file, usually called `config.yml`, from the person who set up your network." Big **Import config…** button. |
| The picked file isn't a config (doesn't parse, isn't a mapping, or has no `pki` key) | "That doesn't look like a Nebula config file" | "You picked `notes.txt`. Pick the config file you received, usually called `config.yml`." **Choose another file…** |
| A file the config refers to is missing | "A file is missing: `laptop.key`" | "Your config expects `laptop.key` at a place that doesn't exist on this computer. Find the `laptop.key` you were given." **Choose the file…** opens a picker in the user's home folder so the user can point to it. The GUI then inlines that file instead. |
| A referenced file can't be read | "Can't open `laptop.key`" | "The file exists but can't be opened. Copy it somewhere in your home folder and try again." **Choose the file…** |
| A referenced file has the wrong contents | "`laptop.key` isn't a key file" | "It may have been damaged while copying. Ask for the files again." |
| `pki` section missing | "This config is incomplete" | "It doesn't say which certificate to use. Ask the person who set up your network for a new config." |
| Config turns on Nebula's SSH console (`sshd_enabled`) | "This config turns on a remote admin console" | "This app doesn't allow it, for safety." **Turn it off and import** button: the GUI sets `sshd.enabled: false` in its copy and sends it again. |
| Host certificate expired (`cert_expired`) | "Your network certificate has expired" | "It ran out on 3 March 2027. Ask the person who set up your network for a new config." Also shown as a banner as soon as the status reports `cert.expired`, and when Connect fails with this reason, so a cert that expires after import never shows as "Couldn't connect". |
| CA expired (`ca_expired`) | "Your network's certificate authority has expired" | Same steps, and also shown from the status (`cert.ca_expired`) and on Connect. |
| Key doesn't match certificate (`key_mismatch`) | "Your key file doesn't match your certificate" | "The config may be mixing files from two computers. Make sure `laptop.key` and `laptop.crt` came together, or ask for a new config." |
| Certificate not signed by the CA (`wrong_ca`) | "Your certificate belongs to a different network" | "Ask the person who set up your network for a new config." |
| Encrypted key (`key_encrypted`) | "Your key file is password-protected" | "This app can't use password-protected keys. Ask for an unprotected key." |
| Config too large | "This config file is too large" | "Config files are usually a few kilobytes. Make sure you picked the right file." |
| Anything else from the dry run (`invalid_config` with no reason) | "This config file has a problem" | "Use **Get help…** and send the report to the person who set up your network." |

**Connecting**

| Situation | Title | What to do / fix button |
|---|---|---|
| Nebula can't create the network adapter (`tun_unavailable`) | "Couldn't create the network adapter" | "The background service doesn't have the permission it needs. Reinstall the app." On Windows: "…the Wintun driver may be missing." |
| Port in use (`port_in_use`) | "Another program is using Nebula's port" | "Usually this means Nebula is already running on this computer outside this app. Close it, then click Connect." |
| Nebula stopped by itself (`nebula_stopped`) | "The connection stopped unexpectedly" | **Connect again** button. Details show Nebula's message. |
| New config saved while connected | "New config saved" (info, not an error) | **Reconnect to use it** button: `Disconnect`, then `Connect`. |
| Any other connect failure | "Couldn't connect" | **Try again**. |

### Fixes that wait for M4

Two fixes need an installed service, which doesn't exist before the M4 installers. M2 shows the instructions from the table above, and M4 adds the buttons:

- **Start the service:** `pkexec systemctl start nebula-guid` on Linux, a UAC prompt to start the Windows Service, and `launchctl kickstart` through an admin prompt on macOS.
- **Give me permission:** `pkexec usermod -aG nebula-gui $USER` on Linux, then a "Log out to finish" message. The M4 installers should also add the installing user to the group (or DACL) themselves, so this rarely comes up.

## Getting help

When the app can't fix a problem, the user should be able to hand everything needed to someone, or something, that can. **Get help…** creates a single troubleshooting report, written so that an AI assistant can work with it without asking the user follow-up questions they can't answer.

### Report format

Markdown with a fixed structure, so it can be pasted into any chat:

```
# Nebula troubleshooting report
Generated 2026-11-12T14:03:22+01:00 · nebula-gui 0.2.0 · service 0.2.0 · Nebula 1.12.0 · Linux amd64 (Ubuntu 24.04)
Masking: on

## Problem
Couldn't connect (internal / tun_unavailable)
Nebula: "Failed to get a tun/tap device: operation not permitted"

## State
state=error · configured=yes · overlay OVERLAY-1/24
cert HOST-1, groups GROUP-1, valid 2026-03-03 → 2027-03-03 · listen port 4242
Config: 1 lighthouse (PUBLIC-1:4242), am_lighthouse=false, relays off, punchy on,
0 tun.routes, 0 unsafe_routes, firewall 3 inbound / 1 outbound rules

## Recent app events
2026-11-12T14:03:20.002+01:00 connect clicked
2026-11-12T14:03:20.310+01:00 connect failed: internal / tun_unavailable

## Service log (last 300 lines, JSON Lines)
{"time":"2026-11-12T14:03:20.114+01:00","level":"ERROR","msg":"…","component":"nebula",…}

## Notes for the assistant
- This is a Nebula overlay network client. nebula-guid is a privileged background
  service that runs Nebula; nebula-gui is the desktop app the user sees.
- The user is not technical. Suggest one step at a time and say where to click.
- Names and addresses are masked consistently: PUBLIC-1 is always the same address.
- Only the person who runs the network can issue new certificates or configs.
```

Contents:

- **Header:** generation time with time zone, all versions, OS and architecture, and whether masking is on.
- **Problem:** the problem currently shown, with code, reason and raw message. Leave it out when the user opens the report without a problem.
- **State:** `GET /v1/status` and the config summary from `GET /v1/diagnostics` (step 2). It's a summary of counts and flags, never the config itself.
- **Recent app events:** the last 100 entries of the GUI log (step 9).
- **Service log:** the last 300 lines from the persistent log, as JSON Lines with full timestamps, so nothing gets lost in formatting.
- **Notes for the assistant:** fixed text that explains the architecture and how to talk to the user. When the service can't be reached, it adds: "The service log isn't available. On Linux, `journalctl -u nebula-guid -n 200` shows it." The assistant can then guide the user through that.

The report is capped at about 50 KB, so it fits in a chat message, including on free AI chat tiers. To fit more useful lines in that space, certificate objects in log lines (Nebula logs the full cert on handshakes) are shortened to one string, for example `"cert":"HOST-2 [GROUP-1] OVERLAY-3/24 until 2027-03-03 by FP-1"`. Public keys and signatures are dropped. If the report is still too big, the oldest log lines are dropped first, and the report says how many. The persistent log file itself is not changed.

### Masking

Masking is **on by default**, because the main use is pasting into a public AI chat. It replaces private network details with consistent placeholders, so the report still shows which lines refer to the same host:

| Data | Becomes | Kept as is |
|---|---|---|
| Public IPs (v4 and v6), with ports | `PUBLIC-1`, `PUBLIC-1:4242` | |
| LAN underlay IPs (RFC 1918 or ULA, outside the overlay) | `LAN-1` | |
| Overlay IPs and the overlay network | `OVERLAY-1`, with the prefix length (`/24`) kept | |
| Host names, cert names, lighthouse DNS names | `HOST-1` | |
| Cert groups | `GROUP-1` | |
| Cert fingerprints | `FP-1` | |
| IPv6 link-local (`fe80::/10`), which can contain an ID derived from the hardware address | `LINKLOCAL-1` | |
| The user's home directory and user name | `~`, `USER` | |
| | | Loopback, `0.0.0.0`, IPv4 link-local (`169.254.x`), ports, timestamps, levels, error codes, Nebula's messages |

How masking works:

- **Collecting names.** This is automatic; the user never adds anything. Names come from:
  - the status (cert name, groups, overlay network);
  - the config summary (lighthouse DNS names, static host map names);
  - log fields with known keys (`certName`, `vpnAddrs`, `udpAddr`, and so on), which give the names of other hosts;
  - the names the GUI has remembered (see below).
- **Known fields** are replaced exactly, by value.
- **Free text** is replaced more carefully, so Nebula's own words aren't damaged (a host called `dev` must not turn `/dev/net/tun` into `/HOST-1/net/tun`):
  - only whole words, meaning no letter, digit, `/`, `_`, `.` or `-` on either side;
  - only names of at least 4 characters;
  - never words on a built-in stop list of Nebula and system vocabulary (`lighthouse`, `dev`, `user`, `users`, `admin`, `host`, `nebula`, …).

  The list of known field keys and the stop list live in the code. New Nebula log fields that carry names are added to the list of known keys.
- **Addresses.** After the names, a regex pass finds candidate IPv4 and IPv6 addresses. Each match is checked with `netip.ParseAddr` (so times like `14:03:22` don't count) and classified by range. An address is an overlay address if it falls inside the current or remembered overlay network. That check comes before the LAN ranges, because the overlay is often in RFC 1918 space itself. `100.64.0.0/10` (CGNAT) is treated as LAN.
- **Consistency.** One masker is used for the whole report, so `PUBLIC-1` means the same address in every section.
- **Remembered names.** At import and on each status, the GUI saves the cert name, groups, overlay network, lighthouse names and static host names in its settings (user-only permissions). They're used only for masking, so a report made while the service can't be reached still masks the names in the GUI log. They're cleared when the config is removed.
- A checkbox, **Include network details**, turns masking off for reports sent to the person who runs the network. The help text says: "Only turn this on when sending the report to the person who set up your network."

Never in a report, masked or not:

- private keys, certificates or the config itself;
- anything containing `PRIVATE KEY` or `BEGIN NEBULA`. Every input line (log line or GUI event) that contains either string is dropped, and the report says "N lines removed for safety". As a backstop, a final check on the finished text refuses to produce the report if either string is still there, which would mean a bug elsewhere.

### Where Get help… appears

- In every problem view, next to the details.
- At the top of the Logs tab.
- In the window's menu (Help → Get help…), for when something feels wrong without an error being shown.

The **Get help** dialog shows:

- the short explanation: "This report helps someone figure out what's wrong. It's safe to paste into an AI assistant like Claude: personal network details are hidden."
- a read-only preview of the report;
- the **Include network details** checkbox;
- **Copy** and **Save as file…** buttons.

## Steps

Listed in build order. Each one is small enough to commit separately.

### 1. Service: machine-readable error reasons

The service already knows *why* something failed; it should say so in a form the GUI can match without parsing English.

- **API (additive, `api_version` stays 1):**
  - Error body gets an optional `reason`: `{"error": {"code": "invalid_config", "reason": "cert_expired", "message": "…"}}`.
  - `StatusResponse` gets an optional `error_reason` next to `error`.
  - `CertInfo` gets `expired`, `ca_not_after` (the CA that signed the host cert) and `ca_expired`, computed on every status call, so the GUI can show an expired cert before the user clicks Connect.
  - Clients must treat an unknown or missing reason like no reason. Old GUIs ignore the new fields.
- **Reasons:**
  - for `invalid_config`: `yaml_syntax`, `pki_missing`, `sshd_enabled`, `cert_expired`, `ca_expired`, `key_mismatch`, `wrong_ca`, `key_encrypted`
  - for connect failures: `cert_expired` and `ca_expired` (as `invalid_config`, see below), and `tun_unavailable`, `port_in_use`, `nebula_stopped` (as `internal`, and in `state=error`)
- **Where reasons come from:** `nebulactl` is the only package that imports Nebula, so it turns errors into reasons:

  ```go
  package nebulactl
  type Reason string // ReasonCertExpired = "cert_expired", …
  type Error struct { Reason Reason; Err error }
  func (e *Error) Unwrap() error { return e.Err }
  ```

  - `Check` and `Start` return `*nebulactl.Error` for failures they can classify.
  - The service reads the reason with `errors.As`. `errInvalidConfig` and `errStart` get a `Reason` field and keep the error chain with `Unwrap`; today `putConfig` flattens Nebula's error into a string, which breaks `errors.Is`.
  - The fake engine returns the same type, so service tests can produce any reason.
- **How `nebulactl` finds reasons**, most reliable first:
  1. Its own checks before the dry run, and again before `Start`. It already parses the host cert (`nebulactl.ParseCert`). It also parses the CA bundle and compares `NotAfter` with the current time. This finds expiry without depending on Nebula's wording; Nebula's expired-host-cert error is a plain `fmt.Errorf`, not `cert.ErrExpired`.
     - `cert_expired`: the host cert's `NotAfter` has passed.
     - `ca_expired`: every CA in `pki.ca` has expired (Nebula's own rule), **or** the CA that signed the host cert has expired, even if others in the bundle are still valid. A bundle with an old and a new CA during a rotation is fine as long as the signer is valid.
     - `connect()` runs these checks before `Start` and returns `invalid_config` with the reason without starting Nebula. A cert that was valid at import but has expired since then doesn't show up as a generic `internal` error.
  2. `errors.Is` against Nebula's sentinel errors (`cert.ErrPublicPrivateKeyMismatch`, `cert.ErrCaNotFound`, `cert.ErrSignatureMismatch`, `cert.ErrPrivateKeyEncrypted`) and `syscall.EADDRINUSE`, unwrapping `util.ContextualError` where needed.
  3. As a last resort, substring matches on Nebula's messages ("Failed to get a tun/tap device", "failed to initialize interface").
- **Tests:** real-Nebula tests that pin every reason. Extend `nebulatest` to create an expired cert, an expired CA, a bundle with one expired and one valid CA (both as signer and as non-signer), a mismatched key and a cert from another CA. Then check that `Check`/`Start` give the expected reason. A service test with the fake engine checks that a cert that expires after import gives `cert_expired` on Connect and `cert.expired` in the status. When a Nebula upgrade changes a message, these tests fail instead of the GUI quietly falling back to the generic message.
- **Docs:** `docs/ipc-api.md`, with the reason table.

### 2. Service: persistent log and diagnostics endpoint

- **Log file:** `<state dir>/logs/nebula-guid.jsonl`, in the existing state directory (`/var/lib/nebula-gui/`, `/Library/Application Support/nebula-gui/`, `%ProgramData%\nebula-gui\`).
  - Directory `0700`, files `0600`. On Windows, the same ACL as the config store (`store_windows.go`).
  - `slog.NewJSONHandler` at **debug** level, added as a third handler to the existing multi-handler. Stderr and the ring buffer stay as they are (the ring buffer remains info and above).
  - The debug `request` line in `Server.Handler` is skipped for successful `GET` requests (`/v1/status`, `/v1/logs`, `/v1/version`, `/v1/diagnostics`, `/v1/config`). The GUI polls these every few seconds, and otherwise they would fill most of the file and push out Nebula's lines. Requests that change something, and every failed request, are still logged.
  - A small rotating writer, with no new dependency: rotate at 2 MiB and keep `nebula-guid.jsonl`, `.1` and `.2`, so at most about 6 MiB. Each record is written straight away, so the lines just before a crash are kept.
  - On startup, the service logs a `service started` line with all versions, OS, architecture and PID, so restarts are visible in the log.
  - If the file can't be opened, the service logs a warning to stderr and keeps running without it. A broken log must never stop the VPN.
- **`GET /v1/diagnostics?lines=N`** (default 300, max 2000):

  ```json
  {
    "service_version": "0.2.0", "nebula_version": "1.12.0",
    "os": "linux", "arch": "amd64", "os_version": "Ubuntu 24.04",
    "status": { …same as GET /v1/status… },
    "config_summary": {
      "lighthouses": ["203.0.113.7:4242"], "am_lighthouse": false,
      "relays": false, "punchy": true, "listen_port": 4242,
      "tun_routes": 0, "unsafe_routes": 0,
      "firewall_inbound": 3, "firewall_outbound": 1
    },
    "log": [ "{\"time\":…}", … ],
    "log_truncated": false
  }
  ```

  - `log` holds the last `N` lines of the log file as raw JSON strings, read across rotated files.
  - `config_summary` is computed from the stored config. It contains counts, flags and lighthouse addresses only, never certificates or keys. `null` when there's no config. `lighthouse.hosts` lists overlay IPs, so the underlay addresses are looked up in `static_host_map`, and they may be DNS names (`lighthouse.example.net:4242`). The summary also lists the static host map's names, for masking.
  - `os_version` needs code for each platform: `PRETTY_NAME` from `/etc/os-release`, `sw_vers` on macOS, and the registry's `ProductName` and `DisplayVersion` on Windows. Empty if unknown.
- **Instance ID:** at startup the service creates a random `instance_id` and adds it to `VersionResponse`, `StatusResponse`, `LogsResponse` and `DiagnosticsResponse` (additive). When it changes, the GUI knows the service has restarted: it checks the API version again and resets its log buffer (steps 5 and 8). The log's sequence numbers and a check made only on reconnect can't detect this reliably.
  - The data is **not masked**. Masking happens in the GUI, so the same data can produce both versions of the report. The endpoint is behind the same socket permissions as every other endpoint.
- **API types and client:** `api.DiagnosticsResponse`, `Client.Diagnostics(ctx, lines)`, and `InstanceID` on the existing responses.
- **Tests:**
  - polling GETs don't appear in the log file, and a failed GET and a Connect do;
  - `instance_id` is the same across calls and differs between two `Server` instances;
  - rotation: writing past 2 MiB produces `.1`, and `.2` is dropped after the next rotation;
  - reading the last N lines across rotated files, including a partial last line;
  - a real-Nebula start and stop, then a scan of the log file for `PRIVATE KEY` and `BEGIN NEBULA`, which must find nothing;
  - `config_summary` for the lab config.
- **Docs:** `docs/ipc-api.md` (endpoint) and `docs/security.md` (what the log file contains, who can read it, and that keys never go into it).

### 3. Problem catalogue and service connection errors

**New files:**

- `internal/gui/problems.go`: `type problem struct { Title, Explain string; Steps []string; Fix *fixAction; Details string }` and one constructor per row of the [catalogue](#error-catalogue).
- `internal/gui/connerr.go`, plus build-tagged `connerr_unix.go` and `connerr_windows.go`: turns a transport error into the right problem.

| Kind | Unix | Windows |
|---|---|---|
| Not running | `ENOENT`, `ECONNREFUSED` | `ERROR_FILE_NOT_FOUND` |
| No permission | `EACCES` | `ERROR_ACCESS_DENIED` |
| Timeout | context deadline | same |
| Incompatible | API version mismatch (replaces the current `describeVersion` messages) | |
| Service error | `*api.Error`, chosen by `code` and `reason` | |

- `golang.org/x/sys` is already a direct dependency in `go.mod`, so the Windows side needs no new module.
- **Tests:**
  - every catalogue entry has a title, an explanation and at least one step;
  - a "no jargon" test fails if a title, explanation or step contains a banned word from the [rules](#rules-for-messages);
  - a table test that wraps real `syscall` errnos the same way `net.OpError` does. On Linux, a test with a socket file set to mode `0000` gives a real `EACCES`.

### 4. Problem view

**New file:** `internal/gui/problem_view.go`: one reusable widget that shows a `problem`: title in bold, explanation, numbered steps, an optional fix button, a collapsed **Details** section, and **Get help…**.

- Problems about the service or the connection appear as a banner at the top of the window, replacing the status header while they last.
- Problems about an import appear in a dialog, because they're a reply to something the user just did.

### 5. Poller and status model

**New files:** `internal/gui/poller.go` and `status_view.go`.

- **Poller:** one goroutine.
  - On (re)connect it calls `Version` once and checks the API version.
  - After that it calls `Status` every 2 s. If `instance_id` changes, the service has restarted (for example after an upgrade), so it calls `Version` again and records the restart in the GUI log.
  - While the service is unreachable it backs off: 2 → 4 → 8 → 10 s. The banner's **Try again** button polls immediately. When the service answers again, the banner disappears on its own.
  - It sends UI updates through `fyne.Do`, as the current code does.
  - It records changes in reachability and state in the GUI log (step 9), not every poll.
- **`describeStatus(*api.StatusResponse, error) statusView`:** a pure function, tested like `describeVersion`. It produces:
  - the headline in plain words: "Connected", "Not connected", "Connecting…", "Disconnecting…", or the problem title when `state=error`
  - overlay IPs, cert name, cert expiry date as a date ("valid until 3 March 2027"), and how long it's been connected
  - which buttons are enabled
- **Button rules:**

  | State | Main button |
  |---|---|
  | `stopped` | Connect |
  | `running` | Disconnect |
  | `starting`, `stopping` | disabled, with a spinner |
  | `error` | the problem's fix (usually **Connect again**), plus "Dismiss", which calls `Disconnect` to clear the error |
  | `configured=false` | the "Let's get you connected" empty state with **Import config…** |

### 6. Status header and Connect/Disconnect

**New file:** `internal/gui/main_window.go`, which replaces most of `Run`.

- **Layout:** a status header (or a problem banner) at the top, then an `AppTabs` with **Status** and **Logs** (M3 adds a **Peers** tab), a footer with the versions (the current version form, made smaller), and a Help menu with **Get help…**. The tabs arrive with the Logs tab (step 8) and the Help menu with **Get help…** (step 9), so neither shows up empty before then.
- **Connect and Disconnect** run off the UI goroutine, and the button stays disabled until they return. The response is already a `StatusResponse`, so it updates the view right away without waiting for the next poll.
- **Timeouts:** 5 s for reads (`Version`, `Status`, `Logs`, `Diagnostics`, `Config`) and 30 s for actions (`Connect`, `Disconnect`, `PutConfig`, `DeleteConfig`). `Disconnect` can take more than 10 s while Nebula's packet readers exit (`nebulactl.stopTimeout`), and a slow but successful action must not show "isn't responding". Status polling keeps running during an action, so the header can show "Disconnecting…".
- **Connect failures** become a problem banner using the reason from step 1, for example "Couldn't create the network adapter", with the raw `internal: Failed to get a tun/tap device` under Details.

### 7. Config import with PEM inlining

**New files:**

- `internal/gui/inline.go`: pure, no Fyne.
- `internal/gui/import.go`: the dialogs.

**Inlining function:**

```go
func inlinePKI(raw []byte, baseDir string, read func(string) ([]byte, error)) (string, error)
```

- It works on a `yaml.Node`, using `go.yaml.in/yaml/v3`, which is already in `go.mod`. That keeps comments and key order.
- Before inlining, it checks that the file is a config at all: it parses as YAML, the top level is a mapping, and it has a `pki` key.
- For each of `pki.ca`, `pki.cert` and `pki.key`:
  - A value that already starts with `-----BEGIN` is left as is.
  - Anything else is treated as a path, used as written. A relative path resolves against the config file's directory, and `~/` expands. There's no guessing: if the path doesn't exist, for example `/etc/nebula/host.key` from the admin's machine, the import asks the user for the file.
  - It reads the file with a 1 MiB cap and checks that the contents decode as PEM. The node becomes a literal block scalar (`|`).
- Errors are typed (`errFileMissing{Key, Path}`, `errFileUnreadable`, `errNotPEM`, `errNotConfig`, `errTooLarge`), so the dialog can pick the right catalogue entry and offer **Choose the file…** with the key it's for.
- If the result is bigger than the service's 1 MiB body limit, it refuses before sending.
- `read` is a parameter, so tests can use a map-backed fake. There's also a real temp-dir test covering relative paths.
- **Tests:**
  - already inline
  - absolute and relative paths
  - `~` expansion
  - a missing file, and replacing it with a picked file
  - a non-PEM file
  - a `.txt` that isn't a config
  - a CA bundle (several PEM blocks)
  - other keys and comments preserved
  - `pki` missing entirely
  - `sshd.enabled: true` switched off by the fix, with the rest of the document unchanged

**Import flow:**

1. **Import config…** opens a file picker filtered to `.yml`, `.yaml`, `.conf` and `.txt`. Files with no extension aren't offered; a well-formed config has one of these. The directory of the picked path is `baseDir`. On Linux the picker is the desktop's own, through the XDG Desktop Portal (`picker.go`, `portal_linux.go`); where there's no portal, and on Windows and macOS until M4, it's Fyne's `dialog.NewFileOpen`, sized to fill the window.
2. The GUI inlines the PKI. A missing or broken file shows its problem dialog, and **Choose the file…** opens a picker that starts in the user's home folder, with no filter. After the user picks the file, the import continues.
3. The GUI sends the result with `PutConfig`. An `invalid_config` answer shows the problem for its `reason`. For `sshd_enabled`, **Turn it off and import** changes the GUI's copy and sends it again.
4. On success: "Config imported. You're ready to connect." If the response has `restart_required: true`, the header shows "New config saved" with **Reconnect to use it**, which calls `Disconnect` and then `Connect`.
5. Each step is recorded in the GUI log: the file name, not its contents, and the result.

**Remove config (optional):** a small extra item, with a confirmation dialog ("You'll need the config file again to reconnect"), that calls `DeleteConfig`. The API already exists, and it makes testing repeat imports easier.

### 8. Log view

**New files:** `internal/gui/logs.go` and `log_view.go`.

- **Model:** a client-side ring buffer of 2000 lines, the same size as the service's.
  - It polls `Logs(since)` every 2 s while the Logs tab is visible. Otherwise it polls every 10 s, so the buffer stays current.
  - **Service restart:** when `resp.InstanceID` differs from the last one, the GUI clears the buffer and fetches again from 0. A check on `resp.Next < since` alone would miss a restart once the new service has logged more lines than the GUI had seen.
- **`formatLogLine(api.LogLine) string`** produces `15:04:05 INFO  msg key=val …` for display only, with fields sorted and the `component` field turned into a `[nebula]` prefix. It's a pure function with a unit test. The report uses the full JSON lines from the persistent log, not this format.
- **Widget:** a `widget.List` with monospace labels, so it scales to 2000 rows.
  - A short line at the top: "These are technical details for troubleshooting. If something isn't working, use **Get help…**."
  - A "Follow" checkbox scrolls to the bottom when new lines arrive. Fyne doesn't make it reliable to detect that the user scrolled away, so the checkbox makes that choice explicit.
  - **Get help…** replaces the earlier "Copy all" idea: the report contains the same lines, plus context and masking.
  - Optional: a level filter (warn+ / all).
  - Rows are cut off at the window's edge, so selecting one shows the whole line, selectable, in a pane of fixed height below the list.
  - The window gets two tabs, **Status** and **Logs**. The Status tab keeps its scroll container, and the Logs tab's list scrolls by itself.

### 9. GUI log, troubleshooting report and masking

**GUI log** (`internal/gui/applog.go`). This is built together with the poller (commit 4), so every later step records its events from the start:

- Records what the user did and what happened: service reachability changes, version check results, imports (file name and result), problems shown (code and reason), and Connect/Disconnect with their results.
- Kept in memory (last 500 entries) and in a JSON Lines file under `os.UserConfigDir()/nebula-gui/logs/`, rotated at 1 MiB with one old file kept. So it survives a GUI restart, and covers the case where the service was never reachable.
- Never records config contents, PEM, or full paths outside the file name. Home-directory paths are written as `~/…`.

**Report** (`internal/gui/report.go`): `buildReport(reportInput) string`, a pure function. Its input is the current problem, the status, the `GET /v1/diagnostics` response (or the error from trying to get it), the GUI log, the versions and OS, and the masking setting. It produces the [format above](#report-format).

**Masking** (`internal/gui/mask.go`): a `masker` with consistent placeholders, as described under [Masking](#masking). It's created once per report.

**Dialog** (`internal/gui/help_dialog.go`): the [Get help dialog](#where-get-help-appears). **Save as file…** writes `nebula-report-<date>.md` with mode `0600`.

**Tests:**

- `buildReport` golden tests for: a running connection, a connect failure, an unreachable service (with the `journalctl` hint and no service log), and an import failure;
- masking with **synthetic fixtures** (`testdata/`), not the lab. The lab's lighthouses are at `127.0.0.1`, which is never masked, so the lab wouldn't exercise `PUBLIC-n`. The fixtures use TEST-NET addresses (`203.0.113.x`, `198.51.100.x`), `10.x` LAN addresses, `2001:db8::/32`, an `fe80::` address, and addresses inside free text. No fixture address, host name, cert name or group appears in the masked report, and the same address gets the same placeholder in every section;
- names that are common words: a host called `dev` leaves `/dev/net/tun` alone, a host called `lighthouse` leaves `am_lighthouse` alone, and a group called `users` stays as is in free text while it's masked in known fields;
- times like `14:03:22` and versions like `1.12.0` are not taken for addresses;
- IPv6 addresses, addresses with ports, and addresses inside free-text messages are masked; loopback, `0.0.0.0` and `169.254.x` are not;
- with the service unreachable, remembered names and the remembered overlay network still mask the GUI log, and overlay addresses become `OVERLAY-n`, not `LAN-n`;
- with masking off, the names come through unchanged;
- input lines containing `PRIVATE KEY` or `BEGIN NEBULA` are dropped and counted; a finished report that still contains either string is refused;
- certificate objects in log lines are shortened;
- the 50 KB cap drops the oldest log lines first and says so in the report.

### 10. Test lab and docs

- **`testlab/lab.sh`:**
  - Also write `testlab/dev/config.paths.yml`, a dev config with `pki.*` as **paths**, so the GUI import has something real to inline. The existing inline `config.yml` stays for `lab-import`/curl. Both files are gitignored, and `*.yml` under `testlab/dev/` is already covered.
  - Add a `lab.sh broken` command that writes bad configs for manual testing: one expired, one with a mismatched key, one with a missing key file, and one with `sshd.enabled: true`.
- **`docs/testing.md`:** add a manual GUI smoke test for M2:
  - import `config.paths.yml`, connect, see "Connected" and `192.168.100.20`, see the handshake in the logs, disconnect
  - stop the service and check the "isn't running" banner, then start it and check the banner clears without clicking anything
  - `chmod 600` the socket and check the permission message
  - import each config from `lab.sh broken` and check its message and fix button
  - re-import while connected and check **Reconnect to use it**
  - open **Get help…** after a failed connect, paste the masked report into an AI assistant, and check that it identifies the problem without asking for anything the report should have contained
  - restart the service and check that the report still shows the log lines from before the restart
- **Other docs:**
  - `docs/roadmap.md`: tick the M2 items
  - `README.md`: update the status line, which is already stale after M1
  - `docs/architecture.md`: GUI section, the persistent log, and the diagnostics endpoint
  - `docs/security.md`: the log files and masking (see step 2)

## Risks and decisions

- **Nebula's wording can change between versions.** The service prefers its own checks and `errors.Is`, and the real-Nebula tests from step 1 catch any string-match fallbacks that break on an upgrade.
- **Masking can miss things.** A host name that appears only in free text and isn't in the config, status or a known log field won't be found. The default is safe for addresses (every IP is masked by the regex pass), but names rely on the collection step. The golden tests use the lab's names, and new log fields that carry names should be added to the known-keys list.
- **Debug logging volume.** The GUI's polling requests are kept out of the file (step 2), but at debug level a busy network may still fill 6 MiB in hours rather than days. That's fine for troubleshooting a current problem. If it turns out to be too short, raise the size or drop the file to info level.
- **The service log is unreadable when the service is down.** The file is `0600` and owned by the service. That's deliberate, since it contains network details. The report instead tells the assistant how to read the system log on Linux. On Windows and macOS, the service writes to the log file before anything else at startup, so the next successful start shows why the previous one failed.
- **File picker:** on Linux the GUI calls the XDG Desktop Portal's file chooser itself over D-Bus, so users get their desktop's picker. Fyne only does that in Flatpak builds. For an app outside a sandbox, the portal returns the file's real path, so relative `pki.*` paths still resolve. In a sandbox (a Flatpak build), it would return a path under `/run/user/…/doc/`, where the key files next to the config can't be read, and every relative path would need **Choose the file…**; zip import (M5) would avoid that. Without a portal, the GUI falls back to Fyne's picker for the rest of the session. Native pickers for Windows (`IFileOpenDialog`) and macOS (`NSOpenPanel`) come with the M4 installers.
- **Paths from the admin's machine.** Configs often contain paths like `/etc/nebula/host.key` that don't exist on the user's computer. Import uses paths as written and asks for each missing file, which can mean up to three pickers. That's accepted for M2; zip import in M5 removes most of these cases.
- **Polling while the window is hidden** doesn't apply until there's a tray (M4), so there's no throttling for that in M2.
- **CI:** the pure functions, the catalogue and the report get unit tests. Fyne widget tests (`fyne.io/fyne/v2/test`) could cover button enabling and the problem view, but they're worth adding only if the window logic grows.

## Suggested commits

1. Service error reasons + `nebulatest` cert fixtures + `ipc-api.md` (step 1)
2. Service persistent log, instance ID + `GET /v1/diagnostics` (step 2)
3. Problem catalogue, connection errors and problem view (steps 3–4)
4. GUI log, poller, status header and Connect/Disconnect (the GUI log from step 9, and steps 5–6)
5. Config import with its fixes, plus `config.paths.yml` and `lab.sh broken` (step 7 and the test lab part of step 10)
6. Log view (step 8)
7. Troubleshooting report, masking and the Get help dialog (step 9)
8. Docs, roadmap and README status
