#!/bin/sh
# @authorship: AI-Assisted
# @architect: AI (Claude Opus 5.5)
#
# Checks what the macOS uninstaller does with each user's own files, as
# root. CI runs it around each uninstall, for the user who ran sudo and
# for a second home folder in /Users that has no account behind it,
# which is all remove-user-files looks at.
#
#   check-uninstall.sh seed   make the files the app would leave, and
#                             another app's beside them
#   check-uninstall.sh kept   after an uninstall: only the LaunchAgents
#                             are gone
#   check-uninstall.sh gone   after --remove-everything: all are gone
#
# Another app's files must survive both.

second=/Users/nebula-gui-test
homes=$second
[ -n "${SUDO_USER:-}" ] && [ -d "/Users/$SUDO_USER" ] && homes="/Users/$SUDO_USER $homes"

# The names in internal/userfiles.
dirs="Library/Application Support/nebula-gui
Library/Preferences/fyne/io.github.tucono.nebulagui
Library/Caches/nebula-gui"
agent=Library/LaunchAgents/io.github.tucono.nebulagui.plist
others="Library/Application Support/other-app/keep.json
Library/Preferences/fyne/other-app/keep.json
Library/LaunchAgents/com.example.other-app.plist"

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
missing() { ! [ -e "$1" ] && ! [ -L "$1" ]; }
put() {
	mkdir -p "$(dirname "$1")" && echo '{}' >"$1"
}

case "${1:-}" in
seed | kept | gone) ;;
*)
	echo "usage: $0 seed|kept|gone" >&2
	exit 2
	;;
esac

# Word splitting on the homes only; the paths have spaces.
for home in $homes; do
	if [ "$1" = seed ]; then
		mkdir -p "$home"
		echo "$dirs" | while IFS= read -r d; do
			put "$home/$d/saved.json"
		done
		put "$home/$agent"
		echo "$others" | while IFS= read -r f; do
			put "$home/$f"
		done
		echo "seeded $home"
		continue
	fi
	# A pipe would run check in a subshell, losing failed.
	while IFS= read -r d; do
		if [ "$1" = kept ]; then
			check "$home/$d kept" test -f "$home/$d/saved.json"
		else
			check "$home/$d removed" missing "$home/$d"
		fi
	done <<-EOF
		$dirs
	EOF
	check "$home/$agent removed" missing "$home/$agent"
	while IFS= read -r f; do
		check "another app's $home/$f kept" test -f "$home/$f"
	done <<-EOF
		$others
	EOF
done

[ "$1" = seed ] && exit 0
if [ -n "$failed" ]; then
	exit 1
fi
echo "all checks passed ($1)"
