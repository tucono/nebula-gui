<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# Setting up a network for nebula-gui users

This guide is for the person who runs the network: you create the certificates, run the lighthouse and hand each person a config file. The people you hand them to only install the app and click **Import config…**.

All addresses and names below are examples (overlay `192.168.100.0/24`, `lighthouse.example.net`). Use your own.

## Overview

1. Create a certificate authority (CA) once, on one machine you control.
2. Run a lighthouse on a server with a public address.
3. For each device, sign a certificate and build a single `config.yml` with [`tools/netadmin/make-config.sh`](../tools/netadmin/make-config.sh).
4. Send each config privately. The person imports it and connects.
5. Reissue configs before the certificates expire, and block lost devices.

## 1. The certificate authority

Work in a private folder **outside this repository**, for example `~/nebula-ca/`, readable only by you (`chmod 700`).

Use the `nebula-cert` that matches the Nebula built into the app. From the repo:

```bash
go build -o ~/nebula-ca/ github.com/slackhq/nebula/cmd/nebula-cert
cd ~/nebula-ca
./nebula-cert ca -name "Our network" -duration 17520h   # two years
```

This writes `ca.crt` and `ca.key`.

- **`ca.key` never leaves this folder.** Anyone holding it can add devices to your network. Back it up offline (for example on an encrypted USB stick). Add `-encrypt` to the `ca` command to protect it with a passphrase.
- `ca.crt` is public within the network: it goes into every config.
- Every device certificate stops working when the CA expires, so choose a duration you're happy to renew on. Put the expiry date in your calendar.

## 2. The lighthouse

The lighthouse helps devices find each other. It runs plain `nebula`, not this app, on a server with a public IP address or DNS name.

- Sign it a certificate like any device (step 3), typically with the first overlay address, `192.168.100.1`.
- In its config set `lighthouse.am_lighthouse: true`, a fixed `listen.port` (such as `4242`), and leave `lighthouse.hosts` empty.
- Allow that UDP port through the server's firewall.
- To help devices behind strict NAT reach each other, also set `relay.am_relay: true`, and keep the `relay` section in the device template.

Nebula's [example config](https://github.com/slackhq/nebula/blob/master/examples/config.yml) documents every setting.

## 3. A certificate and config per device

### Plan names, addresses and groups

Sign **one certificate per device**, not per person: a laptop and a desktop each get their own. Keep a list (a spreadsheet is fine) with, per device:

| Name | Overlay IP | Groups | Expires | Fingerprint |
|---|---|---|---|---|
| `alice-laptop` | `192.168.100.10` | `family` | 2028-11-18 | `…` |
| `bob-desktop` | `192.168.100.11` | `friends` | 2028-11-18 | `…` |

- **Names and IPs must be unique.** The app shows peers by their certificate name and lets people save their own names per overlay IP, so keep a device's IP when you reissue its certificate.
- **Groups** are what the firewall rules match on. Decide them up front, for example `family` and `friends`.
- The **fingerprint** (`nebula-cert print -path alice-laptop.crt`) is what you need to block the device later.

### Sign the certificate

```bash
./nebula-cert sign -name alice-laptop -networks 192.168.100.10/24 -groups family \
  -out-crt alice-laptop.crt -out-key alice-laptop.key
```

The certificate is valid until one second before the CA expires unless you pass `-duration`. Don't encrypt device keys: the app can't use password-protected keys.

**If the person is comfortable with a terminal**, it's better that their private key never passes through you. They run `nebula-cert keygen -out-key alice-laptop.key -out-pub alice-laptop.pub` and send you only the `.pub` file. You sign it with `-in-pub alice-laptop.pub` instead of `-out-key`, and send back the certificate together with a config whose `pki` section gives the CA and certificate inline and the key as a file name (`key: alice-laptop.key`). When they import it, the app reads the key from next to the config, or asks them to find it. For most people, generating the key for them, as above, is fine.

### Build the config

Set up the template once:

```bash
cp <repo>/tools/netadmin/config-template.example.yml ~/nebula-ca/config-template.yml
```

Edit it: put your lighthouse's overlay IP and public address in `static_host_map`, `lighthouse.hosts` and `relay.relays`, and adjust the firewall rules to your groups. By default a device accepts pings from everyone and all traffic from the `family` group. Things to keep in mind:

- Don't add a `pki` section; the script adds it.
- Keep `sshd` off. The app refuses configs that turn Nebula's SSH console on.
- Leave `tun.dev` unset. The app picks a working name on Windows.
- Keep `listen.port: 0`, so devices behind home routers don't need any ports opened.
- Avoid `tun.unsafe_routes` unless you mean to route traffic through someone else's network.

Then, for each device:

```bash
cd ~/nebula-ca
<repo>/tools/netadmin/make-config.sh ca.crt alice-laptop.crt alice-laptop.key
```

This writes `config-alice-laptop.yml`, readable only by you: the template with the CA, the certificate and the key pasted in. Because everything is in one file, the app imports it without asking for anything else. The script refuses a CA key in place of a device key, a password-protected key, a template with a `pki` section or with `sshd` turned on, and, when `nebula-cert` is on `PATH` (or set in `NEBULA_CERT`), a certificate that's expired or signed by a different CA. `-o` names the output file, `-t` picks another template and `-f` overwrites. Run it with no arguments for the full usage.

## 4. Sending the config

The config contains the device's private key. Treat it like a password:

- Hand it over **in person** (USB stick) or through an **end-to-end encrypted messenger** such as Signal. Not email, SMS or a shared cloud folder.
- Tell the person: install nebula-gui, click **Import config…**, pick the file, then **Connect**. After importing they can delete the file; the app keeps its own protected copy.
- Delete your copy of the config once it's imported. You can always build it again from the certificate and key, or sign a new certificate.

If something goes wrong, the person can use **Get help…** in the app and send you the report. Ticking **Include network details** gives you the real names and addresses.

## 5. Renewing and blocking

### Before certificates expire

The app warns people when their certificate or the CA has expired, and tells them to ask you for a new config. To avoid that:

- About a month before a device certificate expires, sign a new one with the **same name and IP**, build a new config and send it. Importing it replaces the old one; the app offers to reconnect.
- Before the CA expires, create a new CA and reissue every certificate. To avoid a hard cutover, put **both** CA certificates in `pki.ca` for a while (one after the other in the same file), then remove the old one once everyone has a new config.

### A lost or compromised device

Add the certificate's fingerprint to `pki.blocklist` in the lighthouse's config and restart it:

```yaml
pki:
  blocklist:
    - c99d4e650533b92061b09918e838a5a0a6aaee21eed1d12fd937682865936c72
```

The lighthouse then refuses the device, but other devices that already know its address could still connect to it until they also have the blocklist. For a stolen device, add the `blocklist` to the template as well and send everyone a new config. If many devices are affected, or the CA key itself may have leaked, create a new CA and reissue everything.

## Checklist

- [ ] `ca.key` is outside the repository, readable only by you, and backed up offline.
- [ ] Every device has its own certificate, with a unique name and IP, recorded in your list.
- [ ] Configs went out privately, and your copies are deleted.
- [ ] Reminders are set for certificate and CA expiry.
