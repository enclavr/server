package validator

import (
	"errors"
	"html"
	"strings"
	"unicode"
)

const MaxMessageLength = 4000
const MinMessageLength = 1

var (
	ErrMessageTooShort = errors.New("message content is too short")
	ErrMessageTooLong  = errors.New("message content exceeds maximum length")
	ErrContentEmpty    = errors.New("message content cannot be empty")
)

func ValidateMessageContent(content string) error {
	trimmed := strings.TrimSpace(content)

	if len(trimmed) < MinMessageLength {
		return ErrMessageTooShort
	}

	if len(trimmed) > MaxMessageLength {
		return ErrMessageTooLong
	}

	return nil
}

func SanitizeMessageContent(content string) string {
	content = html.EscapeString(content)

	content = removeExcessiveWhitespace(content)

	return content
}

func removeExcessiveWhitespace(s string) string {
	var result []rune
	var prevSpace bool

	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				result = append(result, ' ')
				prevSpace = true
			}
		} else {
			result = append(result, r)
			prevSpace = false
		}
	}

	return string(result)
}

func TruncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen]
}
