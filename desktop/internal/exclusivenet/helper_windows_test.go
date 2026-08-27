//go:build windows

package exclusivenet

import "testing"

func TestValidateHelperArguments(t *testing.T) {
	validToken := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for _, test := range []struct {
		name    string
		address string
		token   string
		valid   bool
	}{
		{name: "loopback", address: "127.0.0.1:54321", token: validToken, valid: true},
		{name: "non loopback", address: "192.0.2.10:54321", token: validToken},
		{name: "ipv6", address: "[::1]:54321", token: validToken},
		{name: "short token", address: "127.0.0.1:54321", token: "abcd"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateHelperArguments(test.address, test.token)
			if test.valid && err != nil {
				t.Fatal(err)
			}
			if !test.valid && err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestRunHelperIgnoresNormalApplicationArguments(t *testing.T) {
	handled, code := runHelperIfRequested([]string{"--some-normal-argument"})
	if handled || code != 0 {
		t.Fatalf("normal application invocation was handled as helper: handled=%v code=%d", handled, code)
	}
}
