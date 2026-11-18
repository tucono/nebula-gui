#!/bin/sh
# @authorship: AI-Assisted
# @architect: AI (Claude Opus 5.5)
#
# Checks an installed .deb or .rpm, as root. CI runs it after installing
# the package.
#
#   check-install.sh [--running <user>]
#
# Without arguments it checks the files, their modes, and the service user
# and group, which is all a container without systemd can show. With
# --running it also checks that systemd started the service as its own
# user, that the socket has the right owner and mode, that <user> (who ran
# sudo to install) was added to the group, and that the API answers that
# user.

failed=
fail() {
	echo "FAIL: $*" >&2
	failed=1
}

# mode_owner <path> <mode> <owner:group>
mode_owner() {
	got=$(stat -c '%a %U:%G' "$1" 2>/dev/null) || {
		fail "$1 is missing"
		return
	}
	[ "$got" = "$2 $3" ] || fail "$1 is $got, want $2 $3"
}

mode_owner /usr/bin/nebula-gui 755 root:root
mode_owner /usr/libexec/nebula-gui/nebula-guid 755 root:root
mode_owner /usr/lib/systemd/system/nebula-guid.service 644 root:root
mode_owner /usr/lib/sysusers.d/nebula-gui.conf 644 root:root
mode_owner /usr/share/applications/io.github.tucono.nebulagui.desktop 644 root:root
mode_owner /usr/share/icons/hicolor/256x256/apps/io.github.tucono.nebulagui.png 644 root:root
mode_owner /usr/share/icons/hicolor/scalable/apps/io.github.tucono.nebulagui.svg 644 root:root

getent group nebula-gui >/dev/null || fail "no nebula-gui group"
if entry=$(getent passwd nebula-guid); then
	shell=$(echo "$entry" | cut -d: -f7)
	case "$shell" in
	*/nologin | */false) ;;
	*) fail "nebula-guid has a login shell: $shell" ;;
	esac
	uid=$(echo "$entry" | cut -d: -f3)
	[ "$uid" -lt 1000 ] || fail "nebula-guid isn't a system user (uid $uid)"
else
	fail "no nebula-guid user"
fi

/usr/libexec/nebula-gui/nebula-guid --version >/dev/null || fail "nebula-guid --version"

if [ "$1" = --running ]; then
	installer=$2
	systemctl is-enabled --quiet nebula-guid.service || fail "service isn't enabled"
	systemctl is-active --quiet nebula-guid.service || {
		fail "service isn't running"
		journalctl -u nebula-guid.service --no-pager | tail -20 >&2
	}
	user=$(ps -o user= -p "$(systemctl show -p MainPID --value nebula-guid.service)")
	[ "$user" = nebula-guid ] || fail "service runs as $user, want nebula-guid"
	mode_owner /run/nebula-gui/nebula-guid.sock 660 nebula-guid:nebula-gui
	mode_owner /var/lib/nebula-gui 700 nebula-guid:nebula-guid
	systemd-analyze verify /usr/lib/systemd/system/nebula-guid.service || fail "systemd-analyze verify"

	id -nG "$installer" | tr ' ' '\n' | grep -qx nebula-gui || fail "$installer wasn't added to nebula-gui"
	# The installer's session predates the group, so ask as a fresh login
	# would see them.
	out=$(setpriv --reuid="$installer" --regid="$installer" --init-groups \
		curl -sS --unix-socket /run/nebula-gui/nebula-guid.sock http://nebula-guid/v1/status) ||
		fail "API didn't answer $installer"
	echo "status: $out"
	# And someone outside the group gets nothing.
	if setpriv --reuid=nobody --regid=nogroup --clear-groups \
		curl -sS --unix-socket /run/nebula-gui/nebula-guid.sock http://nebula-guid/v1/status >/dev/null 2>&1; then
		fail "API answered a user outside nebula-gui"
	fi
fi

if [ -n "$failed" ]; then
	exit 1
fi
echo "install check passed"
