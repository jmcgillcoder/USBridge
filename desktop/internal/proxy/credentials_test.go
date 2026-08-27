package proxy

import (
	"strings"
	"testing"
)

func TestCredentialsValidationAndMatching(t *testing.T) {
	credentials := Credentials{Username: "usbridge", Password: "secret:with-colon"}
	if err := credentials.Validate(); err != nil {
		t.Fatal(err)
	}
	if !credentials.Matches("usbridge", "secret:with-colon") {
		t.Fatal("valid credentials did not match")
	}
	if credentials.Matches("USBridge", "secret:with-colon") || credentials.Matches("usbridge", "wrong") {
		t.Fatal("invalid credentials matched")
	}

	invalid := []Credentials{
		{},
		{Username: "usbridge"},
		{Username: "user:name", Password: "secret"},
		{Username: strings.Repeat("u", 256), Password: "secret"},
		{Username: "usbridge", Password: strings.Repeat("p", 256)},
	}
	for _, value := range invalid {
		if err := value.Validate(); err == nil {
			t.Fatalf("expected validation error for %+v", value)
		}
	}
}
