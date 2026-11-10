<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# Testing

## Unit tests (`go test ./...`)

| Package | What to test |
|---|---|
| `internal/api` | JSON round-tripping and backward compatibility (decoding with unknown fields) |
| `internal/service` | The state machine, using a **fake `nebulactl`** interface so no tun device or root is needed. Config validation, including rejecting path-based `pki.*` values. Ping input validation. |
| `internal/pki` | Inlining PEM from relative paths, and extracting cert fields |
| `internal/gui` | Logic only (formatters, view-models). Fyne has `fyne.io/fyne/v2/test` for headless widget tests. |

`internal/service` should depend on an interface, not on Nebula directly:

```go
type Engine interface {
    Start(yaml string) error
    Stop() error
    Hosts(pending bool) []HostInfo
    CloseTunnel(vpnIP netip.Addr) bool
}
```

The real implementation is in `internal/nebulactl`. Tests use a fake.

## Local test lab (integration)

To check real tunnels, you need a lighthouse and at least two hosts. On Linux the cheapest way is **network namespaces** or **Docker**. Nothing is exposed outside the machine.

### 1. Generate a test PKI
```sh
mkdir -p testlab/pki && cd testlab/pki
nebula-cert ca   -name "nebula-gui test CA"
nebula-cert sign -name lighthouse -ip 192.168.100.1/24
nebula-cert sign -name alice      -ip 192.168.100.10/24 -groups users
nebula-cert sign -name bob        -ip 192.168.100.11/24 -groups users
```
Never commit `testlab/pki/` (add it to `.gitignore`).

### 2. Topology (docker compose)
```
          docker bridge 172.30.0.0/24
   ┌──────────────┬──────────────┬──────────────┐
 lighthouse      alice          bob          (host: nebula-gui dev)
 172.30.0.2      172.30.0.10    172.30.0.11
 192.168.100.1   192.168.100.10 192.168.100.11
```
Each container runs the stock `nebula` binary with `cap_add: [NET_ADMIN]` and `devices: [/dev/net/tun]`. The lighthouse's `static_host_map` uses the bridge IP. Your local dev service uses another cert, for example `-name dev -ip 192.168.100.20/24`, and points at the lighthouse's bridge IP.

`make lab-up` and `make lab-down` wrap `docker compose -f testlab/compose.yml up -d / down`.

### 3. Manual smoke test
1. Import `testlab/dev/config.yml` in the GUI and connect.
2. The status shows `running` and overlay IP `192.168.100.20`.
3. The peer table shows the lighthouse as established.
4. Ping `192.168.100.10`. The first attempt may say "connecting…", and after that alice appears as established with an RTT.
5. Disconnect. The tun device disappears (`ip link`) and the peers list is empty.

### Relay scenario (later)
Block direct UDP between alice and the dev host with `iptables` in the container, enable `relay.use_relays` and `am_relay` on the lighthouse, and check that the peer row shows `relayed: true`.

## CI
- `go vet`, `golangci-lint`, and `go test ./...` on Linux, macOS, and Windows runners.
- Linux only: a job that runs the docker test lab and a headless test of the service through its API (no GUI).
