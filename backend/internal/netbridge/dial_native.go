//go:build !js

package netbridge

import (
	"crypto/tls"
	"fmt"
	"net"
)

type Security string

const (
	SecurityTLS      Security = "tls"
	SecuritySTARTTLS Security = "starttls"
	SecurityNone     Security = "none"
)

func Install() {}

func ParseSecurity(s string, legacyTLS bool) Security {
	switch Security(s) {
	case SecurityTLS, SecuritySTARTTLS, SecurityNone:
		return Security(s)
	default:
		if legacyTLS {
			return SecurityTLS
		}
		return SecurityNone
	}
}

func Dial(network, address string, sec Security, insecure bool) (net.Conn, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	cfg := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: insecure, //nolint:gosec // user-controlled lab setting
		MinVersion:         tls.VersionTLS12,
	}

	switch sec {
	case SecurityTLS:
		return tls.Dial(network, address, cfg)
	case SecuritySTARTTLS, SecurityNone:
		return net.Dial(network, address)
	default:
		return nil, fmt.Errorf("unknown security mode %q", sec)
	}
}

func TLSConfig(serverName string, insecure bool) *tls.Config {
	return &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: insecure, //nolint:gosec
		MinVersion:         tls.VersionTLS12,
	}
}
