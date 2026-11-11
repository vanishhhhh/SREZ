package config

import (
	"os"
	"testing"
)

func TestHTTPAddr(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value string
		unset bool
		want  string
	}{
		{name: "unset uses loopback", unset: true, want: "127.0.0.1:8080"},
		{name: "empty uses loopback", want: "127.0.0.1:8080"},
		{name: "explicit loopback port", value: "127.0.0.1:8081", want: "127.0.0.1:8081"},
		{name: "container listens on all interfaces", value: ":8080", want: ":8080"},
		{name: "explicit IPv6 loopback", value: "[::1]:8081", want: "[::1]:8081"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HTTP_ADDR", tt.value)
			if tt.unset {
				if err := os.Unsetenv("HTTP_ADDR"); err != nil {
					t.Fatal(err)
				}
			}
			if got := Must().Addr; got != tt.want {
				t.Fatalf("HTTP address = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDevCatalogWriteIsExplicit(t *testing.T) {
	for _, value := range []string{"", "false", "TRUE", "1", " true ", "true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("SREZ_DEV_CATALOG_WRITE", value)
			if got := Must().DevCatalogWrite; got != (value == "true") {
				t.Fatalf("flag %q enabled=%v", value, got)
			}
		})
	}
}
