#!/usr/bin/env bash
# @authorship: AI-Assisted
# @architect: AI (Claude Opus 5.5)
#
# Local Nebula test lab: a lighthouse and two hosts (alice, bob) running as
# plain processes on 127.0.0.1 with the tun device disabled, so no root or
# Docker is needed. A disabled tun still answers ICMP echo, so the dev
# service can ping alice and bob over the overlay.
#
#   lab.sh up              build nebula, create the PKI and configs, start the nodes
#   lab.sh down            stop the nodes
#   lab.sh status          show which nodes are running
#   lab.sh import SOCKET   PUT testlab/dev/config.yml to the service on SOCKET
#   lab.sh broken          write bad configs to testlab/dev/broken/ for the GUI import
#   lab.sh clean           stop the nodes and delete all generated state, PKI included
#
# Everything generated (pki/, run/, dev/) is gitignored. Never commit it.
#
# Overlay 192.168.100.0/24:
#   lighthouse 192.168.100.1   127.0.0.1:4301
#   alice      192.168.100.10  127.0.0.1:4302
#   bob        192.168.100.11  127.0.0.1:4303
#   dev        192.168.100.20  (the nebula-guid under test, real tun)

set -euo pipefail

LAB="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(dirname "$LAB")"
PKI="$LAB/pki"
RUN="$LAB/run"
DEV="$LAB/dev"
BIN="$RUN/bin"
NODES="lighthouse alice bob"
LH_IP=192.168.100.1
LH_PORT=4301

# No associative arrays: macOS ships bash 3.2.
ip_of() {
	case $1 in
		lighthouse) echo 192.168.100.1 ;;
		alice) echo 192.168.100.10 ;;
		bob) echo 192.168.100.11 ;;
		dev) echo 192.168.100.20 ;;
	esac
}
port_of() {
	case $1 in
		lighthouse) echo 4301 ;;
		alice) echo 4302 ;;
		bob) echo 4303 ;;
	esac
}

die() { echo "lab: $*" >&2; exit 1; }

build() {
	mkdir -p "$BIN"
	# The tool directives in go.mod pin these to the embedded Nebula version.
	(cd "$ROOT" && go build -o "$BIN/" github.com/slackhq/nebula/cmd/nebula github.com/slackhq/nebula/cmd/nebula-cert)
}

make_pki() {
	[[ -f "$PKI/ca.crt" ]] && return
	mkdir -p "$PKI"
	chmod 700 "$PKI"
	"$BIN/nebula-cert" ca -name "nebula-gui test CA" -duration 8760h \
		-out-crt "$PKI/ca.crt" -out-key "$PKI/ca.key"
	local n groups
	for n in lighthouse alice bob dev; do
		groups=""
		[[ $n == lighthouse ]] || groups="-groups users"
		# shellcheck disable=SC2086
		"$BIN/nebula-cert" sign -name "$n" -networks "$(ip_of "$n")/24" $groups \
			-ca-crt "$PKI/ca.crt" -ca-key "$PKI/ca.key" \
			-out-crt "$PKI/$n.crt" -out-key "$PKI/$n.key"
	done
	echo "lab: created test PKI in $PKI"
}

firewall() {
	cat <<'EOF'
firewall:
  outbound:
    - port: any
      proto: any
      host: any
  inbound:
    - proto: icmp
      host: any
EOF
}

# node_config NAME writes the config for a lab node. PKI paths are absolute:
# the lab nodes are stock nebula, which reads them as files.
node_config() {
	local n=$1
	{
		cat <<EOF
pki:
  ca: $PKI/ca.crt
  cert: $PKI/$n.crt
  key: $PKI/$n.key
listen:
  host: 127.0.0.1
  port: $(port_of "$n")
tun:
  disabled: true
logging:
  level: info
EOF
		if [[ $n == lighthouse ]]; then
			printf 'lighthouse:\n  am_lighthouse: true\n'
		else
			cat <<EOF
static_host_map:
  "$LH_IP": ["127.0.0.1:$LH_PORT"]
lighthouse:
  hosts:
    - $LH_IP
EOF
		fi
		firewall
	} >"$RUN/$n.yml"
}

# indent_file FILE prints FILE indented for a YAML block scalar.
indent_file() { sed 's/^/    /' "$1"; }

# dev_rest prints the dev config after its pki section.
dev_rest() {
	cat <<EOF
static_host_map:
  "$LH_IP": ["127.0.0.1:$LH_PORT"]
lighthouse:
  hosts:
    - $LH_IP
listen:
  host: 0.0.0.0
  port: 0
EOF
	firewall
}

# dev_config writes the config for the service under test twice: with the
# PKI inlined as the service requires (config.yml, for lab.sh import), and
# with the PKI as paths relative to the config (config.paths.yml, for the
# GUI's import, which inlines them).
dev_config() {
	mkdir -p "$DEV"
	chmod 700 "$DEV"
	{
		echo "pki:"
		echo "  ca: |"
		indent_file "$PKI/ca.crt"
		echo "  cert: |"
		indent_file "$PKI/dev.crt"
		echo "  key: |"
		indent_file "$PKI/dev.key"
		dev_rest
	} >"$DEV/config.yml"
	paths_config ../pki/ca.crt ../pki/dev.crt ../pki/dev.key >"$DEV/config.paths.yml"
	chmod 600 "$DEV/config.yml" "$DEV/config.paths.yml"
}

# paths_config CA CERT KEY prints a dev config with pki paths.
paths_config() {
	cat <<EOF
# nebula-gui test lab: dev host, pki as file paths
pki:
  ca: $1
  cert: $2
  key: $3
EOF
	dev_rest
}

# broken writes configs the GUI import should refuse, each with its own
# message, into $DEV/broken/. Paths are relative to that directory.
broken() {
	[[ -f $PKI/ca.crt ]] || die "no PKI; run lab.sh up first"
	build
	local B="$DEV/broken"
	rm -rf "$B"
	mkdir -p "$B"
	chmod 700 "$B"

	# A cert that has already expired: nebula-cert can't backdate, so
	# sign one valid for a second and wait it out.
	"$BIN/nebula-cert" sign -name dev -networks "$(ip_of dev)/24" -groups users -duration 1s \
		-ca-crt "$PKI/ca.crt" -ca-key "$PKI/ca.key" \
		-out-crt "$B/expired.crt" -out-key "$B/expired.key"
	paths_config ../../pki/ca.crt expired.crt expired.key >"$B/expired.yml"

	# dev's cert with alice's key.
	paths_config ../../pki/ca.crt ../../pki/dev.crt ../../pki/alice.key >"$B/key-mismatch.yml"

	# A key path from the admin's machine.
	paths_config ../../pki/ca.crt ../../pki/dev.crt /etc/nebula/laptop.key >"$B/missing-key.yml"

	# The sshd debug console turned on.
	{
		paths_config ../../pki/ca.crt ../../pki/dev.crt ../../pki/dev.key
		cat <<'EOF'
sshd:
  enabled: true
  listen: 127.0.0.1:2222
EOF
	} >"$B/sshd.yml"

	# Not a config at all.
	echo "Remember to water the plants." >"$B/notes.txt"

	chmod 600 "$B"/*
	sleep 2 # let expired.crt expire
	echo "lab: wrote broken configs to $B:"
	(cd "$B" && ls -1 ./*.yml ./*.txt)
}

pidfile() { echo "$RUN/$1.pid"; }

running() {
	local f
	f=$(pidfile "$1")
	[[ -f $f ]] && kill -0 "$(cat "$f")" 2>/dev/null
}

up() {
	build
	make_pki
	mkdir -p "$RUN"
	local n
	for n in $NODES; do
		node_config "$n"
		if running "$n"; then
			echo "lab: $n already running"
			continue
		fi
		nohup "$BIN/nebula" -config "$RUN/$n.yml" >"$RUN/$n.log" 2>&1 &
		echo $! >"$(pidfile "$n")"
	done
	dev_config
	sleep 1
	status
	cat <<EOF

Dev config (inline PKI): $DEV/config.yml
Dev config (PKI paths):  $DEV/config.paths.yml   (for the GUI's Import config…)
Import it:  testlab/lab.sh import \$XDG_RUNTIME_DIR/nebula-guid.sock
Node logs:  $RUN/<node>.log
EOF
}

down() {
	local n f
	for n in $NODES; do
		f=$(pidfile "$n")
		if running "$n"; then
			kill "$(cat "$f")"
			echo "lab: stopped $n"
		fi
		rm -f "$f"
	done
}

status() {
	local n
	for n in $NODES; do
		if running "$n"; then
			printf '%-11s up    %-15s 127.0.0.1:%s\n' "$n" "$(ip_of "$n")" "$(port_of "$n")"
		else
			printf '%-11s down  (see %s)\n' "$n" "$RUN/$n.log"
		fi
	done
}

import() {
	local sock=${1:-}
	[[ -n $sock ]] || die "usage: lab.sh import SOCKET"
	[[ -f $DEV/config.yml ]] || die "no dev config; run lab.sh up first"
	local body
	if command -v jq >/dev/null; then
		body=$(jq -Rs '{yaml: .}' "$DEV/config.yml")
	elif command -v python3 >/dev/null; then
		body=$(python3 -c 'import json,sys; print(json.dumps({"yaml": open(sys.argv[1]).read()}))' "$DEV/config.yml")
	else
		die "need jq or python3 to build the request"
	fi
	curl -sS --unix-socket "$sock" -X PUT -H 'Content-Type: application/json' \
		--data-binary @- http://nebula-guid/v1/config <<<"$body"
	echo
}

clean() {
	down
	rm -rf "$RUN" "$DEV" "$PKI"
	echo "lab: removed generated state"
}

case "${1:-}" in
	up) up ;;
	down) down ;;
	status) status ;;
	import) import "${2:-}" ;;
	broken) broken ;;
	clean) clean ;;
	*)
		echo "usage: lab.sh up | down | status | import SOCKET | broken | clean" >&2
		exit 2
		;;
esac
