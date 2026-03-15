package validator

import (
	"strings"
	"testing"
)

func TestValidateMessageContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr error
	}{
		{
			name:    "valid message",
			content: "Hello, World!",
			wantErr: nil,
		},
		{
			name:    "empty message",
			content: "",
			wantErr: ErrMessageTooShort,
		},
		{
			name:    "whitespace only",
			content: "   ",
			wantErr: ErrMessageTooShort,
		},
		{
			name:    "message too long",
			content: string(make([]byte, MaxMessageLength+1)),
			wantErr: ErrMessageTooLong,
		},
		{
			name:    "message near max length",
			content: strings.Repeat("The quick brown fox jumps over the lazy dog. ", 40),
			wantErr: nil,
		},
		{
			name:    "message with control character",
			content: "Hello\x00World",
			wantErr: ErrInvalidCharacters,
		},
		{
			name:    "message with valid newline",
			content: "Hello\nWorld",
			wantErr: nil,
		},
		{
			name:    "message with valid tab",
			content: "Hello\tWorld",
			wantErr: nil,
		},
		{
			name:    "message with valid carriage return",
			content: "Hello\rWorld",
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMessageContent(tt.content)
			if err != tt.wantErr {
				t.Errorf("ValidateMessageContent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSanitizeMessageContent(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		maxLen int
	}{
		{
			name:  "no sanitization needed",
			input: "Hello, World!",
			want:  "Hello, World!",
		},
		{
			name:  "removes control characters",
			input: "Hello\x00World",
			want:  "HelloWorld",
		},
		{
			name:  "escapes HTML",
			input: "<script>alert('xss')</script>",
			want:  "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;",
		},
		{
			name:  "trims excessive whitespace",
			input: "Hello    World",
			want:  "Hello World",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeMessageContent(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeMessageContent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTruncateContent(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "no truncation needed",
			input:  "Hello",
			maxLen: 10,
			want:   "Hello",
		},
		{
			name:   "truncates to max length",
			input:  "Hello, World!",
			maxLen: 5,
			want:   "Hello",
		},
		{
			name:   "exact max length",
			input:  "Hello",
			maxLen: 5,
			want:   "Hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateContent(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("TruncateContent() = %v, want %v", got, tt.want)
			}
		})
	}
}
