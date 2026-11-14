<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# M3 plan: Peers and ping

Detailed plan for the [M3 roadmap items](roadmap.md#m3-peers-and-ping).

## What M3 delivers

At the end of M3, a **Peers** tab shows who this computer is talking to, under names the user recognises:

- a list of peers with their state (connected, connecting, not connected), how they're reached (direct or relayed) and the last ping time;
- **names the user chooses** ("Laptop", "Brother's PC"), shown instead of overlay IPs. The IP is still one click away (**Copy IP**);
- **Add peer…**: save a name and an overlay IP for a peer that has no tunnel yet, so it shows up in the list as "Not connected";
- **Ping** from any row. Pinging a peer with no tunnel makes Nebula find it through the lighthouse and connect, so nobody needs a terminal to "wake up" a connection;
- **Close tunnel** from a row, for when a connection seems stuck.

The people using the app are the same as in M2 (see [Who this is for](m2-plan.md#who-this-is-for)): plain words, no jargon outside the details, and a clear next step when something doesn't work.

**Left for later:** the tray and slower polling while the window is hidden (M4), routes and cert-expiry warnings (M5), testing relays (needs namespaces, see [testing.md](testing.md#relay-scenario-later)).

## Starting point

- `internal/nebulactl.Instance` already has `Hosts(pending bool)` and `CloseTunnel(addr)`, mapped from `Control.ListHostmapHosts` and `Control.CloseTunnel`. The service's `Instance` interface (`internal/service/engine.go`) doesn't expose them yet.
- `internal/api` has `CodePingFailed` but no peer or ping types, and the server has no routes for them.
- The GUI has two tabs (Status, Logs) and a poller that calls `GET /v1/status` every 2 s.
- The GUI already keeps one small settings file, `mask-names.json`, in `os.UserConfigDir()/nebula-gui/` with mode `0600`. Saved peers follow the same pattern.
- `make dev-service` already gives the dev binary `cap_net_raw`, which privileged ICMP needs.

## How peers are shown

### Where names come from

Each row shows one name, picked in this order:

1. **The user's name for it** (a saved peer, see below).
2. **The cert name** from Nebula's hostmap (for example `alice`). Whoever signed the certs chose it.
3. **The overlay IP**, if neither exists.

Below the name, in smaller text: the overlay IP, and the cert name when it differs from the user's name. Hovering isn't needed to see the IP.

### Saved peers

A saved peer is a name and an overlay IP the user typed in, or gave to a peer that was already in the list (**Rename…**).

- Stored by the GUI, not the service. Names are one user's labels and the service is shared by every user of the computer; the API stays unchanged.
- File: `os.UserConfigDir()/nebula-gui/peers.json`, mode `0600`, like `mask-names.json`:
  ```json
  { "peers": [ { "ip": "192.168.100.10", "name": "Alice's laptop" } ] }
  ```
- Keyed by overlay IP. A peer's IP is fixed by its cert, while cert names can change when a cert is re-signed. If a cert is re-signed with a different IP, the saved name simply stops matching.
- Names are 1–40 characters after trimming. Two saved peers can't share an IP. Two can share a name (the IP below tells them apart).
- At most 200 saved peers, which is plenty for a network of friends and family.
- **Kept when the config is removed.** Removing a config is usually followed by importing a new one for the same network. Saved peers whose IP isn't in the current overlay network are hidden, not deleted, so they come back if the old network does.

### The list

One list, sorted: connected peers first, then connecting, then not connected; alphabetically by shown name within each group.

| State | Shown as | Comes from |
|---|---|---|
| `established` | **Connected**, with "direct" or "relayed" and the last ping time if there is one | `GET /v1/peers` |
| `pending` | **Connecting…** | `GET /v1/peers` |
| saved, no tunnel | **Not connected** | `peers.json` only |

- A peer matches a saved peer if any of its `vpn_ips` equals the saved IP.
- **Not connected doesn't mean offline.** Nebula only knows about peers it has a tunnel with, and it closes idle tunnels after a while. The empty state and a one-line note at the top of the list say so: "Not connected means there's no connection right now. Ping to check whether it's reachable."
- When Nebula isn't running, the list shows only saved peers, all as Not connected, with Ping disabled and a note: "Connect to see who's online."
- When there are no peers at all: an empty state that explains what the tab is for, with **Add peer…**.

The list is a `widget.List` with a custom row, not a `widget.Table`: rows are a name block, a state block and a **⋯** button, and long names are truncated so the window never grows (see [Risks](#risks-and-decisions)).

### Row actions

| Action | When | What it does |
|---|---|---|
| **Ping** | Nebula running | Pings the peer; see [Pinging](#pinging) |
| **Copy IP** | always | Copies the first overlay IP to the clipboard |
| **Rename…** / **Give it a name…** | always | Saves or changes the user's name. For a peer without a saved name, this creates one. |
| **Remove from list** | saved peers | Deletes the saved name. Doesn't touch Nebula; a connected peer stays in the list under its cert name. |
| **Close connection** | `established` or `pending` | `POST /v1/peers/{ip}/close`. The row returns to Not connected (saved) or disappears (unsaved). Nebula reconnects as soon as either side sends traffic again. |

The menu is the only place for actions; there's no right-click menu, which Fyne's list doesn't support per row without a custom widget.

### Add peer…

A button above the list opens a dialog:

- **Name** (for example "Brother's PC") and **Overlay IP** (for example `192.168.100.10`).
- The IP is checked as the user types, against the `overlay_ips` from the latest status (which carry the prefix, such as `192.168.100.3/24`):
  - not an IP: "That doesn't look like an address. It should look like 192.168.100.10."
  - outside the network: "That address isn't on your Nebula network. Addresses on your network start with 192.168.100." (built from the prefix);
  - this computer's own IP: "That's this computer."
  - already saved: "Already in your list as Alice's laptop."
  - with no status yet (service unreachable or not configured), only the format is checked.
- Buttons: **Add**, and **Add and ping** (enabled while Nebula is running), which saves the peer and pings it right away.

There's no separate "Ping…" dialog for one-off addresses: adding a peer and pinging it covers that, and **Remove from list** undoes it.

## Pinging

### What the user sees

Ping sends 3 echo requests with a 5 s timeout. While it runs, the row shows "Pinging…" and its Ping action is disabled.

The first ping to a peer without a tunnel often loses its first replies while Nebula asks the lighthouse and handshakes. So the result is judged together with the tunnel state after the ping (the GUI polls `GET /v1/peers` right after):

| Result | Shown in the row |
|---|---|
| some replies | "23 ms" (average), and "2 of 3 replies" when some were lost |
| no replies, no tunnel before, a tunnel (pending or established) now | "Connecting…", and the GUI pings **once more** automatically |
| no replies after that retry, tunnel established | "Connected, but it didn't answer. Its firewall may not allow ping." |
| no replies, no tunnel | "No answer. It may be switched off or not connected to Nebula, or the address may be wrong." |
| request rejected (`rate_limited`) | "Too many pings at once. Try again in a moment." |
| `ping_failed` / `internal` | a [problem](m2-plan.md#rules-for-messages) with details and **Get help…** |

`describePing(before, after *api.Peer, res *api.PingResponse, err error) pingView` is a pure function with a table test, like `describeStatus`.

The "firewall" wording matters: Nebula's own firewall drops ICMP unless the peer's `firewall.inbound` allows it. The lab allows it; a real network might not.

### In the service

- `POST /v1/ping` uses `github.com/prometheus-community/pro-bing` in privileged mode (raw ICMP socket). The service is root or SYSTEM in production; in development `make dev-service` already sets `cap_net_raw`.
- The packets go out through the tun device like any other traffic, so Nebula does the lighthouse lookup and the handshake. The service doesn't call `CreateTunnel` itself.
- **Checks**, in this order:
  1. Nebula must be running (`not_running`).
  2. `vpn_ip` must parse as an IP (`bad_request`), and be inside one of this host's cert networks and not this host's own address (`bad_request` with reason `not_in_overlay` or `own_address`).
  3. `count` 1–10 (default 3), `timeout_ms` 100–15000 (default 5000) (`bad_request`).
  4. Rate limits (`rate_limited`, HTTP 429): at most **4 pings in flight**, at most **one per peer** at a time, and at most **60 pings started per minute** across all clients.
- After a ping with at least one reply, the average RTT is kept in memory per overlay IP and returned as `last_ping_rtt_ms` in `GET /v1/peers`. The map is cleared on disconnect and bounded by the overlay network's size in practice (only addresses that answered are stored).
- The service logs each ping at info level with the IP, count and result, so troubleshooting reports show it.

## Steps

### 1. Service: peers and closing tunnels

**Files:** `internal/api/types.go`, `client.go`; `internal/service/engine.go`, `manager.go`, `server.go`, `fake_test.go`.

- `api.Peer` and `api.PeersResponse` as in [ipc-api.md](ipc-api.md#get-v1peers). `Client.Peers` and `Client.ClosePeer`.
- `Instance` gains `Hosts(pending bool) []nebulactl.HostInfo` and `CloseTunnel(netip.Addr) error`. The real one forwards to `nebulactl`; the fake keeps a host list tests can set.
- `GET /v1/peers`: established hosts then pending ones, mapped to `api.Peer` with `last_ping_rtt_ms` from step 2 (null for now). A host in both maps is listed once, as established. When Nebula isn't running the list is empty, not an error.
- `POST /v1/peers/{vpn_ip}/close`: `bad_request` for a bad IP, `not_running`, and the new `not_found` (404) when there's no tunnel (`nebulactl.ErrNoTunnel`). Returns `204 No Content`.
- Tests with the fake engine: listing in both states, dedup, the close errors.

### 2. Service: ping

**Files:** `internal/service/ping.go`, `ping_test.go`; `internal/api` types and `Client.Ping`.

- A small `pinger` interface (`Ping(ctx, addr, count, timeout) (PingResult, error)`) so tests use a fake; the real one wraps pro-bing. `go get github.com/prometheus-community/pro-bing` lands in this commit.
- Validation, the rate limiter and the RTT map from [In the service](#in-the-service). The limiter is a counter plus a per-IP set under the manager's lock, and a one-minute token bucket.
- New error codes `not_found` and `rate_limited`, and the `not_in_overlay` and `own_address` reasons, in `internal/api` and [ipc-api.md](ipc-api.md).
- Tests: every validation branch, both limits, the RTT map filling and clearing on disconnect, and `last_ping_rtt_ms` appearing in `GET /v1/peers`.
- No real-ICMP test in CI: it needs a tun device and `cap_net_raw`. The lab smoke test covers it.

### 3. GUI: saved peers and the peer model

**Files:** `internal/gui/peerstore.go`, `peers.go` and tests.

- `peerStore`: load, add, rename, remove and save `peers.json`, `0600`, with the limits from [Saved peers](#saved-peers). A missing or damaged file starts empty, like `nameStore`. An empty path keeps it in memory (tests).
- `buildPeerRows(peers []api.Peer, saved []savedPeer, overlays []netip.Prefix, running bool) []peerRow`: a pure function that merges, names, filters by overlay network, sorts and decides which actions each row has. Table tests cover every row of [The list](#the-list) and [Row actions](#row-actions).
- `checkNewPeer(name, ip string, st *api.StatusResponse, saved []savedPeer) error`: the Add peer checks, with the messages above.

### 4. GUI: polling peers

**File:** `internal/gui/poller.go`.

- While Nebula is running, the poller also calls `GET /v1/peers`: every 2 s while the Peers tab shows, every 10 s otherwise (the Logs tab's pattern). When it isn't running, it doesn't call it.
- `Poke` (already used by **Try again**) also refreshes peers, so a ping or close shows its effect at once.
- The status header gains a peer count when connected: "Connected · 2 peers".

### 5. GUI: Peers tab

**File:** `internal/gui/peers_view.go`.

- A third tab, **Peers**, between Status and Logs. Above the list: **Add peer…** and the one-line note about Not connected.
- Rows and the ⋯ menu as above. Rename and the confirmation for **Close connection** ("Close the connection to Alice's laptop? It reconnects the next time either computer sends something.") are small dialogs.
- **Copy IP** uses the window's clipboard and says "Copied" briefly in the row.

### 6. GUI: Add peer and ping

**Files:** `internal/gui/add_peer_dialog.go`, `ping.go` and tests.

- The Add peer dialog with live checks (step 3) and **Add** / **Add and ping**.
- The ping flow: call `Ping`, poke the poller, apply `describePing`, retry once when it says so. Pings run off the UI goroutine and report back through `fyne.Do`.
- Ping errors that aren't plain results go through the M2 problem catalogue, with new entries for `rate_limited` and the two reasons (which shouldn't happen after the GUI's own checks, but the service is the authority).

### 7. GUI: logs and troubleshooting report

**Files:** `internal/gui/applog.go` callers, `report.go`, `mask.go`, tests and golden files.

- The GUI log records peer actions by **IP only**: "ping 192.168.100.10: 3/3, 23 ms", "peer added 192.168.100.10", "closed connection to 192.168.100.10". Saved names never go in the GUI log.
- The report gains a **Peers** section built from `GET /v1/peers`: one line per peer with cert name, overlay IP, state, direct/relayed, current remote and last RTT. Masking already handles all of these (`HOST-n`, `OVERLAY-n`, `PUBLIC-n`). Saved names are **never in a report**, masked or not: they're personal ("Mom's laptop"), they don't help with troubleshooting, and a free-form name can't be masked reliably.
- `rememberedNames` also collects peer cert names from `GET /v1/peers`, so a report made later still masks them.
- Golden files gain a peers section, using the lab's names and documentation ranges only.

### 8. Test lab and docs

- `docs/testing.md`: the M3 GUI smoke test (below) and the new unit tests.
- `docs/architecture.md`: the Peers tab, saved peers, the ping flow.
- `docs/ipc-api.md`: the final endpoint shapes and new error codes.
- `docs/security.md`: ping limits, `peers.json`, saved names never in reports.
- `docs/roadmap.md`: tick the M3 items. `README.md`: status line.

### GUI smoke test

The manual test for M3 is in [testing.md](testing.md#gui-smoke-test-m3): add Alice by IP, see her as Not connected, ping her and see her become Connected under her name, plus the Add peer checks, closing a connection, a peer that doesn't answer, and a report without saved names.

## Risks and decisions

- **Ping can mislead.** A peer that drops ICMP looks unreachable even when the tunnel works. The result wording uses the tunnel state to tell "didn't answer" from "couldn't reach", and the docs for whoever runs the network (README now, user docs in M5) should mention allowing `icmp` inbound.
- **Privileged ICMP on each platform.** Linux needs `cap_net_raw` (root in production, `setcap` in dev). macOS and Windows services run as root or SYSTEM. If pro-bing turns out to need special handling on Windows (raw sockets and the Wintun adapter), the fallback is `IcmpSendEcho2` through `golang.org/x/sys/windows`, behind the same `pinger` interface.
- **Window growth on COSMIC/Wayland.** Content that outgrows the window has shifted the layout and the pointer on the developer's desktop. Rows truncate long names and IPs, the list scrolls inside a fixed area, and dialogs have fixed widths.
- **Saved names are per user, per computer.** They don't follow a user to another machine, and the admin can't hand them out. That's fine for now; a later option is reading names from the config (for example a `nebula-gui:` section) that the admin distributes.
- **IPv6 overlays.** Nebula 1.12 certs can carry several networks, including IPv6. Checks use every prefix in `overlay_ips`, and a peer can match a saved IP on any of its `vpn_ips`.
- **The rate limits are guesses.** 4 in flight and 60 a minute are generous for a person clicking Ping and too low for a scan. Revisit if real use hits them.

## Suggested commits

1. Service: peers and close endpoints, API types and client (step 1)
2. Service: ping with limits and RTT tracking (step 2)
3. GUI: saved peers and peer rows, with the poller change (steps 3–4)
4. GUI: Peers tab, Add peer and ping (steps 5–6)
5. GUI: peer actions in the GUI log and report (step 7)
6. Docs, roadmap and README status (step 8)
