package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"github.com/1mb-dev/goflow/pkg/scheduling/scheduler"
	"github.com/1mb-dev/goflow/pkg/scheduling/workerpool"

	"github.com/1mb-dev/markgo/internal/config"
	apperrors "github.com/1mb-dev/markgo/internal/errors"
	"github.com/1mb-dev/markgo/internal/models"
)

// Ensure EmailService implements EmailServiceInterface
var _ EmailServiceInterface = (*EmailService)(nil)

// EmailService provides email functionality.
type EmailService struct {
	config       config.EmailConfig
	blogTitle    string
	logger       *slog.Logger
	auth         smtp.Auth
	recentEmails map[string]time.Time
	mutex        sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc

	// goflow integration
	scheduler scheduler.Scheduler
}

// NewEmailService creates a new EmailService instance. blogTitle is the
// configured Blog.Title; it appears in subject prefixes and the test email
// body so owners see their blog's identity, not the engine name.
func NewEmailService(cfg *config.EmailConfig, blogTitle string, logger *slog.Logger) *EmailService {
	var auth smtp.Auth
	if cfg.Username != "" && cfg.Password != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Initialize goflow scheduler for email cleanup tasks
	goflowScheduler := scheduler.New()
	//nolint:errcheck // Ignore error: email service should continue even if scheduler fails to start
	_ = goflowScheduler.Start()

	es := &EmailService{
		config:       *cfg,
		blogTitle:    blogTitle,
		logger:       logger,
		auth:         auth,
		recentEmails: make(map[string]time.Time),
		ctx:          ctx,
		cancel:       cancel,
		scheduler:    goflowScheduler,
	}

	// Setup scheduled cleanup using goflow instead of manual goroutine
	es.setupEmailCleanupTasks()

	return es
}

// SendContactMessage sends a contact form message via email
func (e *EmailService) SendContactMessage(msg *models.ContactMessage) error {
	if e.config.Username == "" || e.config.Password == "" {
		e.logger.Warn("Email credentials not configured, skipping email send")
		return apperrors.ErrEmailNotConfigured
	}

	// Check for duplicate submission
	msgHash := e.generateMessageHash(msg)
	if e.isDuplicateEmail(msgHash) {
		e.logger.Warn("Duplicate email detected, skipping send",
			"from", msg.Email,
			"name", msg.Name,
			"subject", msg.Subject)
		return fmt.Errorf("duplicate email detected")
	}

	// Mark this email as sent
	e.markEmailSent(msgHash)

	// Create email content
	subject := e.contactSubject(msg.Subject)
	body, err := e.generateContactEmailBody(msg)
	if err != nil {
		return apperrors.NewHTTPError(500, "Failed to generate email template", err)
	}

	// Send email
	if err := e.sendEmail(e.config.To, subject, body); err != nil {
		return err // sendEmail should return appropriate error types
	}

	e.logger.Info("Contact form email sent successfully",
		"from", msg.Email,
		"name", msg.Name,
		"subject", msg.Subject)

	return nil
}

// SendNotification sends a general notification email
func (e *EmailService) SendNotification(to, subject, body string) error {
	if e.config.Username == "" || e.config.Password == "" {
		e.logger.Warn("Email credentials not configured, skipping notification")
		return apperrors.ErrEmailNotConfigured
	}

	return e.sendEmail(to, subject, body)
}

func (e *EmailService) sendEmail(to, subject, body string) error {
	// Build message — rejects header injection before anything is sent.
	msg, err := e.buildEmailMessage(e.config.From, to, subject, body)
	if err != nil {
		e.logger.Warn("Rejected outbound email with unsafe header value (possible injection)",
			"to", to,
			"error", err)
		return err
	}

	// Connect to SMTP server
	addr := fmt.Sprintf("%s:%d", e.config.Host, e.config.Port)

	e.logger.Debug("Sending email",
		"host", e.config.Host,
		"port", e.config.Port,
		"to", to,
		"subject", subject)

	// Send email
	err = smtp.SendMail(addr, e.auth, e.config.From, []string{to}, []byte(msg))
	if err != nil {
		e.logger.Error("Failed to send email",
			"error", err,
			"host", e.config.Host,
			"port", e.config.Port)
		return err
	}

	return nil
}

// errUnsafeEmailHeader is returned when a value bound for an email header
// contains CR, LF, or NUL — the bytes that enable SMTP header injection (an
// attacker-supplied "\r\nBcc: victim@x" in a contact-form field would forge
// headers and turn the operator's mail server into a relay).
var errUnsafeEmailHeader = errors.New("email header value contains CR, LF, or NUL")

// unsafeEmailHeaderValue reports whether v cannot be safely placed in an email
// header: CR/LF would split the header block (injection); NUL can truncate in
// downstream C-based parsers.
func unsafeEmailHeaderValue(v string) bool {
	return strings.ContainsAny(v, "\r\n\x00")
}

func (e *EmailService) buildEmailMessage(from, to, subject, body string) (string, error) {
	// Header-injection guard at the single chokepoint every outbound header
	// passes through, so no current or future caller can smuggle CR/LF into a
	// header value. Reject (don't silently strip) — a control char here is an
	// injection attempt, not a typo.
	for _, h := range []struct{ name, value string }{
		{"From", from}, {"To", to}, {"Subject", subject},
	} {
		if unsafeEmailHeaderValue(h.value) {
			return "", fmt.Errorf("%s header: %w", h.name, errUnsafeEmailHeader)
		}
	}

	var msg bytes.Buffer

	// Headers
	fmt.Fprintf(&msg, "From: %s\r\n", from)
	fmt.Fprintf(&msg, "To: %s\r\n", to)
	fmt.Fprintf(&msg, "Subject: %s\r\n", subject)
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	fmt.Fprintf(&msg, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	msg.WriteString("\r\n")

	// Body
	msg.WriteString(body)

	return msg.String(), nil
}

func (e *EmailService) generateContactEmailBody(msg *models.ContactMessage) (string, error) {
	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Contact Form Submission</title>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #f4f4f4; padding: 20px; border-radius: 5px; margin-bottom: 20px; }
        .content { background: #fff; padding: 20px; border: 1px solid #ddd; border-radius: 5px; }
        .field { margin-bottom: 15px; }
        .label { font-weight: bold; color: #555; }
        .value { margin-top: 5px; padding: 10px; background: #f9f9f9; border-radius: 3px; }
        .message { white-space: pre-wrap; }
        .footer {
            margin-top: 20px; padding: 15px; background: #f4f4f4;
            border-radius: 5px; font-size: 12px; color: #666;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h2>New Contact Form Submission</h2>
            <p>Received: {{.Timestamp}}</p>
        </div>

        <div class="content">
            <div class="field">
                <div class="label">Name:</div>
                <div class="value">{{.Name}}</div>
            </div>

            <div class="field">
                <div class="label">Email:</div>
                <div class="value">{{.Email}}</div>
            </div>

            <div class="field">
                <div class="label">Subject:</div>
                <div class="value">{{.Subject}}</div>
            </div>

            <div class="field">
                <div class="label">Message:</div>
                <div class="value message">{{.Message}}</div>
            </div>
        </div>

        <div class="footer">
            <p>This message was sent from the contact form on your website</p>
            <p>Reply directly to this email to respond to {{.Name}} ({{.Email}})</p>
        </div>
    </div>
</body>
</html>`

	t, err := template.New("contact").Parse(tmpl)
	if err != nil {
		return "", err
	}

	data := struct {
		Name      string
		Email     string
		Subject   string
		Message   string
		Timestamp string
	}{
		Name:      msg.Name,
		Email:     msg.Email,
		Subject:   msg.Subject,
		Message:   msg.Message,
		Timestamp: time.Now().Format("January 2, 2006 at 3:04 PM MST"),
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// TestConnection tests the email configuration
func (e *EmailService) TestConnection() error {
	if e.config.Username == "" || e.config.Password == "" {
		return apperrors.ErrEmailNotConfigured
	}

	addr := fmt.Sprintf("%s:%d", e.config.Host, e.config.Port)

	// Try to connect
	client, err := smtp.Dial(addr)
	if err != nil {
		return apperrors.NewHTTPError(503, "Email service temporarily unavailable", err)
	}
	defer func() { _ = client.Close() }()

	// Test authentication
	if e.auth != nil {
		if err := client.Auth(e.auth); err != nil {
			return apperrors.ErrSMTPAuthFailed
		}
	}

	e.logger.Info("Email service connection test successful")
	return nil
}

// SendTestEmail sends a test email to verify configuration
func (e *EmailService) SendTestEmail() error {
	return e.sendEmail(e.config.To, e.testEmailSubject(), e.generateTestEmailBody())
}

// contactSubject formats a contact-form email subject with the blog title prefix.
func (e *EmailService) contactSubject(msgSubject string) string {
	return fmt.Sprintf("[%s] Contact Form: %s", e.blogTitle, msgSubject)
}

// testEmailSubject formats the subject line for the admin test email.
func (e *EmailService) testEmailSubject() string {
	return e.blogTitle + " Email Service Test"
}

func (e *EmailService) generateTestEmailBody() string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Email Service Test</title>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #4CAF50; color: white; padding: 20px; border-radius: 5px; text-align: center; }
        .content { padding: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h2>✅ Email Service Test</h2>
        </div>
        <div class="content">
            <p>This is a test email from %s.</p>
            <p><strong>Timestamp:</strong> %s</p>
            <p><strong>From:</strong> %s</p>
            <p><strong>SMTP Host:</strong> %s:%d</p>
            <p>If you received this email, the email service is working correctly!</p>
        </div>
    </div>
</body>
</html>`,
		e.blogTitle,
		time.Now().Format("January 2, 2006 at 3:04 PM MST"),
		e.config.From,
		e.config.Host,
		e.config.Port)
}

// ValidateConfig validates the email configuration
func (e *EmailService) ValidateConfig() []string {
	var errs []string

	if e.config.Host == "" {
		errs = append(errs, "SMTP host is required")
	}

	if e.config.Port == 0 {
		errs = append(errs, "SMTP port is required")
	}

	if e.config.Username == "" {
		errs = append(errs, "SMTP username is required")
	}

	if e.config.Password == "" {
		errs = append(errs, "SMTP password is required")
	}

	if e.config.From == "" {
		errs = append(errs, "From email address is required")
	}

	if e.config.To == "" {
		errs = append(errs, "To email address is required")
	}

	// Validate email format
	if e.config.From != "" && !e.isValidEmail(e.config.From) {
		errs = append(errs, "From email address is invalid")
	}

	if e.config.To != "" && !e.isValidEmail(e.config.To) {
		errs = append(errs, "To email address is invalid")
	}

	return errs
}

func (e *EmailService) isValidEmail(email string) bool {
	// Basic email validation
	return strings.Contains(email, "@") && strings.Contains(email, ".")
}

// generateMessageHash creates a unique hash for a contact message
func (e *EmailService) generateMessageHash(msg *models.ContactMessage) string {
	data := fmt.Sprintf("%s|%s|%s|%s", msg.Name, msg.Email, msg.Subject, msg.Message)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)[:16] // Use first 16 chars for brevity
}

// isDuplicateEmail checks if the email was sent recently (within 5 minutes)
func (e *EmailService) isDuplicateEmail(hash string) bool {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	sentTime, exists := e.recentEmails[hash]
	if !exists {
		return false
	}

	// Consider it duplicate if sent within last 5 minutes
	return time.Since(sentTime) < 5*time.Minute
}

// markEmailSent marks an email as sent in the recent emails map
func (e *EmailService) markEmailSent(hash string) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.recentEmails[hash] = time.Now()
}

// setupEmailCleanupTasks configures background cleanup tasks using goflow scheduler
func (e *EmailService) setupEmailCleanupTasks() {
	if e.scheduler == nil {
		return
	}

	// Email cleanup task
	cleanupTask := workerpool.TaskFunc(func(_ context.Context) error {
		e.performCleanup()
		return nil
	})

	// Schedule cleanup every 10 minutes using cron format (6 fields: second, minute, hour, day, month, weekday)
	//nolint:errcheck // Ignore error: email service should continue even if cleanup scheduling fails
	_ = e.scheduler.ScheduleCron("email-cleanup", "0 */10 * * * *", cleanupTask)
}

// performCleanup removes old entries from the recent emails cache
func (e *EmailService) performCleanup() {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	cutoff := time.Now().Add(-10 * time.Minute)
	cleaned := 0

	for hash, sentTime := range e.recentEmails {
		if sentTime.Before(cutoff) {
			delete(e.recentEmails, hash)
			cleaned++
		}
	}

	if cleaned > 0 {
		e.logger.Debug("Cleaned up recent emails cache",
			"removed", cleaned,
			"remaining", len(e.recentEmails))
	}
}

// Shutdown gracefully shuts down the email service
func (e *EmailService) Shutdown() {
	e.logger.Info("Shutting down email service")

	// Stop goflow scheduler
	if e.scheduler != nil {
		e.scheduler.Stop()
	}

	// Cancel context
	e.cancel()
}

// GetConfig returns the current email configuration (without sensitive data)
func (e *EmailService) GetConfig() map[string]any {
	return map[string]any{
		"host":     e.config.Host,
		"port":     e.config.Port,
		"from":     e.config.From,
		"to":       e.config.To,
		"use_ssl":  e.config.UseSSL,
		"has_auth": e.config.Username != "" && e.config.Password != "",
	}
}
