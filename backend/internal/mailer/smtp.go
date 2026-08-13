//go:build !js

package mailer

import (
	"fmt"
	"net"
	"net/smtp"

	"email.local/backend/internal/netbridge"

	"github.com/pocketbase/pocketbase/core"
)

func SendSMTP(account *core.Record, raw []byte, from string, to []string) error {
	host := account.GetString("smtp_host")
	port := int(account.GetFloat("smtp_port"))
	address := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	security := netbridge.ParseSecurity(
		account.GetString("smtp_security"),
		account.GetBool("smtp_tls"),
	)

	conn, err := netbridge.Dial(
		"tcp",
		address,
		security,
		account.GetBool("tls_insecure"),
	)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if security == netbridge.SecuritySTARTTLS {
		tlsConfig := netbridge.TLSConfig(host, account.GetBool("tls_insecure"))
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}

	auth := smtp.PlainAuth(
		"",
		account.GetString("username"),
		account.GetString("password"),
		host,
	)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("smtp recipient %q: %w", recipient, err)
		}
	}

	data, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := data.Write(raw); err != nil {
		_ = data.Close()
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := data.Close(); err != nil {
		return fmt.Errorf("smtp data close: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("smtp quit: %w", err)
	}
	return nil
}
