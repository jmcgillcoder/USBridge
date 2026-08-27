package routing

import "testing"

func TestParseIPMode(t *testing.T) {
	for input, want := range map[string]IPMode{
		"auto": IPModeAuto,
		"IPv4": IPModeIPv4,
		"v6":   IPModeIPv6,
	} {
		got, err := ParseIPMode(input)
		if err != nil {
			t.Fatalf("ParseIPMode(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseIPMode(%q) = %v, want %v", input, got, want)
		}
	}
	if _, err := ParseIPMode("ipx"); err == nil {
		t.Fatal("ParseIPMode(ipx) unexpectedly succeeded")
	}
}
