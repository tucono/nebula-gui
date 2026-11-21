// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// capNetAdmin is CAP_NET_ADMIN's bit in the capability sets.
const capNetAdmin = 12

// errNoNetAdmin explains how the service is meant to run.
var errNoNetAdmin = errors.New("needs CAP_NET_ADMIN to create the Nebula network interface. " +
	"Install the package, which runs it as a system service, or run it as root")

// checkNetAdmin fails when the process lacks CAP_NET_ADMIN in its
// effective set. Root normally has it; the systemd unit grants it to the
// service user.
func checkNetAdmin() error {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		// Can't tell; let Nebula report it on connect.
		return nil
	}
	defer func() { _ = f.Close() }()
	ok, err := hasCapNetAdmin(f)
	if err != nil || ok {
		return nil
	}
	return errNoNetAdmin
}

// hasCapNetAdmin reads the CapEff line of a /proc/<pid>/status file.
func hasCapNetAdmin(status io.Reader) (bool, error) {
	sc := bufio.NewScanner(status)
	for sc.Scan() {
		v, ok := strings.CutPrefix(sc.Text(), "CapEff:")
		if !ok {
			continue
		}
		caps, err := strconv.ParseUint(strings.TrimSpace(v), 16, 64)
		if err != nil {
			return false, fmt.Errorf("parse CapEff: %w", err)
		}
		return caps&(1<<capNetAdmin) != 0, nil
	}
	if err := sc.Err(); err != nil {
		return false, err
	}
	return false, errors.New("no CapEff line")
}
