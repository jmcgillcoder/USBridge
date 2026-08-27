package proxy

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"strings"
	"unicode/utf8"
)

const maxCredentialBytes = 255

// Credentials are shared by HTTP Basic proxy authentication and SOCKS5 RFC 1929.
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (credentials Credentials) Validate() error {
	if credentials.Username == "" {
		return errors.New("proxy username is empty")
	}
	if credentials.Password == "" {
		return errors.New("proxy password is empty")
	}
	if !utf8.ValidString(credentials.Username) || !utf8.ValidString(credentials.Password) {
		return errors.New("proxy credentials must be valid UTF-8")
	}
	if strings.Contains(credentials.Username, ":") {
		return errors.New("proxy username cannot contain a colon")
	}
	if len(credentials.Username) > maxCredentialBytes || len(credentials.Password) > maxCredentialBytes {
		return errors.New("proxy username and password must not exceed 255 bytes")
	}
	return nil
}

func (credentials Credentials) Matches(username, password string) bool {
	expectedUsername := sha256.Sum256([]byte(credentials.Username))
	actualUsername := sha256.Sum256([]byte(username))
	expectedPassword := sha256.Sum256([]byte(credentials.Password))
	actualPassword := sha256.Sum256([]byte(password))
	return subtle.ConstantTimeCompare(expectedUsername[:], actualUsername[:])&
		subtle.ConstantTimeCompare(expectedPassword[:], actualPassword[:]) == 1
}
