// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package main

import (
	"strings"
	"testing"
)

func TestHasCapNetAdmin(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
		err    bool
	}{
		{"root", "Name:\tnebula-guid\nCapInh:\t0000000000000000\nCapEff:\t000001ffffffffff\n", true, false},
		{"systemd unit", "CapPrm:\t0000000000003000\nCapEff:\t0000000000003000\n", true, false},
		{"net_raw only", "CapEff:\t0000000000002000\n", false, false},
		{"ordinary user", "CapEff:\t0000000000000000\n", false, false},
		{"garbled", "CapEff:\tzz\n", false, true},
		{"missing", "Name:\tnebula-guid\n", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := hasCapNetAdmin(strings.NewReader(tc.status))
			if got != tc.want || (err != nil) != tc.err {
				t.Fatalf("got %v, %v; want %v, error %v", got, err, tc.want, tc.err)
			}
		})
	}
}
