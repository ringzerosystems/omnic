package service

import (
	"fmt"
	"net/http"
)

// TwilioError represents a standard Twilio REST API error response.
type TwilioError struct {
	Message  string `json:"message"`
	MoreInfo string `json:"more_info"`
	Code     int    `json:"code"`
	Status   int    `json:"status"`
}

func (e *TwilioError) Error() string {
	return fmt.Sprintf("twilio error %d: %s (status %d)", e.Code, e.Message, e.Status)
}

// NewTwilioError creates a custom TwilioError.
func NewTwilioError(code, status int, message string) *TwilioError {
	return &TwilioError{
		Code:     code,
		Status:   status,
		Message:  message,
		MoreInfo: fmt.Sprintf("https://www.twilio.com/docs/errors/%d", code),
	}
}

// ErrNotFound returns a 404 resource not found error.
func ErrNotFound(msg string) *TwilioError {
	if msg == "" {
		msg = "The requested resource was not found"
	}
	return NewTwilioError(20404, http.StatusNotFound, msg)
}

// ErrMissingTo returns a 400 missing 'To' parameter error.
func ErrMissingTo() *TwilioError {
	return NewTwilioError(21604, http.StatusBadRequest, "A 'To' phone number is required.")
}

// ErrMissingBody returns a 400 missing body/media error.
func ErrMissingBody() *TwilioError {
	return NewTwilioError(21602, http.StatusBadRequest, "Message body or media URL is required.")
}

// ErrInvalidFrom returns a 400 invalid from number error.
func ErrInvalidFrom(from string) *TwilioError {
	return NewTwilioError(21606, http.StatusBadRequest, fmt.Sprintf("The From phone number %s is not a valid, SMS-capable Twilio number.", from))
}

// ErrUnsubscribed returns a 400 blocked by opt-out error.
func ErrUnsubscribed() *TwilioError {
	return NewTwilioError(21610, http.StatusBadRequest, "Attempt to send to unsubscribed recipient.")
}

// ErrMaxBodyExceeded returns a 400 body too long error.
func ErrMaxBodyExceeded() *TwilioError {
	return NewTwilioError(21617, http.StatusBadRequest, "The message body exceeds the maximum character limit.")
}

// ErrWebhookFailed returns an 11200 HTTP retrieval failure error.
func ErrWebhookFailed(msg string) *TwilioError {
	if msg == "" {
		msg = "HTTP retrieval failure"
	}
	return NewTwilioError(11200, http.StatusBadGateway, msg)
}
