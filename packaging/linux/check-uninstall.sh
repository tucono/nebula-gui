#!/bin/sh
# @authorship: AI-Assisted
# @architect: AI (Claude Opus 5.5)
#
# Checks what removing the .deb or .rpm does with each user's own files,
# as root. CI runs it around each removal, for two test users it creates,
# alice and bob.
#
#   check-uninstall.sh seed   create the users, the files the app would
#                             leave, and another app's beside them
#   check-uninstall.sh kept   after apt remove: only the autostart files
#                             are gone
#   check-uninstall.sh gone   after apt purge or nebula-guid
#                             remove-everything: all are gone, with the
#                             state directory, user and group
#
# bob's ~/.cache is a symlink to a folder outside his home, holding a
# nebula-gui folder of root's. Removing his files must not follow it.

users="alice bob"
trap_dir=/var/tmp/nebula-gui-trap

# The Linux names in internal/userfiles.
dirs=".config/nebula-gui
.config/fyne/io.github.tucono.nebulagui
.cache/nebula-gui"
autostart=.config/autostart/nebula-gui.desktop
others=".config/other-app/keep.json
.config/fyne/other-app/keep.json
.config/autostart/other-app.desktop
.cache/other-app/keep.json"

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
no_user() { ! getent passwd "$1" >/dev/null; }
no_group() { ! getent group "$1" >/dev/null; }

# seed_user makes u's files as u, as the app would.
seed_user() {
	u=$1
	getent passwd "$u" >/dev/null || useradd --create-home --shell /bin/sh "$u" || exit 1
	home=$(getent passwd "$u" | cut -d: -f6)
	if [ "$u" = bob ]; then
		mkdir -p "$trap_dir/nebula-gui"
		echo '{}' >"$trap_dir/nebula-gui/keep.json"
		chmod 1777 "$trap_dir"
		rm -rf "$home/.cache"
		ln -s "$trap_dir" "$home/.cache"
	fi
	runuser -u "$u" -- sh -c '
		cd "$HOME" || exit 1
		put() { mkdir -p "$(dirname "$1")" && echo "{}" >"$1"; }
		echo "$1" | while IFS= read -r d; do
			# bob'"'"'s cache folder is root'"'"'s, through the symlink.
			[ -L .cache ] && [ "$d" = .cache/nebula-gui ] && continue
			put "$d/saved.json"
		done
		put "$2"
		echo "$3" | while IFS= read -r f; do put "$f"; done
	' sh "$dirs" "$autostart" "$others" || exit 1
	echo "seeded $home"
}

case "${1:-}" in
seed)
	for u in $users; do seed_user "$u"; done
	exit 0
	;;
kept | gone) ;;
*)
	echo "usage: $0 seed|kept|gone" >&2
	exit 2
	;;
esac

for u in $users; do
	home=$(getent passwd "$u" | cut -d: -f6)
	# A pipe would run check in a subshell, losing failed.
	while IFS= read -r d; do
		[ "$u" = bob ] && [ "$d" = .cache/nebula-gui ] && continue
		if [ "$1" = kept ]; then
			check "$home/$d kept" test -f "$home/$d/saved.json"
		else
			check "$home/$d removed" missing "$home/$d"
		fi
	done <<-EOF
		$dirs
	EOF
	check "$home/$autostart removed" missing "$home/$autostart"
	while IFS= read -r f; do
		check "another app's $home/$f kept" test -f "$home/$f"
	done <<-EOF
		$others
	EOF
done
check "a folder behind a symlink in bob's home kept" test -f "$trap_dir/nebula-gui/keep.json"

if [ "$1" = gone ]; then
	check "state directory removed" missing /var/lib/nebula-gui
	check "nebula-guid user removed" no_user nebula-guid
	check "nebula-gui group removed" no_group nebula-gui
	check "nebula-guid group removed" no_group nebula-guid
fi

if [ -n "$failed" ]; then
	exit 1
fi
echo "all checks passed ($1)"
