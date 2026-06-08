package services

import (
	"errors"
	"log/slog"
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/1mb-dev/markgo/internal/config"
	"github.com/1mb-dev/markgo/internal/models"
)

const testBlogTitle = "TestBlog"

func newTestEmailService(cfg *config.EmailConfig) *EmailService {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	return NewEmailService(cfg, testBlogTitle, logger)
}

func TestNewEmailService(t *testing.T) {
	tests := []struct {
		name    string
		config  config.EmailConfig
		hasAuth bool
	}{
		{"with credentials", config.EmailConfig{Host: "smtp.gmail.com", Port: 587, Username: "u", Password: "p", From: "a@b.com", To: "c@d.com", UseSSL: true}, true},
		{"without credentials", config.EmailConfig{Host: "smtp.gmail.com", Port: 587, From: "a@b.com", To: "c@d.com"}, false},
		{"empty config", config.EmailConfig{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newTestEmailService(&tt.config)
			assert.NotNil(t, service)
			if tt.hasAuth {
				assert.NotNil(t, service.auth)
			} else {
				assert.Nil(t, service.auth)
			}
		})
	}
}

func TestEmailService_SendContactMessage_NoCredentials(t *testing.T) {
	configs := []config.EmailConfig{
		{Host: "smtp.example.com", Port: 587, From: "a@b.com", To: "c@d.com"},
		{Host: "smtp.example.com", Port: 587, Username: "", Password: "", From: "a@b.com", To: "c@d.com"},
	}

	msg := &models.ContactMessage{Name: "John", Email: "john@example.com", Subject: "Test", Message: "Hello"}
	for _, cfg := range configs {
		service := newTestEmailService(&cfg)
		err := service.SendContactMessage(msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "email credentials not configured")
	}
}

func TestEmailService_SendNotification_NoCredentials(t *testing.T) {
	cfg := config.EmailConfig{Host: "smtp.example.com", Port: 587, From: "a@b.com", To: "c@d.com"}
	service := newTestEmailService(&cfg)
	err := service.SendNotification("c@d.com", "Subject", "Body")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "email credentials not configured")
}

func TestEmailService_TestConnection_NoCredentials(t *testing.T) {
	tests := []struct {
		name   string
		config config.EmailConfig
	}{
		{"no credentials", config.EmailConfig{Host: "smtp.example.com", Port: 587}},
		{"empty username", config.EmailConfig{Host: "smtp.example.com", Port: 587, Password: "p"}},
		{"empty password", config.EmailConfig{Host: "smtp.example.com", Port: 587, Username: "u"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newTestEmailService(&tt.config)
			err := service.TestConnection()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "email credentials not configured")
		})
	}
}

func TestEmailService_BuildEmailMessage(t *testing.T) {
	service := newTestEmailService(&config.EmailConfig{})
	msg, err := service.buildEmailMessage("from@a.com", "to@b.com", "Subject", "<p>Body</p>")
	assert.NoError(t, err)

	assert.Contains(t, msg, "From: from@a.com")
	assert.Contains(t, msg, "To: to@b.com")
	assert.Contains(t, msg, "Subject: Subject")
	assert.Contains(t, msg, "MIME-Version: 1.0")
	assert.Contains(t, msg, "Content-Type: text/html; charset=UTF-8")
	assert.Contains(t, msg, "<p>Body</p>")
	assert.Contains(t, msg, "\r\n")
}

// TestEmailService_BuildEmailMessage_RejectsHeaderInjection pins the v3.21.1
// fix: a CR/LF/NUL in any header value (notably an attacker-supplied contact
// Subject) is rejected at the buildEmailMessage chokepoint, so the SMTP message
// can never carry a forged header. Reject, don't strip — the email isn't sent.
func TestEmailService_BuildEmailMessage_RejectsHeaderInjection(t *testing.T) {
	service := newTestEmailService(&config.EmailConfig{})

	const safe = "ok@a.com"
	cases := []struct {
		name, from, to, subject string
	}{
		{"CRLF in subject (Bcc injection)", safe, safe, "Hi\r\nBcc: victim@evil.com"},
		{"bare LF in subject", safe, safe, "Hi\nBcc: victim@evil.com"},
		{"bare CR in subject", safe, safe, "Hi\rX-Inject: 1"},
		{"NUL in subject", safe, safe, "Hi\x00truncate"},
		{"CRLF in To", safe, "ok@a.com\r\nCc: victim@evil.com", "Subject"},
		{"CRLF in From", "ok@a.com\r\nReturn-Path: x", safe, "Subject"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := service.buildEmailMessage(tc.from, tc.to, tc.subject, "<p>body</p>")
			if !errors.Is(err, errUnsafeEmailHeader) {
				t.Fatalf("want errUnsafeEmailHeader, got err=%v", err)
			}
			if msg != "" {
				t.Errorf("want empty message on rejection, got:\n%s", msg)
			}
		})
	}
}

func TestEmailService_GenerateContactEmailBody(t *testing.T) {
	service := newTestEmailService(&config.EmailConfig{})
	msg := &models.ContactMessage{Name: "John", Email: "john@example.com", Subject: "Test", Message: "Hello"}

	body, err := service.generateContactEmailBody(msg)
	assert.NoError(t, err)
	assert.Contains(t, body, "John")
	assert.Contains(t, body, "john@example.com")
	assert.Contains(t, body, "<!DOCTYPE html>")
	assert.Contains(t, body, "New Contact Form Submission")
}

func TestEmailService_GenerateTestEmailBody(t *testing.T) {
	cfg := config.EmailConfig{Host: "smtp.example.com", Port: 587, From: "a@b.com"}
	service := newTestEmailService(&cfg)

	body := service.generateTestEmailBody()
	assert.Contains(t, body, "<!DOCTYPE html>")
	assert.Contains(t, body, "Email Service Test")
	assert.Contains(t, body, cfg.From)
	assert.Contains(t, body, cfg.Host)
	assert.Contains(t, body, testBlogTitle, "test email body should carry blog title")
	assert.NotContains(t, body, "markgo", "engine name should not leak into test email body")
}

func TestEmailService_ContactSubject(t *testing.T) {
	cases := []struct {
		blogTitle string
		input     string
		want      string
	}{
		{"TestBlog", "Hello", "[TestBlog] Contact Form: Hello"},
		{"notes.1mb.dev", "Bug report", "[notes.1mb.dev] Contact Form: Bug report"},
		{"My Awesome Blog", "Question about post", "[My Awesome Blog] Contact Form: Question about post"},
		{"TestBlog", "", "[TestBlog] Contact Form: "},
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	for _, tc := range cases {
		t.Run(tc.blogTitle+"|"+tc.input, func(t *testing.T) {
			svc := NewEmailService(&config.EmailConfig{}, tc.blogTitle, logger)
			assert.Equal(t, tc.want, svc.contactSubject(tc.input))
		})
	}
}

func TestEmailService_TestEmailSubject(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cases := map[string]string{
		"TestBlog":        "TestBlog Email Service Test",
		"notes.1mb.dev":   "notes.1mb.dev Email Service Test",
		"My Awesome Blog": "My Awesome Blog Email Service Test",
	}
	for title, want := range cases {
		svc := NewEmailService(&config.EmailConfig{}, title, logger)
		assert.Equal(t, want, svc.testEmailSubject(), "blogTitle=%q", title)
	}
}

func TestEmailService_ValidateConfig(t *testing.T) {
	tests := []struct {
		name           string
		config         config.EmailConfig
		expectedErrors []string
	}{
		{
			"valid config",
			config.EmailConfig{Host: "smtp.gmail.com", Port: 587, Username: "u@g.com", Password: "p", From: "u@g.com", To: "r@g.com"},
			[]string{},
		},
		{
			"empty config",
			config.EmailConfig{},
			[]string{"SMTP host is required", "SMTP port is required", "SMTP username is required", "SMTP password is required", "From email address is required", "To email address is required"},
		},
		{
			"invalid emails",
			config.EmailConfig{Host: "smtp.gmail.com", Port: 587, Username: "u@g.com", Password: "p", From: "invalid", To: "invalid"},
			[]string{"From email address is invalid", "To email address is invalid"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newTestEmailService(&tt.config)
			errs := service.ValidateConfig()
			assert.Equal(t, len(tt.expectedErrors), len(errs))
			for _, expected := range tt.expectedErrors {
				assert.True(t, slices.Contains(errs, expected), "missing: %s", expected)
			}
		})
	}
}

func TestEmailService_IsValidEmail(t *testing.T) {
	service := newTestEmailService(&config.EmailConfig{})
	tests := []struct {
		email string
		valid bool
	}{
		{"test@example.com", true},
		{"user@domain.org", true},
		{"name.surname@company.co.uk", true},
		{"invalid-email", false},
		{"@domain.com", true}, // basic validation: has @ and .
		{"user@", false},
		{"", false},
		{"user@domain", false},
		{"user.domain.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			assert.Equal(t, tt.valid, service.isValidEmail(tt.email))
		})
	}
}

func TestEmailService_GetConfig(t *testing.T) {
	cfg := config.EmailConfig{Host: "smtp.gmail.com", Port: 587, Username: "u@g.com", Password: "p", From: "u@g.com", To: "r@g.com", UseSSL: true}
	service := newTestEmailService(&cfg)
	m := service.GetConfig()

	assert.Equal(t, cfg.Host, m["host"])
	assert.Equal(t, cfg.Port, m["port"])
	assert.Equal(t, cfg.From, m["from"])
	assert.Equal(t, cfg.To, m["to"])
	assert.Equal(t, true, m["has_auth"])

	// sensitive data not exposed
	_, hasUsername := m["username"]
	_, hasPassword := m["password"]
	assert.False(t, hasUsername)
	assert.False(t, hasPassword)
}
