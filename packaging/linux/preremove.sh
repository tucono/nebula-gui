#!/bin/sh
# @authorship: AI-Assisted
# @architect: AI (Claude Opus 5.5)
#
# Runs before the package's files are removed. On removal (not upgrade) it
# stops and disables the service, which stops Nebula and removes its
# network interface.
#
# Arguments: dpkg passes "remove" on removal; rpm passes 0 on removal and 1
# on upgrade.

case "$1" in
remove | 0) ;;
*) exit 0 ;;
esac

if [ -d /run/systemd/system ]; then
	systemctl disable --now nebula-guid.service || true
fi
exit 0
