package services

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	logger       *slog.Logger
	auth         smtp.Auth
	recentEmails map[string]time.Time
	mutex        sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc

	// goflow integration
	scheduler scheduler.Scheduler
}

// NewEmailService creates a new EmailService instance.
func NewEmailService(cfg *config.EmailConfig, logger *slog.Logger) *EmailService {
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
	subject := fmt.Sprintf("[markgo] Contact Form: %s", msg.Subject)
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
	// Create message
	msg := e.buildEmailMessage(e.config.From, to, subject, body)

	// Connect to SMTP server
	addr := fmt.Sprintf("%s:%d", e.config.Host, e.config.Port)

	e.logger.Debug("Sending email",
		"host", e.config.Host,
		"port", e.config.Port,
		"to", to,
		"subject", subject)

	// Send email
	err := smtp.SendMail(addr, e.auth, e.config.From, []string{to}, []byte(msg))
	if err != nil {
		e.logger.Error("Failed to send email",
			"error", err,
			"host", e.config.Host,
			"port", e.config.Port)
		return err
	}

	return nil
}

func (e *EmailService) buildEmailMessage(from, to, subject, body string) string {
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

	return msg.String()
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
	subject := "markgo Email Service Test"
	body := e.generateTestEmailBody()

	return e.sendEmail(e.config.To, subject, body)
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
            <p>This is a test email from the markgo application.</p>
            <p><strong>Timestamp:</strong> %s</p>
            <p><strong>From:</strong> %s</p>
            <p><strong>SMTP Host:</strong> %s:%d</p>
            <p>If you received this email, the email service is working correctly!</p>
        </div>
    </div>
</body>
</html>`,
		time.Now().Format("January 2, 2006 at 3:04 PM MST"),
		e.config.From,
		e.config.Host,
		e.config.Port)
}

// ValidateConfig validates the email configuration
func (e *EmailService) ValidateConfig() []string {
	var errors []string

	if e.config.Host == "" {
		errors = append(errors, "SMTP host is required")
	}

	if e.config.Port == 0 {
		errors = append(errors, "SMTP port is required")
	}

	if e.config.Username == "" {
		errors = append(errors, "SMTP username is required")
	}

	if e.config.Password == "" {
		errors = append(errors, "SMTP password is required")
	}

	if e.config.From == "" {
		errors = append(errors, "From email address is required")
	}

	if e.config.To == "" {
		errors = append(errors, "To email address is required")
	}

	// Validate email format
	if e.config.From != "" && !e.isValidEmail(e.config.From) {
		errors = append(errors, "From email address is invalid")
	}

	if e.config.To != "" && !e.isValidEmail(e.config.To) {
		errors = append(errors, "To email address is invalid")
	}

	return errors
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
