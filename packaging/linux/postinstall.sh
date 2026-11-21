#!/bin/sh
# @authorship: AI-Assisted
# @architect: AI (Claude Opus 5.5)
#
# Runs after the .deb or .rpm is unpacked. It creates the service user and
# the access group, gives the person installing access, and starts the
# service.
#
# Arguments: dpkg passes "configure <old version>", with no old version on
# a fresh install; rpm passes 1 on a fresh install and 2 on an upgrade.
# Installing again after a removal (not a purge) is fresh too, though dpkg
# passes the old version then: preremove.sh leaves a mark for it.

group=nebula-gui
user=nebula-guid
unit=nebula-guid

case "$1" in
configure) if [ -z "$2" ]; then fresh=1; else fresh=; fi ;;
1) fresh=1 ;;
2) fresh= ;;
*) exit 0 ;;
esac
removed=/var/lib/nebula-gui/.removed
if [ -e "$removed" ]; then
	fresh=1
	rm -f "$removed"
fi

create_users() {
	if command -v systemd-sysusers >/dev/null 2>&1; then
		systemd-sysusers /usr/lib/sysusers.d/nebula-gui.conf
		return
	fi
	getent group "$group" >/dev/null || groupadd --system "$group" || return
	getent passwd "$user" >/dev/null ||
		useradd --system --user-group --home-dir / --no-create-home \
			--shell /usr/sbin/nologin --comment "nebula-gui background service" "$user"
}

# installing_user prints the name of the person installing, when there's a
# way to tell, and nothing otherwise.
installing_user() {
	if [ -n "${SUDO_USER:-}" ] && [ "$SUDO_USER" != root ]; then
		echo "$SUDO_USER"
		return 0
	fi
	if [ -n "${PKEXEC_UID:-}" ] && [ "$PKEXEC_UID" != 0 ]; then
		getent passwd "$PKEXEC_UID" | cut -d: -f1
		return 0
	fi
	# A software centre installs through PackageKit, which doesn't say who
	# asked. The person signed in at the screen is the one double-clicking.
	command -v loginctl >/dev/null 2>&1 || return 0
	session=$(loginctl show-seat seat0 --property=ActiveSession --value 2>/dev/null)
	[ -n "$session" ] || return 0
	name=$(loginctl show-session "$session" --property=Name --value 2>/dev/null)
	if [ -n "$name" ] && [ "$name" != root ]; then
		echo "$name"
	fi
	return 0
}

if ! create_users; then
	echo "nebula-gui: couldn't create the $user user and $group group" >&2
	exit 1
fi

# Only a fresh install grants access: an upgrade is often run by someone
# else, who shouldn't gain it as a side effect.
if [ -n "$fresh" ]; then
	name=$(installing_user)
	if [ -n "$name" ] && getent passwd "$name" >/dev/null; then
		if usermod --append --groups "$group" "$name"; then
			echo "nebula-gui: added $name to the $group group. Restart the computer to use the app."
		else
			echo "nebula-gui: couldn't add $name to the $group group" >&2
		fi
	else
		echo "nebula-gui: add each person who'll use the app to the $group group:"
		echo "  sudo usermod -aG $group <name>"
	fi
fi

# Not under systemd (a container, say): the files are in place, and there's
# nothing to start.
[ -d /run/systemd/system ] || exit 0

systemctl daemon-reload || true
if [ -n "$fresh" ]; then
	systemctl enable --now "$unit.service" ||
		echo "nebula-gui: couldn't start the service; see: journalctl -u $unit" >&2
else
	# Runs the new version if the service was running; a service someone
	# stopped or disabled stays that way.
	systemctl try-restart "$unit.service" || true
fi
exit 0
