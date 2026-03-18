package middleware

import (
	"context"
	"net/http"
	"strings"
)

type APIVersion struct {
	Major int
	Minor int
}

type contextKey string

const (
	APIVersionKey contextKey = "api_version"
)

func (v APIVersion) ToHeader() string {
	return "v" + string(rune('0'+v.Major)) + "." + string(rune('0'+v.Minor))
}

func (v APIVersion) GreaterThan(other APIVersion) bool {
	if v.Major != other.Major {
		return v.Major > other.Major
	}
	return v.Minor > other.Minor
}

func (v APIVersion) IsCompatibleWith(required APIVersion) bool {
	return v.Major == required.Major && v.Minor >= required.Minor
}

var (
	DefaultAPIVersion = APIVersion{Major: 1, Minor: 0}
	SupportedVersions = []APIVersion{
		{Major: 1, Minor: 0},
		{Major: 1, Minor: 1},
		{Major: 2, Minor: 0},
	}
)

type APIVersionMiddleware struct {
	defaultVersion APIVersion
	supported      []APIVersion
}

func NewAPIVersionMiddleware() *APIVersionMiddleware {
	return &APIVersionMiddleware{
		defaultVersion: DefaultAPIVersion,
		supported:      SupportedVersions,
	}
}

func (m *APIVersionMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		version := m.extractVersion(r)

		if version.Major == 0 {
			version = m.defaultVersion
		}

		ctx := context.WithValue(r.Context(), APIVersionKey, version)

		w.Header().Set("X-API-Version", version.ToHeader())
		w.Header().Set("X-API-Supported-Versions", m.getSupportedVersionsHeader())

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *APIVersionMiddleware) extractVersion(r *http.Request) APIVersion {
	version := m.defaultVersion

	acceptVersion := r.Header.Get("Accept")
	if acceptVersion != "" {
		if strings.HasPrefix(acceptVersion, "application/vnd.enclavr.") {
			versionStr := strings.TrimPrefix(acceptVersion, "application/vnd.enclavr.")
			versionStr = strings.TrimPrefix(versionStr, "v")
			version = m.parseVersion(versionStr)
		}
	}

	urlPath := r.URL.Path
	if strings.HasPrefix(urlPath, "/api/v") {
		parts := strings.SplitN(urlPath, "/", 4)
		if len(parts) >= 3 && strings.HasPrefix(parts[2], "v") {
			versionStr := strings.TrimPrefix(parts[2], "v")
			version = m.parseVersion(versionStr)
		}
	}

	return version
}

func (m *APIVersionMiddleware) parseVersion(versionStr string) APIVersion {
	version := APIVersion{Major: 1, Minor: 0}

	parts := strings.SplitN(versionStr, ".", 2)
	if len(parts) > 0 {
		if major, err := parseInt(parts[0]); err == nil {
			version.Major = major
		}
	}
	if len(parts) > 1 {
		if minor, err := parseInt(parts[1]); err == nil {
			version.Minor = minor
		}
	}

	return version
}

func parseInt(s string) (int, error) {
	var result int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errInvalidNumber
		}
		result = result*10 + int(c-'0')
	}
	return result, nil
}

var errInvalidNumber = &invalidNumberError{}

type invalidNumberError struct{}

func (e *invalidNumberError) Error() string {
	return "invalid number"
}

func (m *APIVersionMiddleware) getSupportedVersionsHeader() string {
	var versions []string
	for _, v := range m.supported {
		versions = append(versions, "v"+string(rune('0'+v.Major))+"."+string(rune('0'+v.Minor)))
	}
	return strings.Join(versions, ", ")
}

func GetAPIVersion(r *http.Request) APIVersion {
	if version, ok := r.Context().Value(APIVersionKey).(APIVersion); ok {
		return version
	}
	return DefaultAPIVersion
}

func RequireAPIVersion(minVersion APIVersion) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			version := GetAPIVersion(r)

			if version.Major != minVersion.Major || version.Minor < minVersion.Minor {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-API-Version", version.ToHeader())
				w.WriteHeader(http.StatusUpgradeRequired)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
