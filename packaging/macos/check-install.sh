#!/bin/sh
# @authorship: AI-Assisted
# @architect: AI (Claude Opus 5.5)
#
# Checks what the macOS installer left on this Mac, as root. CI runs it
# after installing the .pkg and after each uninstall.
#
#   check-install.sh installed   the service runs and answers
#   check-install.sh removed     uninstalled; settings and group kept
#   check-install.sh purged      uninstalled with --remove-settings

label=io.github.tucono.nebula-guid
plist=/Library/LaunchDaemons/$label.plist
app=/Applications/nebula-gui.app
libexec=/usr/local/libexec/nebula-gui
sock=/var/run/nebula-guid.sock
state="/Library/Application Support/nebula-gui"
group=nebula-gui

failed=
check() {
	what=$1
	shift
	if "$@"; then
		echo "ok:   $what"
	else
		echo "FAIL: $what"
		failed=1
	fi
}
missing() { ! [ -e "$1" ]; }
has_group() { dseditgroup -o read "$group" >/dev/null 2>&1; }
no_group() { ! has_group; }
universal() {
	archs=$(lipo -archs "$1" 2>/dev/null)
	echo "      $1: $archs"
	case " $archs " in *" x86_64 "*) ;; *) return 1 ;; esac
	case " $archs " in *" arm64 "*) ;; *) return 1 ;; esac
}
mode_owner() {
	got=$(stat -f '%Lp %Su:%Sg' "$1" 2>/dev/null)
	echo "      $1: $got"
	[ "$got" = "$2" ]
}
job_running() { launchctl print "system/$label" 2>/dev/null | grep -q 'state = running'; }
no_job() { ! launchctl print "system/$label" >/dev/null 2>&1; }
not_running() { ! pgrep -x nebula-guid >/dev/null; }
no_receipt() { ! pkgutil --pkg-info io.github.tucono.nebula-gui >/dev/null 2>&1; }

# status [user] prints the HTTP status of GET /v1/status over the socket,
# asked as root or as user.
status() {
	if [ -n "${1:-}" ]; then
		sudo -u "$1" curl -s -o /dev/null -w '%{http_code}' --unix-socket "$sock" http://nebula-guid/v1/status
	else
		curl -s -o /dev/null -w '%{http_code}' --unix-socket "$sock" http://nebula-guid/v1/status
	fi
}
answers() { [ "$(status "$1")" = 200 ]; }
refuses() { [ "$(status "$1")" != 200 ]; }

case "${1:-}" in
installed)
	check "app installed" test -x "$app/Contents/MacOS/nebula-gui"
	check "app is universal" universal "$app/Contents/MacOS/nebula-gui"
	check "app signature" codesign --verify "$app"
	check "service installed" test -x "$libexec/nebula-guid"
	check "service is universal" universal "$libexec/nebula-guid"
	check "uninstall script installed" test -x "$libexec/uninstall.sh"
	check "third-party notices" grep -q '^github.com/slackhq/nebula ' "$app/Contents/Resources/THIRD_PARTY_NOTICES.txt"
	echo "      version: $("$libexec/nebula-guid" --version)"
	check "launch daemon" mode_owner "$plist" "644 root:wheel"
	check "package receipt" pkgutil --pkg-info io.github.tucono.nebula-gui

	# The postinstall script started it; give it a moment to listen.
	i=0
	while [ $i -lt 20 ] && ! answers ""; do
		sleep 0.5
		i=$((i + 1))
	done
	check "service running" job_running
	check "socket" mode_owner "$sock" "660 root:$group"
	check "API answers root" answers ""
	check "state directory" test -d "$state"
	check "state directory is private" sh -c "stat -f '%Lp %Su' '$state' | grep -qx '700 root'"

	check "$group group exists" has_group
	console=$(stat -f%Su /dev/console)
	case "$console" in
	"" | root | loginwindow | _mbsetupuser) echo "      no one at the screen ($console); no member to check" ;;
	*)
		check "$console added to $group" dseditgroup -o checkmember -m "$console" "$group"
		check "API answers $console" answers "$console"
		;;
	esac
	check "API refuses nobody" refuses nobody
	;;
removed | purged)
	check "service stopped" no_job
	check "nebula-guid not running" not_running
	check "launch daemon removed" missing "$plist"
	check "app removed" missing "$app"
	check "service files removed" missing "$libexec"
	check "socket removed" missing "$sock"
	check "package receipt removed" no_receipt
	if [ "$1" = removed ]; then
		check "state directory kept" test -d "$state"
		check "$group group kept" has_group
	else
		check "state directory removed" missing "$state"
		check "$group group removed" no_group
	fi
	;;
*)
	echo "usage: $0 installed|removed|purged" >&2
	exit 2
	;;
esac

if [ -n "$failed" ]; then
	exit 1
fi
echo "all checks passed ($1)"
