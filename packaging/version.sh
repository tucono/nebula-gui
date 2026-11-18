#!/bin/sh
# @authorship: AI-Assisted
# @architect: AI (Claude Opus 5.5)
#
# Prints the release version for the binaries and packages.
#
#   packaging/version.sh            semantic version, for the binaries
#   packaging/version.sh --package  the same, with "~" before a pre-release,
#                                   so dpkg and rpm sort it before the release
#
# A commit tagged v0.1.0 gives 0.1.0, and v0.1.0-rc.1 gives 0.1.0-rc.1
# (0.1.0~rc.1 for packages). Other commits give 0.0.0-dev.<commits>.<hash>,
# with .dirty added when the tree has changes; those aren't released.

set -e

if tag=$(git describe --tags --exact-match --match 'v[0-9]*' 2>/dev/null); then
	v=${tag#v}
else
	v="0.0.0-dev.$(git rev-list --count HEAD).$(git rev-parse --short HEAD)"
	git diff --quiet HEAD -- 2>/dev/null || v="$v.dirty"
fi

if [ "$1" = --package ]; then
	# Only the first "-" starts the pre-release; later ones belong to it.
	v=$(echo "$v" | sed 's/-/~/')
fi
echo "$v"
