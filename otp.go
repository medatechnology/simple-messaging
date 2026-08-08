package simplemessage

import (
	"fmt"
	"strings"

	"github.com/medatechnology/goutil/encryption"
)

// DefaultOTPLength is used when SendRequest.CodeLength is not set.
const DefaultOTPLength = 6

// generateCode creates a numeric OTP code with goutil/encryption. Reuse is
// mandatory: the ecosystem never reimplements random/OTP generation.
func generateCode(length int) string {
	if length <= 0 {
		length = DefaultOTPLength
	}
	// GenerateOTP(digit, set, separator): digit = digits per group, set = 1
	// (single group → plain code, no separator). GenerateDefaultOTP() is the
	// same with library defaults — see goutil/encryption/random.go.
	code := encryption.GenerateOTP(length, 1, "")
	if code == "" {
		// Fallback through the secure numeric generator (same goutil package).
		code, _ = encryption.GenerateSecureRandomNumber(length)
	}
	return code
}

// prepareMessage fills the code into the request (generating it when absent)
// and renders the final body/subject.
func prepareMessage(req *SendRequest) (code, subject, body string) {
	code = req.Code
	if code == "" {
		code = generateCode(req.CodeLength)
	}

	subject = req.Subject
	if subject == "" {
		switch req.MessageType {
		case MessageTypeOTP, MessageTypeVerification:
			subject = "Your verification code"
		case MessageTypeWelcome:
			subject = "Welcome!"
		default:
			subject = "Notification"
		}
	}

	body = req.Body
	if strings.Contains(body, "{code}") {
		body = strings.ReplaceAll(body, "{code}", code)
	} else if req.MessageType == MessageTypeOTP || req.MessageType == MessageTypeVerification {
		validity := ""
		if req.ExpiresIn > 0 {
			validity = fmt.Sprintf(" (valid for %s)", req.ExpiresIn)
		}
		body = fmt.Sprintf("Your verification code is: %s%s", code, validity)
	}
	return code, subject, body
}
