#!/bin/sh
# @authorship: AI-Assisted
# @architect: AI (Claude Opus 5.5)
#
# Runs before the package's files are removed. On removal (not upgrade) it
# quits the app in every session, removes every user's start-at-sign-in
# entry, which would point at the removed app, and stops and disables the
# service, which stops Nebula and removes its network interface. Each
# user's own files are kept, unless a purge removes them (postremove.sh).
#
# Arguments: dpkg passes "remove" on removal; rpm passes 0 on removal and 1
# on upgrade.

case "$1" in
remove | 0) ;;
*) exit 0 ;;
esac

# Quit first, so the app can't write its settings back.
pkill -x nebula-gui 2>/dev/null || true
/usr/libexec/nebula-gui/nebula-guid remove-user-files --autostart-only || true

if [ -d /run/systemd/system ]; then
	systemctl disable --now nebula-guid.service || true
fi
exit 0
