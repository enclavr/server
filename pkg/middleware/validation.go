package middleware

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/enclavr/server/pkg/errors"
)

type ValidationRule func(string) bool

type FieldValidator struct {
	Field   string
	Rules   []ValidationRule
	Message string
}

func Required(field, message string) FieldValidator {
	return FieldValidator{
		Field: field,
		Rules: []ValidationRule{
			func(s string) bool {
				return strings.TrimSpace(s) != ""
			},
		},
		Message: message,
	}
}

func MinLength(field string, length int) FieldValidator {
	return FieldValidator{
		Field: field,
		Rules: []ValidationRule{
			func(s string) bool {
				return len(s) >= length
			},
		},
		Message: field + " must be at least " + string(rune(length)) + " characters",
	}
}

func MaxLength(field string, length int) FieldValidator {
	return FieldValidator{
		Field: field,
		Rules: []ValidationRule{
			func(s string) bool {
				return len(s) <= length
			},
		},
		Message: field + " must be at most " + string(rune(length)) + " characters",
	}
}

func Email(field string) FieldValidator {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return FieldValidator{
		Field: field,
		Rules: []ValidationRule{
			func(s string) bool {
				return emailRegex.MatchString(s)
			},
		},
		Message: field + " must be a valid email address",
	}
}

func UUID(field string) FieldValidator {
	uuidRegex := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	return FieldValidator{
		Field: field,
		Rules: []ValidationRule{
			func(s string) bool {
				return uuidRegex.MatchString(s)
			},
		},
		Message: field + " must be a valid UUID",
	}
}

func URL(field string) FieldValidator {
	urlRegex := regexp.MustCompile(`^https?://[^\s]+$`)
	return FieldValidator{
		Field: field,
		Rules: []ValidationRule{
			func(s string) bool {
				return urlRegex.MatchString(s)
			},
		},
		Message: field + " must be a valid URL",
	}
}

func Numeric(field string) FieldValidator {
	numericRegex := regexp.MustCompile(`^[0-9]+$`)
	return FieldValidator{
		Field: field,
		Rules: []ValidationRule{
			func(s string) bool {
				return numericRegex.MatchString(s)
			},
		},
		Message: field + " must be numeric",
	}
}

func AlphaNumeric(field string) FieldValidator {
	alphaNumericRegex := regexp.MustCompile(`^[a-zA-Z0-9]+$`)
	return FieldValidator{
		Field: field,
		Rules: []ValidationRule{
			func(s string) bool {
				return alphaNumericRegex.MatchString(s)
			},
		},
		Message: field + " must be alphanumeric",
	}
}

func In(field string, allowed ...string) FieldValidator {
	allowedSet := make(map[string]bool)
	for _, a := range allowed {
		allowedSet[a] = true
	}
	return FieldValidator{
		Field: field,
		Rules: []ValidationRule{
			func(s string) bool {
				return allowedSet[s]
			},
		},
		Message: field + " must be one of: " + strings.Join(allowed, ", "),
	}
}

func Pattern(field, pattern, message string) FieldValidator {
	regex := regexp.MustCompile(pattern)
	return FieldValidator{
		Field: field,
		Rules: []ValidationRule{
			func(s string) bool {
				return regex.MatchString(s)
			},
		},
		Message: message,
	}
}

type JSONValidator struct {
	Field   string
	Message string
}

func ValidateJSON(body interface{}, validators ...FieldValidator) *errors.Error {
	data, jsonErr := json.Marshal(body)
	if jsonErr != nil {
		return errors.InvalidParam("body", "Invalid JSON")
	}

	var fields map[string]interface{}
	if jsonErr := json.Unmarshal(data, &fields); jsonErr != nil {
		return errors.InvalidParam("body", "Invalid JSON structure")
	}

	var validationErrors []string

	for _, validator := range validators {
		value, exists := fields[validator.Field]
		if !exists {
			validationErrors = append(validationErrors, validator.Field+" is required")
			continue
		}

		strValue, ok := value.(string)
		if !ok {
			validationErrors = append(validationErrors, validator.Field+" must be a string")
			continue
		}

		for _, rule := range validator.Rules {
			if !rule(strValue) {
				validationErrors = append(validationErrors, validator.Message)
				break
			}
		}
	}

	if len(validationErrors) > 0 {
		return &errors.Error{
			Code:    errors.ErrCodeValidation,
			Message: "Validation failed",
			Details: validationErrors,
		}
	}

	return nil
}

func ValidateRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		if contentType == "" || !strings.Contains(contentType, "application/json") {
			WriteAPIError(w, errors.InvalidParam("Content-Type", "must be application/json"))
			return
		}

		if r.ContentLength > 10*1024*1024 {
			WriteAPIError(w, errors.New(errors.ErrCodeTooLarge, "Request body too large (max 10MB)"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func RequireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		if contentType == "" || !strings.Contains(contentType, "application/json") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnsupportedMediaType)
			if err := json.NewEncoder(w).Encode(map[string]string{
				"error":   "Unsupported Media Type",
				"message": "Content-Type must be application/json",
			}); err != nil {
				log.Printf("Error encoding response: %v", err)
			}
			return
		}
		next.ServeHTTP(w, r)
	})
}

type QueryParamValidator struct {
	Param    string
	Rules    []ValidationRule
	Message  string
	Required bool
}

func (v QueryParamValidator) Validate(value string) string {
	if value == "" && v.Required {
		return v.Param + " is required"
	}
	if value == "" && !v.Required {
		return ""
	}

	for _, rule := range v.Rules {
		if !rule(value) {
			return v.Message
		}
	}
	return ""
}

type QueryValidator struct {
	validators []QueryParamValidator
}

func NewQueryValidator() *QueryValidator {
	return &QueryValidator{
		validators: make([]QueryParamValidator, 0),
	}
}

func (v *QueryValidator) Add(param string, required bool, rules ...ValidationRule) *QueryValidator {
	message := param + " is invalid"
	v.validators = append(v.validators, QueryParamValidator{
		Param:    param,
		Required: required,
		Rules:    rules,
		Message:  message,
	})
	return v
}

func (v *QueryValidator) ValidateQuery(r *http.Request) []string {
	var validationErrors []string
	for _, validator := range v.validators {
		value := r.URL.Query().Get(validator.Param)
		if errMsg := validator.Validate(value); errMsg != "" {
			validationErrors = append(validationErrors, errMsg)
		}
	}
	return validationErrors
}

func (v *QueryValidator) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if validationErrors := v.ValidateQuery(r); len(validationErrors) > 0 {
			WriteError(w, http.StatusBadRequest, "Query parameter validation failed: "+strings.Join(validationErrors, ", "))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func ValidateRequestSize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				WriteError(w, http.StatusRequestEntityTooLarge, "Request body too large")
				return
			}

			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
