#!/usr/bin/env bash
# @authorship: AI-Assisted
# @architect: AI (Claude Opus 5.5)
#
# make-config.sh builds a single, ready-to-send config.yml for one device:
# the template with the CA certificate, the device's certificate and its key
# pasted in, so nebula-gui imports it without asking for any other file.
# It can also sign a new device certificate and key via nebula-cert, and
# appends each generated config to a TSV log (created_at, config, name,
# nebula_ip, expires, groups, fingerprint).
#
# Usage:
#   make-config.sh [-f] [-t TEMPLATE] [-o OUT] [-l LOG] CA_CRT HOST_CRT HOST_KEY
#   make-config.sh [-f] [-t TEMPLATE] [-o OUT] [-l LOG] -n NAME -i IP/CIDR [-k CA_KEY] [-g GROUPS] [-d DURATION] [-c CERT_DIR] [CA_CRT]
#
#   -n NAME      sign a new cert and key for NAME (requires -i)
#   -i IP/CIDR   Nebula IP/network in CIDR notation (e.g. 192.168.100.10/24)
#   -k CA_KEY    CA private key for signing (default: ca.key next to CA_CRT)
#   -g GROUPS    comma-separated groups for the new cert
#   -d DURATION  cert lifetime, e.g. 8760h (default: 1s before CA expires)
#   -c CERT_DIR  directory to write signed NAME.crt and NAME.key (default: .)
#   -t TEMPLATE  config without a pki section (default ./config-template.yml)
#   -o OUT       output file (default ./config-NAME.yml, NAME from cert)
#   -l LOG       append a TSV log entry to LOG (default ./make-config.log; - to disable)
#   -f           overwrite existing output (and signed cert/key) if present
#
# Examples, from the folder holding the CA:
#   make-config.sh ca.crt alice-laptop.crt alice-laptop.key
#   make-config.sh -n alice-laptop -i 192.168.100.10/24 -g family ca.crt
#
# If nebula-cert is on PATH (or NEBULA_CERT points to it), the certificate is
# checked against the CA and for expiry, and its details are recorded in LOG.
#
# The output holds a private key. It's written readable by you only; send
# it privately (in person, or an end-to-end encrypted messenger) and delete
# it once the device has imported it. See docs/network-setup.md.

set -euo pipefail

die() { echo "make-config: $*" >&2; exit 1; }
usage() {
	cat >&2 <<'EOF'
Usage:
  make-config.sh [-f] [-t TEMPLATE] [-o OUT] [-l LOG] CA_CRT HOST_CRT HOST_KEY
  make-config.sh [-f] [-t TEMPLATE] [-o OUT] [-l LOG] -n NAME -i IP/CIDR [-k CA_KEY] [-g GROUPS] [-d DURATION] [-c CERT_DIR] [CA_CRT]

  -n NAME      sign a new cert and key for NAME (requires -i)
  -i IP/CIDR   Nebula IP/network in CIDR notation (e.g. 192.168.100.10/24)
  -k CA_KEY    CA private key for signing (default: ca.key next to CA_CRT)
  -g GROUPS    comma-separated groups for the new cert
  -d DURATION  cert lifetime, e.g. 8760h (default: 1s before CA expires)
  -c CERT_DIR  directory to write signed NAME.crt and NAME.key (default: .)
  -t TEMPLATE  config without a pki section (default ./config-template.yml)
  -o OUT       output file (default ./config-NAME.yml, NAME from cert)
  -l LOG       append a TSV log entry to LOG (default ./make-config.log; - to disable)
  -f           overwrite existing output (and signed cert/key) if present
EOF
	exit 2
}

template=./config-template.yml
out=
log_file=./make-config.log
force=0
cert_name=
cert_net=
ca_key=
cert_groups=
cert_duration=
cert_dir=.

while getopts ":ft:o:l:n:i:k:g:d:c:h" opt; do
	case $opt in
		f) force=1 ;;
		t) template=$OPTARG ;;
		o) out=$OPTARG ;;
		l) log_file=$OPTARG ;;
		n) cert_name=$OPTARG ;;
		i) cert_net=$OPTARG ;;
		k) ca_key=$OPTARG ;;
		g) cert_groups=$OPTARG ;;
		d) cert_duration=$OPTARG ;;
		c) cert_dir=$OPTARG ;;
		h) usage ;;
		*) usage ;;
	esac
done
shift $((OPTIND - 1))

nebula_cert=${NEBULA_CERT:-$(command -v nebula-cert || true)}

sign_mode=0
if [[ -n $cert_name || -n $cert_net ]]; then
	[[ -n $cert_name && -n $cert_net ]] || die "-n and -i must be used together"
	[[ $# -le 1 ]] || usage
	ca=${1:-./ca.crt}
	[[ -n $ca_key ]] || ca_key=$(dirname "$ca")/ca.key
	sign_mode=1
else
	[[ -z $ca_key && -z $cert_groups && -z $cert_duration && $cert_dir == "." ]] ||
		die "-k, -g, -d, and -c require -n and -i"
	[[ $# -eq 3 ]] || usage
	ca=$1 crt=$2 key=$3
fi

# first_line FILE prints FILE's first non-empty line.
first_line() { grep -m1 -v '^[[:space:]]*$' "$1" || true; }

for f in "$template" "$ca"; do
	[[ -f $f && -r $f ]] || die "can't read $f"
done

[[ $(first_line "$ca") == "-----BEGIN NEBULA CERTIFICATE"* ]] ||
	die "$ca isn't a Nebula certificate (expected the CA's .crt)"

# The template must not bring its own pki section or turn on the SSH
# console; nebula-gui would refuse the latter.
if grep -qE '^pki[[:space:]]*:' "$template"; then
	die "$template has a pki section; remove it, this script adds it"
fi
if awk '
	/^[^[:space:]#]/ { in_sshd = ($0 ~ /^sshd[[:space:]]*:/) }
	in_sshd && /^[[:space:]]+enabled[[:space:]]*:[[:space:]]*(true|yes|on)/ { found = 1 }
	END { exit !found }
' "$template"; then
	die "$template turns on the SSH console (sshd.enabled); nebula-gui refuses that"
fi

if [[ $sign_mode -eq 1 ]]; then
	[[ -n $nebula_cert ]] || die "nebula-cert is required to sign certificates (put it on PATH or set NEBULA_CERT)"
	[[ -f $ca_key && -r $ca_key ]] || die "can't read $ca_key"
	[[ $cert_name != */* ]] || die "-n NAME must not contain '/'"

	if [[ -z $out ]]; then
		out=./config-${cert_name}.yml
	fi
	if [[ -e $out && $force -ne 1 ]]; then
		die "$out exists; use -f to overwrite"
	fi

	mkdir -p "$cert_dir"
	crt=$cert_dir/$cert_name.crt
	key=$cert_dir/$cert_name.key
	if [[ ( -e $crt || -e $key ) && $force -ne 1 ]]; then
		die "$crt or $key exists; use -f to overwrite"
	fi

	umask 077
	sign_dir=$(mktemp -d "$cert_dir/.make-config-sign.XXXXXX")
	trap 'rm -rf "$sign_dir"' EXIT

	sign_args=(
		-ca-crt "$ca"
		-ca-key "$ca_key"
		-name "$cert_name"
		-networks "$cert_net"
		-out-crt "$sign_dir/host.crt"
		-out-key "$sign_dir/host.key"
	)
	[[ -z $cert_groups ]] || sign_args+=(-groups "$cert_groups")
	[[ -z $cert_duration ]] || sign_args+=(-duration "$cert_duration")

	"$nebula_cert" sign "${sign_args[@]}"
	chmod 600 "$sign_dir/host.key"
	mv "$sign_dir/host.crt" "$crt"
	mv "$sign_dir/host.key" "$key"
	rmdir "$sign_dir"
	trap - EXIT
	echo "make-config: signed $crt and $key"
fi

for f in "$crt" "$key"; do
	[[ -f $f && -r $f ]] || die "can't read $f"
done

[[ $(first_line "$crt") == "-----BEGIN NEBULA CERTIFICATE"* ]] ||
	die "$crt isn't a Nebula certificate (expected the device's .crt)"

keyhead=$(first_line "$key")
case $keyhead in
	*ENCRYPTED*) die "$key is password-protected; nebula-gui needs an unprotected key" ;;
	*"NEBULA ED25519 "* | *"NEBULA ECDSA "*)
		die "$key looks like a CA signing key, not a device key. Never hand out the CA key" ;;
	"-----BEGIN NEBULA "*"PRIVATE KEY-----") ;;
	*) die "$key isn't a Nebula private key" ;;
esac
if [[ $(grep -c -- '-----BEGIN' "$key") -ne 1 ]]; then
	die "$key should hold exactly one key"
fi
if [[ $(grep -c -- '-----BEGIN' "$crt") -ne 1 ]]; then
	die "$crt should hold exactly one certificate"
fi

if [[ -n $nebula_cert ]]; then
	"$nebula_cert" verify -ca "$ca" -crt "$crt" >/dev/null 2>&1 ||
		die "$crt isn't valid for $ca (signed by another CA, or expired): run '$nebula_cert verify -ca $ca -crt $crt'"
else
	echo "make-config: nebula-cert not found, skipping the certificate check" >&2
fi

if [[ -z $out ]]; then
	name=$(basename "$crt")
	out=./config-${name%.crt}.yml
fi
if [[ -e $out && $force -ne 1 ]]; then
	die "$out exists; use -f to overwrite"
fi

indent() { sed -e 's/\r$//' -e '/^[[:space:]]*$/d' -e 's/^/    /' "$1"; }

umask 077
mkdir -p "$(dirname "$out")"
tmp=$(mktemp "$(dirname "$out")/.make-config.XXXXXX")
trap 'rm -f "$tmp"' EXIT
{
	echo "# Nebula config for $(basename "${crt%.crt}"), made $(date +%Y-%m-%d)."
	echo "# Import it in nebula-gui with \"Import config...\". It contains a private"
	echo "# key: don't share it, and delete it once it's imported."
	echo "pki:"
	echo "  ca: |"
	indent "$ca"
	echo "  cert: |"
	indent "$crt"
	echo "  key: |"
	indent "$key"
	echo
	# The template's leading comment block is notes for you, not the user.
	sed 's/\r$//' "$template" | awk 'body || !/^(#|[[:space:]]*$)/ { body = 1; print }'
} >"$tmp"
chmod 600 "$tmp"
mv "$tmp" "$out"
trap - EXIT
echo "make-config: wrote $out"

if [[ $log_file != "-" ]]; then
	created_at=$(date +%Y-%m-%dT%H:%M:%S%z)
	log_name=$(basename "${crt%.crt}")
	log_ip=${cert_net:-unknown}
	log_expires=unknown
	log_groups=${cert_groups:--}
	log_fp=unknown

	if [[ -n $nebula_cert ]]; then
		cert_info=$("$nebula_cert" print -path "$crt" | awk '
			function unquote(s) {
				gsub(/^[[:space:]]*"|"[[:space:],]*$/, "", s)
				return s
			}
			/"isCa":/ { next }
			/"name":/ { name = unquote(substr($0, index($0, ":") + 1)) }
			/"notAfter":/ { expires = unquote(substr($0, index($0, ":") + 1)) }
			/"fingerprint":/ { fp = unquote(substr($0, index($0, ":") + 1)) }
			/"networks": \[\]/ { next }
			/"networks": \[/ { in_net = 1; next }
			in_net && /\]/ { in_net = 0; next }
			in_net { v = unquote($0); if (v != "") nets = (nets ? nets "," : "") v }
			/"groups": \[\]/ { next }
			/"groups": \[/ { in_grp = 1; next }
			in_grp && /\]/ { in_grp = 0; next }
			in_grp { v = unquote($0); if (v != "") grps = (grps ? grps "," : "") v }
			END {
				printf "%s\t%s\t%s\t%s\t%s\n", name, nets, expires, (grps ? grps : "-"), fp
			}
		')
		IFS=$'\t' read -r parsed_name parsed_ip parsed_expires parsed_groups parsed_fp <<<"$cert_info"
		[[ -z $parsed_name ]] || log_name=$parsed_name
		[[ -z $parsed_ip ]] || log_ip=$parsed_ip
		[[ -z $parsed_expires ]] || log_expires=$parsed_expires
		[[ -z $parsed_groups ]] || log_groups=$parsed_groups
		[[ -z $parsed_fp ]] || log_fp=$parsed_fp
	fi

	mkdir -p "$(dirname "$log_file")"
	if [[ ! -s $log_file ]]; then
		printf 'created_at\tconfig\tname\tnebula_ip\texpires\tgroups\tfingerprint\n' >"$log_file"
		chmod 600 "$log_file"
	fi
	printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
		"$created_at" "$out" "$log_name" "$log_ip" "$log_expires" "$log_groups" "$log_fp" >>"$log_file"
	echo "make-config: logged to $log_file"
fi
