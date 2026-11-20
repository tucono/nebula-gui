#!/bin/sh
# @authorship: AI-Assisted
# @architect: AI (Claude Opus 5.5)
#
# Runs after the package's files are removed, so systemd forgets the unit.
#
# A removal keeps the state directory (/var/lib/nebula-gui: the imported
# config, settings and logs) and each user's own files, so a reinstall
# picks up where it left off. A purge (apt purge) removes everything: the
# state directory, every user's files, and the nebula-guid user and
# nebula-gui group. rpm has no purge; on Fedora, nebula-guid
# remove-everything does the same before dnf remove.
#
# Arguments: dpkg passes "remove" or "purge"; rpm passes 0 on removal.

# The Linux paths in internal/userfiles, relative to a home folder. The
# binaries are gone by now, so nebula-guid can't be asked; a Go test
# checks this list against userfiles.For.
user_paths=".config/nebula-gui
.config/fyne/io.github.tucono.nebulagui
.cache/nebula-gui
.config/autostart/nebula-gui.desktop"

case "$1" in
remove | purge | 0) ;;
*) exit 0 ;;
esac

if [ -d /run/systemd/system ]; then
	systemctl daemon-reload || true
fi
[ "$1" = purge ] || exit 0

rm -rf /var/lib/nebula-gui

# Each person's account, as nebula-guid remove-user-files lists them: a UID
# from 1000, not nobody, and a home folder. Their files are removed as
# them, so a symlink in their home folder can't lead root elsewhere.
getent passwd | while IFS=: read -r name _ uid _ _ home _; do
	case "$uid" in '' | *[!0-9]*) continue ;; esac
	[ "$uid" -ge 1000 ] && [ "$uid" -ne 65534 ] || continue
	case "$home" in /?*) ;; *) continue ;; esac
	[ -d "$home" ] || continue
	echo "$user_paths" | while IFS= read -r p; do
		[ -e "$home/$p" ] || [ -L "$home/$p" ] || continue
		runuser -u "$name" -- rm -rf -- "$home/$p" ||
			echo "nebula-gui: couldn't remove $home/$p" >&2
	done
done

if getent passwd nebula-guid >/dev/null; then
	userdel nebula-guid || echo "nebula-gui: couldn't remove the nebula-guid user" >&2
fi
for g in nebula-gui nebula-guid; do
	if getent group "$g" >/dev/null; then
		groupdel "$g" || echo "nebula-gui: couldn't remove the $g group" >&2
	fi
done
exit 0
