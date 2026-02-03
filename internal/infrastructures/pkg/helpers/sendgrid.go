package helpers

import (
	"fmt"
	"log"

	"github.com/reyimanuel/letter-administration/internal/infrastructures/config"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

type SendGridEmailService struct {
	client *sendgrid.Client
	from   *mail.Email
}

func NewSendGridEmailService() *SendGridEmailService {
	cfg := config.Get()

	senderName := cfg.SendGrid.SenderName
	if senderName == "" {
		senderName = "E-Voting System"
	}

	log.Printf("[SendGrid] Initializing with sender: %s <%s>", senderName, cfg.SendGrid.SenderEmail)

	return &SendGridEmailService{
		client: sendgrid.NewSendClient(cfg.SendGrid.APIKey),
		from:   mail.NewEmail(senderName, cfg.SendGrid.SenderEmail),
	}
}

func (s *SendGridEmailService) SendEmail(to, subject, htmlBody string) error {
	toEmail := mail.NewEmail("", to)

	plainTextBody := htmlBody

	message := mail.NewSingleEmail(s.from, subject, toEmail, plainTextBody, htmlBody)

	log.Printf("[SendGrid] Sending email to: %s with subject: %s", to, subject)

	response, err := s.client.Send(message)
	if err != nil {
		log.Printf("[SendGrid] Error sending email: %v", err)
		return fmt.Errorf("failed to send email via SendGrid: %w", err)
	}

	if response.StatusCode >= 400 {
		log.Printf("[SendGrid] API returned error status %d: %s", response.StatusCode, response.Body)
		return fmt.Errorf("SendGrid returned error status %d: %s", response.StatusCode, response.Body)
	}

	log.Printf("[SendGrid] Email sent successfully to: %s (Status: %d)", to, response.StatusCode)
	return nil
}

func (s *SendGridEmailService) SendEmailWithAttachment(to, subject, htmlBody string, attachmentData []byte, attachmentName string) error {
	toEmail := mail.NewEmail("", to)
	plainTextBody := htmlBody

	message := mail.NewSingleEmail(s.from, subject, toEmail, plainTextBody, htmlBody)

	attachment := mail.NewAttachment()
	attachment.SetContent(string(attachmentData))
	attachment.SetType("application/octet-stream")
	attachment.SetFilename(attachmentName)
	attachment.SetDisposition("attachment")
	message.AddAttachment(attachment)

	log.Printf("[SendGrid] Sending email with attachment to: %s", to)

	response, err := s.client.Send(message)
	if err != nil {
		log.Printf("[SendGrid] Error sending email with attachment: %v", err)
		return fmt.Errorf("failed to send email with attachment via SendGrid: %w", err)
	}

	if response.StatusCode >= 400 {
		log.Printf("[SendGrid] API returned error status %d: %s", response.StatusCode, response.Body)
		return fmt.Errorf("SendGrid returned error status %d: %s", response.StatusCode, response.Body)
	}

	log.Printf("[SendGrid] Email with attachment sent successfully to: %s", to)
	return nil
}
