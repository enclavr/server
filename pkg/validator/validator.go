package validator

import (
	"errors"
	"html"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxMessageLength = 4000
const MinMessageLength = 1

var (
	ErrMessageTooShort   = errors.New("message content is too short")
	ErrMessageTooLong    = errors.New("message content exceeds maximum length")
	ErrContentEmpty      = errors.New("message content cannot be empty")
	ErrInvalidCharacters = errors.New("message contains invalid characters")
	ErrSpamDetected      = errors.New("message flagged as potential spam")
)

func hasConsecutiveRepeatedChars(s string, minLen int) bool {
	if len(s) < minLen {
		return false
	}
	for i := 0; i <= len(s)-minLen; i++ {
		char := s[i]
		repeat := 1
		for j := i + 1; j < len(s) && s[j] == char; j++ {
			repeat++
		}
		if repeat >= minLen {
			return true
		}
		i += repeat - 1
	}
	return false
}

func hasRepeatedPattern(s string, minPatternLen int, minRepeats int) bool {
	if len(s) < minPatternLen*minRepeats {
		return false
	}
	for i := 0; i <= len(s)-minPatternLen*minRepeats; i++ {
		pattern := s[i : i+minPatternLen]
		repeats := 0
		pos := i
		for pos+minPatternLen <= len(s) && s[pos:pos+minPatternLen] == pattern {
			repeats++
			pos += minPatternLen
		}
		if repeats >= minRepeats {
			return true
		}
	}
	return false
}

func hasMultipleURLs(s string) bool {
	count := 0
	inURL := false
	for i := 0; i < len(s); i++ {
		if i+4 <= len(s) && (s[i:i+4] == "http" || s[i:i+4] == "HTTP") {
			count++
			inURL = true
		} else if inURL && (s[i] == ' ' || s[i] == '\n' || s[i] == '\t') {
			inURL = false
		}
	}
	return count >= 2
}

func hasControlChars(s string) bool {
	for _, r := range s {
		if (r >= 0x00 && r <= 0x08) || r == 0x0B || r == 0x0C || (r >= 0x0E && r <= 0x1F) || r == 0x7F {
			return true
		}
	}
	return false
}

func ValidateMessageContent(content string) error {
	trimmed := strings.TrimSpace(content)

	if len(trimmed) < MinMessageLength {
		return ErrMessageTooShort
	}

	if len(trimmed) > MaxMessageLength {
		return ErrMessageTooLong
	}

	if hasControlChars(trimmed) {
		return ErrInvalidCharacters
	}

	if hasConsecutiveRepeatedChars(trimmed, 11) {
		return ErrSpamDetected
	}

	if hasMultipleURLs(trimmed) {
		return ErrSpamDetected
	}

	if hasRepeatedPattern(trimmed, 3, 6) {
		return ErrSpamDetected
	}

	return nil
}

func SanitizeMessageContent(content string) string {
	content = strings.Map(func(r rune) rune {
		if (r >= 0x00 && r <= 0x08) || r == 0x0B || r == 0x0C || (r >= 0x0E && r <= 0x1F) || r == 0x7F {
			return -1
		}
		return r
	}, content)

	content = html.EscapeString(content)

	content = removeExcessiveWhitespace(content)

	content = removeInvalidUnicode(content)

	if utf8.RuneCountInString(content) > MaxMessageLength {
		runes := []rune(content)
		content = string(runes[:MaxMessageLength])
	}

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

func removeInvalidUnicode(s string) string {
	var result []rune
	for _, r := range s {
		if r == 0 || (r >= 0xD800 && r <= 0xDFFF) {
			continue
		}
		if r < 32 && r != '\t' && r != '\n' && r != '\r' {
			continue
		}
		result = append(result, r)
	}
	return string(result)
}

func TruncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen]
}
