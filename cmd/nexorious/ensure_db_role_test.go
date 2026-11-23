package main

import (
	"net/url"
	"testing"
)

func TestSplitAppAndAdminURL(t *testing.T) {
	// The Helm helper URL-encodes the password; it must come back decoded.
	target, adminURL, err := splitAppAndAdminURL(
		"postgresql://nexorious_app:p%40ss%3Aw%2Fd@db:5432/nexorious?sslmode=disable",
		"nexorious", "adm:in@pw")
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if target != (appTarget{role: "nexorious_app", password: "p@ss:w/d", database: "nexorious"}) {
		t.Fatalf("target = %+v", target)
	}
	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatalf("parse admin URL: %v", err)
	}
	pw, _ := u.User.Password()
	if u.User.Username() != "nexorious" || pw != "adm:in@pw" || u.Host != "db:5432" ||
		u.Path != "/nexorious" || u.RawQuery != "sslmode=disable" {
		t.Fatalf("admin URL = %s, want same host/db/query as the admin user", adminURL)
	}

	for _, bad := range []string{
		"postgresql://db:5432/nexorious", // no user
		"postgresql://app:pw@db:5432",    // no database
		"postgresql://app:pw@db:5432/",   // empty database
	} {
		if _, _, err := splitAppAndAdminURL(bad, "nexorious", "x"); err == nil {
			t.Errorf("splitAppAndAdminURL(%q) succeeded, want error", bad)
		}
	}
}
