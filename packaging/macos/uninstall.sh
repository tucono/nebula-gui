#!/bin/sh
# @authorship: AI-Assisted
# @architect: AI (Claude Opus 5.5)
#
# Removes nebula-gui from this Mac. The package installs it next to the
# service, and the Uninstall nebula-gui app runs it:
#
#   sudo /usr/local/libexec/nebula-gui/uninstall.sh [--remove-everything]
#
# It stops the service (which stops Nebula), quits the app, and removes
# the app, the uninstaller app, the service and every user's
# start-at-sign-in entry, which would point at the removed app. The
# imported config, the service's logs, the nebula-gui group and each
# user's own files (saved peer names, app log, settings) are kept, so a
# reinstall picks up where it left off; --remove-everything removes them
# too, for every user. --remove-settings is its older name.

label=io.github.tucono.nebula-guid
group=nebula-gui
state="/Library/Application Support/nebula-gui"
libexec=/usr/local/libexec/nebula-gui

usage() {
	echo "usage: sudo $0 [--remove-everything]" >&2
	exit 2
}

everything=
case "${1:-}" in
"") ;;
--remove-everything | --remove-settings) everything=1 ;;
*) usage ;;
esac
[ $# -le 1 ] || usage
if [ "$(id -u)" != 0 ]; then
	echo "nebula-gui: run this with sudo" >&2
	exit 1
fi

# bootout waits for the service to exit. The app is quit before its
# users' files go, so it can't write its settings back.
launchctl bootout "system/$label" 2>/dev/null || true
pkill -x nebula-gui 2>/dev/null || true

# Every user's files, while nebula-guid is still here to name them. A
# failure is printed and the rest goes on.
if [ -n "$everything" ]; then
	"$libexec/nebula-guid" remove-user-files || true
else
	"$libexec/nebula-guid" remove-user-files --autostart-only || true
fi

rm -f "/Library/LaunchDaemons/$label.plist"
rm -rf /Applications/nebula-gui.app "/Applications/Uninstall nebula-gui.app" "$libexec"
rm -f /var/run/nebula-guid.sock /var/log/nebula-guid.log
pkgutil --forget io.github.tucono.nebula-gui >/dev/null 2>&1 || true

if [ -n "$everything" ]; then
	rm -rf "$state"
	if dseditgroup -o read "$group" >/dev/null 2>&1; then
		dseditgroup -o delete "$group" || echo "nebula-gui: couldn't remove the $group group" >&2
	fi
	echo "nebula-gui has been removed, with everything it saved."
else
	echo "nebula-gui has been removed. Its settings are kept for a reinstall,"
	echo "in $state and each user's Library."
fi
