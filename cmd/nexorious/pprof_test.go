package main

import "testing"

func TestPprofAddrIsLoopback(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		want    bool
		wantErr bool
	}{
		{name: "ipv4 loopback", addr: "127.0.0.1:6060", want: true},
		{name: "ipv4 loopback other", addr: "127.0.0.5:6060", want: true},
		{name: "ipv6 loopback", addr: "[::1]:6060", want: true},
		{name: "all interfaces v4", addr: "0.0.0.0:6060", want: false},
		{name: "all interfaces v6", addr: "[::]:6060", want: false},
		{name: "private lan", addr: "192.168.1.10:6060", want: false},
		{name: "empty host binds all", addr: ":6060", want: false},
		{name: "missing port", addr: "127.0.0.1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pprofAddrIsLoopback(tt.addr)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("pprofAddrIsLoopback(%q) expected error, got nil", tt.addr)
				}
				return
			}
			if err != nil {
				t.Fatalf("pprofAddrIsLoopback(%q) unexpected error: %v", tt.addr, err)
			}
			if got != tt.want {
				t.Errorf("pprofAddrIsLoopback(%q) = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}
