#!/bin/sh
# @authorship: AI-Assisted
# @architect: AI (Claude Opus 5.5)
#
# Removes nebula-gui from this Mac. The package installs it next to the
# service:
#
#   sudo /usr/local/libexec/nebula-gui/uninstall.sh [--remove-settings]
#
# It stops the service (which stops Nebula), quits the app, and removes
# the app, the service and each user's start-at-sign-in entry, which
# would point at the removed app. The imported config, the service's logs
# and the nebula-gui group are kept, so a reinstall picks up where it left
# off; --remove-settings removes them too. Each user's own app settings
# (in ~/Library) are left alone.

label=io.github.tucono.nebula-guid
group=nebula-gui
state="/Library/Application Support/nebula-gui"

usage() {
	echo "usage: sudo $0 [--remove-settings]" >&2
	exit 2
}

remove_settings=
case "${1:-}" in
"") ;;
--remove-settings) remove_settings=1 ;;
*) usage ;;
esac
[ $# -le 1 ] || usage
if [ "$(id -u)" != 0 ]; then
	echo "nebula-gui: run this with sudo" >&2
	exit 1
fi

# bootout waits for the service to exit.
launchctl bootout "system/$label" 2>/dev/null || true
pkill -x nebula-gui 2>/dev/null || true

rm -f "/Library/LaunchDaemons/$label.plist"
rm -rf /Applications/nebula-gui.app /usr/local/libexec/nebula-gui
rm -f /var/run/nebula-guid.sock /var/log/nebula-guid.log
for agent in /Users/*/Library/LaunchAgents/io.github.tucono.nebulagui.plist; do
	[ -f "$agent" ] && rm -f "$agent"
done
pkgutil --forget io.github.tucono.nebula-gui >/dev/null 2>&1 || true

if [ -n "$remove_settings" ]; then
	rm -rf "$state"
	if dseditgroup -o read "$group" >/dev/null 2>&1; then
		dseditgroup -o delete "$group" || echo "nebula-gui: couldn't remove the $group group" >&2
	fi
	echo "nebula-gui has been removed, with its settings."
else
	echo "nebula-gui has been removed. Its settings are kept in $state;"
	echo "run this again with --remove-settings to remove them too."
fi
