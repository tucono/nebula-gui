<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# Testing

## Unit tests (`go test ./...`)

| Package | What to test |
|---|---|
| `internal/api` | JSON round-tripping and backward compatibility (decoding with unknown fields) |
| `internal/service` | The state machine, using a **fake engine** so no tun device or root is needed. Config validation, including rejecting path-based `pki.*` values. Config store, redaction, log buffer. Ping input validation (M3). |
| `internal/pki` | Inlining PEM from relative paths, and extracting cert fields |
| `internal/gui` | Logic only (formatters, view-models). Fyne has `fyne.io/fyne/v2/test` for headless widget tests. |

`internal/service` depends on an interface, not on Nebula directly (`internal/service/engine.go`):

```go
type Engine interface {
    Version() string
    Check(yaml string) (*ConfigInfo, error) // dry run + cert details
    Start(yaml string) (Instance, error)
}

type Instance interface {
    Stop()
    Done() <-chan struct{}
    Err() error
}
```

`NebulaEngine` adapts `internal/nebulactl` to it. Unit tests use a fake (`internal/service/fake_test.go`). M3 adds `Hosts` and `CloseTunnel` to `Instance`.

### Tests with a real Nebula

`internal/nebulactl/nebulatest` generates a throwaway CA, host cert and config in memory, so no key material is ever committed. Its config sets `tun.disabled: true`, so Nebula runs without privileges. `nebulactl.TestStartStop` and `service.TestRealNebulaOverSocket` use it to check the full import, connect and disconnect path in plain `go test`.

These tests skip under `-race`. Nebula 1.12.0's disabled tun device has a data race on close, which the detector reports. Real tun devices don't use that code.

## Local test lab (integration)

`testlab/lab.sh` runs a lighthouse and two hosts as plain `nebula` processes on 127.0.0.1, with the tun device disabled. That needs no root, Docker or network namespaces, and nothing is exposed outside the machine. A disabled tun device still answers ICMP echo, so the dev service can ping alice and bob over the overlay.

```
            127.0.0.1 (underlay)
   ┌──────────────┬──────────────┬──────────────┐
 lighthouse      alice          bob          dev (nebula-guid under test)
 :4301           :4302          :4303        random port, real tun
 192.168.100.1   192.168.100.10 192.168.100.11 192.168.100.20
```

```sh
make lab-up        # build nebula + nebula-cert at the pinned version, create the PKI, start the nodes
make dev-service   # other terminal: the service under test (needs CAP_NET_ADMIN)
make lab-import    # PUT testlab/dev/config.yml (inline PKI) to the dev service
testlab/lab.sh broken  # bad configs for the GUI import, in testlab/dev/broken/
make lab-down
testlab/lab.sh clean   # also deletes the generated PKI
```

`lab.sh up` writes the PKI to `testlab/pki/`, node configs and logs to `testlab/run/`, and the dev config to `testlab/dev/`, twice: `config.yml` with the PKI inline (for `lab-import` and curl), and `config.paths.yml` with the PKI as relative paths (for the GUI's **Import config…**, which inlines them). All three directories are gitignored. Never commit them.

`lab.sh broken` writes configs the GUI import should refuse, each with its own message, to `testlab/dev/broken/`: `expired.yml` (a host cert that has expired), `key-mismatch.yml` (dev's cert with alice's key), `missing-key.yml` (a key path from the admin's machine), `sshd.yml` (the debug console turned on) and `notes.txt` (not a config).

### Smoke test with curl (M1)
```sh
S=$XDG_RUNTIME_DIR/nebula-guid.sock
make lab-import
curl -s --unix-socket $S http://x/v1/status          # configured, cert "dev", stopped
curl -s --unix-socket $S -X POST http://x/v1/connect  # state "running"
curl -s --unix-socket $S "http://x/v1/logs?since=0"  # "Handshake message received" from lighthouse
curl -s --unix-socket $S http://x/v1/config          # pki.key is <redacted>
ping -c 3 192.168.100.10                              # alice answers over the overlay
curl -s --unix-socket $S -X POST http://x/v1/disconnect
```

Without `CAP_NET_ADMIN`, connect fails with `internal` ("Failed to get a tun/tap device"), and `/v1/status` shows state `error`.

### Operator check after GUI commits
Unit and Fyne tests can't show how the window behaves on a real desktop: layout, resizing, focus, the clipboard, and the compositor. So every commit that changes the GUI (`internal/gui`, `cmd/nebula-gui`) is followed by a short check that a person runs by hand:

1. The author of the commit writes a check plan: numbered steps covering what the commit changed, each with a command to run where one is needed and the expected result.
2. Every plan also includes the baseline below.
3. The operator runs the plan and reports pass or fail for each step, with a screenshot for anything that looks wrong.
4. Failures are fixed in a follow-up commit, and that commit gets its own check.

**Baseline**, for every GUI commit:
- The window opens at its normal size, with nothing cut off or overlapping.
- Resize the window smaller and larger. The content reflows or scrolls, and never slides under the title bar.
- Every button reacts where the pointer is.
- Closing the window quits the app.

Note which desktop and session the check ran on (for example COSMIC on Wayland), because compositors differ.

### GUI smoke test (M2/M3)
1. Import `testlab/dev/config.paths.yml` in the GUI and connect.
2. The status shows `running` and overlay IP `192.168.100.20`.
3. The peer table shows the lighthouse as established.
4. Ping `192.168.100.10`. The first attempt may say "connecting…", and after that alice appears as established with an RTT.
5. Disconnect. The tun device disappears (`ip link`) and the peers list is empty.

### Relay scenario (later)
With every node on 127.0.0.1, direct UDP can't be blocked per host. Testing relays needs separate network namespaces or containers: block direct UDP between alice and the dev host, enable `relay.use_relays` and `am_relay` on the lighthouse, and check that the peer row shows `relayed: true`.

## CI
- `go vet`, `golangci-lint`, and `go test ./...` on Linux, macOS, and Windows runners.
- The real-Nebula tests above need no privileges, so they run in CI: `nebulactl.TestStartStop` on all three OSes, and `service.TestRealNebulaOverSocket` on Linux and macOS.
