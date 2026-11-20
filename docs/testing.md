<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# Testing

## Unit tests (`go test ./...`)

| Package | What to test |
|---|---|
| `internal/api` | JSON round-tripping and backward compatibility (decoding with unknown fields) |
| `internal/service` | The state machine, using a **fake engine** so no tun device or root is needed. Config validation, including rejecting path-based `pki.*` values. Config store, redaction, log buffer. Peers and close (dedup, errors), ping validation, rate limits and the RTT map, with a fake pinger (M3). Settings and autoconnect: off by default and after an import, a damaged file, retries and what stops them, unknown fields refused (M4). |
| `cmd/nebula-guid` | Argument handling, the start-up privilege check, and the Windows service handler's stop path with a fake `Serve` (M4). |
| `internal/autostart` | Each OS's start-at-sign-in entry, written to a temp home; on Windows the Run value under a test key in HKCU, and an entry turned off in Task Manager (M4). |
| `internal/userfiles` | Each OS's per-user paths from a home folder, and that they match where the standard library and Fyne put this user's files. Removing them from a fake home: everything, or only the start-at-sign-in entry; other apps' files kept; a symlink to outside the home folder, at the app's folder or above it, never followed. Listing users from `passwd` lines, from a `/Users`-like folder and from a fake Windows profile list. On Windows, removing the Run and StartupApproved values from a hive, and, on an elevated runner, from a saved `NTUSER.DAT` loaded and unloaded as for someone who isn't signed in (M4.5). |
| `internal/pki` | Inlining PEM from relative paths, and extracting cert fields |
| `internal/gui` | Logic (problem catalogue, status model, PEM inlining, log formatting, masking; in M3 the saved-peer store, peer rows, the Peers tab, Add peer checks, ping results and the report's Peers section) plus headless window tests with `fyne.io/fyne/v2/test` and a fake service. M4 adds `trayModel`'s table test, the hidden-window poll rates, two in-process copies for the single-instance socket, the Settings tab and the installer-aware problems. The troubleshooting report has golden files in `internal/gui/testdata/`, built from a synthetic service log that uses only documentation ranges and made-up names; run `go test ./internal/gui -run TestReportGolden -update` to rewrite them, and review the diff. |

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

`NebulaEngine` adapts `internal/nebulactl` to it. Unit tests use a fake (`internal/service/fake_test.go`). M3 adds `Hosts` and `CloseTunnel` to `Instance`, and a `pinger` interface with a fake, so ping is tested without ICMP.

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

To test from a VM (Windows, macOS), start the lab with `LAB_HOST` set to this machine's address on the VM's network, for example `LAB_HOST=10.0.2.2 testlab/lab.sh up` (run `lab.sh down` first if it's already up). The nodes listen on that address instead of 127.0.0.1, and the dev configs point at it, so import `testlab/dev/config.paths.yml` on the VM along with `testlab/pki/`. This exposes the lab on that network, so use it only on a host-only or NAT network. The firewall must allow UDP 4301–4303 from the VM.

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
- Closing the window hides it in the tray (or quits the app, on a desktop without a tray). Quit from the tray or File → Quit ends it.

Note which desktop and session the check ran on (for example COSMIC on Wayland), because compositors differ.

### GUI smoke test (M2)
Start with `make lab-up`, `make dev-service` and `make gui`, and `testlab/lab.sh broken` for step 4.

1. **Import and connect.** Import `testlab/dev/config.paths.yml` and click Connect. The header shows "Connected" and `192.168.100.20`. The Logs tab shows the lighthouse's "Handshake message received". Disconnect.
2. **Service stopped.** Stop `make dev-service`. The banner says the background service isn't running. Start it again: the banner clears within a few seconds without clicking anything.
3. **No permission.** `chmod 000 $XDG_RUNTIME_DIR/nebula-guid.sock`. The dev service runs as you, so this is what stands in for a socket owned by another group. The banner explains the missing permission. `chmod 660` it again and the banner clears.
4. **Bad configs.** Import each file in `testlab/dev/broken/`. Each shows its own message; `sshd.yml` offers to turn the console off, and `missing-key.yml` asks for the key file.
5. **Re-import while connected.** Import the config again while connected. The header offers **Reconnect to use it**.
6. **Get help.** Make a connect fail: stop the service, `sudo setcap -r bin/nebula-guid-dev`, and start `bin/nebula-guid-dev` with the arguments `make dev-service` uses. Click Connect, open **Get help…** from the banner and paste the masked report into an AI assistant. It should identify the problem without asking for anything the report should have contained. No `192.168.100.` address or lab host name appears with masking on.
7. **Service restart.** Restart the service and open **Get help…** again. The service log still shows the lines from before the restart.

### GUI smoke test (M3)
1. Import `testlab/dev/config.paths.yml` and connect. The Peers tab shows `lighthouse` as Connected.
2. **Add peer…** with name "Alice" and `192.168.100.10`. The row shows Alice, Not connected.
3. Ping Alice. The row shows "Pinging…", and a line above the list may show "Alice: Connecting…" first, then "Alice: Answered in N ms." The row becomes Connected, direct, with the last ping time, and the cert name `alice` shows under the name.
4. Try to add `10.0.0.1`, this host's own `192.168.100.20`, and `192.168.100.10` again. Each gets its own message.
5. **Close connection** on Alice. She goes back to Not connected; ping brings her back.
6. Add `192.168.100.99` (nothing there) and ping it. It ends with "No answer…", and the row stays Not connected.
   Then ping it and Alice right after each other: each gets its own result line, and neither overwrites the other.
7. Stop bob's lab node, add bob (`192.168.100.11`) and ping: "No answer…". Start it again and ping: Connected.
8. **Get help…**: the Peers section lists `HOST-n · OVERLAY-n · established · direct PUBLIC-n:4242 · last ping N ms` lines, with the same placeholders as the service log. With **Include network details** it shows `alice · 192.168.100.10 · …`. The name "Alice" doesn't appear in the report either way, and Recent app events show `peer added`, `ping` and `closed connection` with only an IP.
9. Disconnect. The tun device disappears (`ip link`). Only saved peers remain, all Not connected, with Ping disabled, and the result lines are gone. A report now says "None: Nebula isn't running." under Peers.
10. Remove the config and import it again. The saved peers are still there.

### Installer smoke tests (M4)

CI installs and removes every package on each push ([development.md](development.md#testing-packages)). These are the checks it can't do, run by hand from a release candidate's draft release: the developer's COSMIC desktop for Linux, a Windows 10 VM reverted to a snapshot before each run, and a Mac that the uninstall script returns to a clean state. Stop `make dev-service` first on Linux, since both use Nebula's port and tun name.

1. **Download** from the draft release and check the file against `SHA256SUMS`. Note the browser's and the OS's warnings, which the README's Install section must match.
2. **Install** by double-clicking. The service runs (`systemctl status nebula-guid` as `nebula-guid`; `sudo launchctl print system/io.github.tucono.nebula-guid`; `sc.exe query nebula-guid`). On Linux, before restarting, the app shows **Restart your computer to finish**; after a restart it opens without a problem. On Windows and macOS it works at once.
3. **Import a config** made with `testlab/` (started with `LAB_HOST` for a VM) or a real one, and connect. The tray shows Connected.
4. **Close the window:** the app stays in the tray, with a notification the first time. Open the app again from the launcher or Start Menu: the same window comes back, and there's one copy (`pgrep -c nebula-gui`, Task Manager).
5. **Reboot** with the config imported and the settings untouched: not connected, and Connect automatically is unticked. Turn it on, reboot, wait a minute before signing in, then sign in: already connected, and the service log shows "connecting automatically" from before the sign-in. Repeat with the lighthouse given by a DNS name.
6. **Start at sign-in:** after the reboot the app is in the tray without its window.
7. **Quit** from the tray: the first time, a dialog says the connection stays on. The tunnel stays up (`ip link`, `ifconfig`, `Get-NetAdapter`). Open the app: Connected.
8. **Without a tray** (COSMIC with the tray applet removed from the panel): closing the window quits the app, and Settings doesn't offer start at sign-in.
9. **Another account** (Windows: a non-admin account added with `net localgroup nebula-gui <name> /add`, then signed in) can use the app. An account not in the group gets the no-permission problem with the right command.
10. **Help → About nebula-gui** shows the version and the notice, and **Open-source licenses** opens `THIRD_PARTY_NOTICES.txt`.
11. **Upgrade** by installing a newer build over it: the connection comes back if it was up, and no one is granted access.
12. **Uninstall:** the service and tun device are gone, the state directory remains. Then remove it (Linux: delete `/var/lib/nebula-gui`; macOS: `--remove-settings`; Windows: answer Yes) and check it's gone.

In M4 these ran step by step as each installer was built, and the release candidates (`v0.1.0-rc.1` to `rc.3`) were then downloaded, installed and connected on all three. Findings: the release check expected the wrong `.deb` name (fixed), Defender quarantined an unsigned, stripped Windows build (fixed with version details and unstripped Windows builds), and the Windows VM needs Mesa for OpenGL ([platforms.md](platforms.md#windows)).

### Relay scenario (later)
With every node on 127.0.0.1, direct UDP can't be blocked per host. Testing relays needs separate network namespaces or containers: block direct UDP between alice and the dev host, enable `relay.use_relays` and `am_relay` on the lighthouse, and check that the peer row shows `relayed: true`.

## CI
- `go vet`, `golangci-lint`, and `go test ./...` on Linux, macOS, and Windows runners.
- Every package built, installed, checked and removed on its own OS ([development.md](development.md#testing-packages)).
- The real-Nebula tests above need no privileges, so they run in CI: `nebulactl.TestStartStop` on all three OSes, and `service.TestRealNebulaOverSocket` on Linux and macOS.
