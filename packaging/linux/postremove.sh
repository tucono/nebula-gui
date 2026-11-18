#!/bin/sh
# @authorship: AI-Assisted
# @architect: AI (Claude Opus 5.5)
#
# Runs after the package's files are removed, so systemd forgets the unit.
#
# The state directory (/var/lib/nebula-gui: the imported config, settings
# and logs) is kept, even on purge, so a reinstall picks up where it left
# off; delete it by hand to remove it. The nebula-guid user and nebula-gui
# group are kept too, as is usual for system users.
#
# Arguments: dpkg passes "remove" or "purge"; rpm passes 0 on removal.

case "$1" in
remove | purge | 0) ;;
*) exit 0 ;;
esac

if [ -d /run/systemd/system ]; then
	systemctl daemon-reload || true
fi
exit 0
