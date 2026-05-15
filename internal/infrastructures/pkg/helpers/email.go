package helpers

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"net/url"
	"strings"

	"github.com/reyimanuel/letter-administration/internal/constants"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/config"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/token"
)

func SendEmail(toEmail string, subject string, plainTextBody string) error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("config not loaded")
	}
	smtpCfg := cfg.SMTP
	if smtpCfg.Host == "" {
		return fmt.Errorf("SMTP_HOST is not set")
	}
	if smtpCfg.Port == "" {
		return fmt.Errorf("SMTP_PORT is not set")
	}

	fromEmail := strings.TrimSpace(smtpCfg.SenderEmail)
	if fromEmail == "" {
		fromEmail = strings.TrimSpace(smtpCfg.User)
	}
	if fromEmail == "" {
		return fmt.Errorf("SMTP_SENDER_EMAIL is not set and SMTP_USER is empty")
	}

	msg := buildPlainTextMessage(fromEmail, toEmail, subject, plainTextBody)

	client, isTLS, err := dialSMTPClient(smtpCfg.Host, smtpCfg.Port)
	if err != nil {
		return err
	}
	defer client.Close()

	if smtpCfg.User != "" && smtpCfg.Pass != "" {
		if !isTLS {
			return fmt.Errorf("refusing to authenticate over non-TLS SMTP connection")
		}
		auth := smtp.PlainAuth("", smtpCfg.User, smtpCfg.Pass, smtpCfg.Host)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}

	if err := client.Mail(fromEmail); err != nil {
		return err
	}
	if err := client.Rcpt(toEmail); err != nil {
		return err
	}

	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	return client.Quit()
}

func buildPlainTextMessage(fromEmail string, toEmail string, subject string, plainTextBody string) string {
	// RFC 5322 requires CRLF line endings.
	cleanBody := strings.ReplaceAll(plainTextBody, "\r\n", "\n")
	cleanBody = strings.ReplaceAll(cleanBody, "\n", "\r\n")

	headers := []string{
		"From: " + fromEmail,
		"To: " + toEmail,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"",
	}

	return strings.Join(headers, "\r\n") + cleanBody + "\r\n"
}

func dialSMTPClient(host string, port string) (*smtp.Client, bool, error) {
	// Implicit TLS is commonly on port 465.
	if port == "465" {
		conn, err := tls.Dial("tcp", net.JoinHostPort(host, port), &tls.Config{ServerName: host})
		if err != nil {
			return nil, false, err
		}
		client, err := smtp.NewClient(conn, host)
		if err != nil {
			_ = conn.Close()
			return nil, false, err
		}
		return client, true, nil
	}

	client, err := smtp.Dial(net.JoinHostPort(host, port))
	if err != nil {
		return nil, false, err
	}

	isTLS := false
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
			_ = client.Close()
			return nil, false, err
		}
		isTLS = true
	}

	return client, isTLS, nil
}

func SendVerificationEmail(userID uint, email string, name string) error {
	verifyToken, err := token.GenerateEmailVerificationToken(userID, email)
	if err != nil {
		return err
	}

	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("config not loaded")
	}

	frontendBaseURL := strings.TrimRight(strings.TrimSpace(cfg.FrontEndURL), "/")
	verifyURL := frontendBaseURL + "/register/verify-email?token=" + url.QueryEscape(verifyToken) + "&email=" + url.QueryEscape(email)

	body := fmt.Sprintf(
		"Halo %s,\n\nKlik tautan berikut untuk memverifikasi email akun Anda:\n\n%s\n\nJika tautan tidak bisa dibuka, gunakan token berikut secara manual:\n\n%s\n\nJika Anda tidak merasa mendaftar, abaikan email ini.",
		name,
		verifyURL,
		verifyToken,
	)

	err = SendEmail(email, "Verifikasi Email Akun", body)
	if err != nil {
		return err
	}
	return nil
}

// SendVerificationEmailWithContext sends a verification email with context timeout
func SendVerificationEmailWithContext(ctx context.Context, userID uint, email string, name string) error {
	// Wrap the email sending in a timeout context
	sendCtx, cancel := context.WithTimeout(ctx, constants.EmailTimeout)
	defer cancel()
	
	// Use a channel to handle the asynchronous operation
	resultChan := make(chan error, 1)
	go func() {
		resultChan <- SendVerificationEmail(userID, email, name)
	}()
	
	select {
	case <-sendCtx.Done():
		return fmt.Errorf("email sending timed out after %v", constants.EmailTimeout)
	case err := <-resultChan:
		return err
	}
}
