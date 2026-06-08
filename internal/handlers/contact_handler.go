package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/1mb-dev/markgo/internal/errors"
	"github.com/1mb-dev/markgo/internal/models"
	"github.com/1mb-dev/markgo/internal/services"
)

// maxContactBodySize caps the contact submission body before binding. The
// largest valid field (message) is 2000 chars, so 64KB is generous headroom
// while bounding the JSON an unauthenticated caller can force us to parse.
const maxContactBodySize = 64 << 10 // 64KB

// ContactHandler handles contact form display and submission.
type ContactHandler struct {
	*BaseHandler
	emailService services.EmailServiceInterface
}

// NewContactHandler creates a new contact handler.
func NewContactHandler(base *BaseHandler, emailService services.EmailServiceInterface) *ContactHandler {
	return &ContactHandler{
		BaseHandler:  base,
		emailService: emailService,
	}
}

// Submit handles contact form submissions.
func (h *ContactHandler) Submit(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxContactBodySize)

	var form struct {
		Name    string `json:"name" binding:"required,min=2,max=50"`
		Email   string `json:"email" binding:"required,email"`
		Subject string `json:"subject" binding:"required,min=5,max=100"`
		Message string `json:"message" binding:"required,min=10,max=2000"`
	}

	if err := c.ShouldBindJSON(&form); err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Request too large"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid form data",
			"details": err.Error(),
		})
		return
	}

	contactMsg := &models.ContactMessage{
		Name:    form.Name,
		Email:   form.Email,
		Subject: form.Subject,
		Message: form.Message,
	}

	if err := h.emailService.SendContactMessage(contactMsg); err != nil {
		if errors.Is(err, apperrors.ErrEmailNotConfigured) {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "Contact form temporarily unavailable",
				"message": "Email service is not configured. Please try again later or contact us through alternative means.",
				"status":  "unavailable",
			})
			return
		}

		h.handleError(c, err, "Failed to send contact message")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Contact message sent successfully",
		"status":  "success",
	})
}
