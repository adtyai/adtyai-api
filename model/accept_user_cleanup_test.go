package model

import "testing"

func TestValidateAcceptCleanupPrefixes(t *testing.T) {
	bad := [][]string{{}, {"accept-"}, {"accept"}, {"accept--"}, {"accept-rf"}, {"accept-%-"}, {"accept-rf-", "accept-"}, {"user-rf-"}}
	for _, u := range bad {
		if err := validateAcceptCleanupPrefixes(u, nil); err == nil {
			t.Errorf("user prefixes %q should be rejected", u)
		}
	}
	for _, o := range []string{"", "A-", "ACC", "ACCEPT-", "accept-", "----", "AB%-", "ORPHAN"} {
		if err := validateAcceptCleanupPrefixes([]string{"accept-rf-"}, []string{o}); err == nil {
			t.Errorf("order prefix %q should be rejected", o)
		}
	}
	if err := validateAcceptCleanupPrefixes([]string{"accept-dual-", "accept-settle-"}, []string{"ACCEPT-OV-", "ORPHAN-A5-"}); err != nil {
		t.Errorf("valid prefixes rejected: %v", err)
	}
}

func TestValidateAcceptCleanupPrefixesMore(t *testing.T) {
	for _, u := range []string{"ACCEPT-rf-", "accept-RF-", "accept-r_f-", "accept_rf-"} {
		if err := validateAcceptCleanupPrefixes([]string{u}, nil); err == nil {
			t.Errorf("user prefix %q should be rejected", u)
		}
	}
	for _, o := range []string{"Order-", "ORDER-", "TEST-", "Test-"} {
		if err := validateAcceptCleanupPrefixes([]string{"accept-rf-"}, []string{o}); err == nil {
			t.Errorf("order prefix %q should be rejected", o)
		}
	}
}
