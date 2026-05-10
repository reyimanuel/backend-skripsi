package helpers

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/go-playground/validator/v10"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
)

var validate *validator.Validate

func init() {
	validate = validator.New(validator.WithRequiredStructEnabled())
	// Register custom validation for safehtml
	_ = validate.RegisterValidation("safehtml", func(fl validator.FieldLevel) bool {
		return IsSafeHTML(fl.Field().String())
	})
	// Register custom validation for strong password
	_ = validate.RegisterValidation("strongpassword", func(fl validator.FieldLevel) bool {
		return IsStrongPassword(fl.Field().String())
	})
}

// ValidateStruct validates the struct using the validator package.
// It returns an error if the validation fails, or nil if it succeeds.
func ValidateStruct(payload any) error {
	err := validate.Struct(payload)
	if err != nil {
		return errs.BadRequest(err.Error())
	}

	return nil
}

func Choose(newVal, oldVal string) string {
	if strings.TrimSpace(newVal) != "" {
		return newVal
	}
	return oldVal
}

func ChooseTime(newValue, oldValue time.Time) time.Time {
	if !newValue.IsZero() {
		return newValue
	}
	return oldValue
}

func FirstValue(values []string) string {
	if len(values) > 0 {
		return values[0]
	}
	return ""
}

func ParseDate(dateStr string) time.Time {
	layout := "2006-01-02"
	t, err := time.Parse(layout, dateStr)
	if err != nil {
		return time.Time{} // return kosong kalau gagal
	}
	return t
}

// Contoh: "15:04"
func ParseTime(timeStr string) string {
	layout := "15:04"
	_, err := time.Parse(layout, timeStr)
	if err != nil {
		return ""
	}
	return timeStr
}

func MatchNameWithEmail(name, email string) bool {
	email = strings.ToLower(email)

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}

	localPart := parts[0]
	domain := parts[1]

	// 2️⃣ Domain wajib
	if domain != "student.unsrat.ac.id" {
		return false
	}

	// 3️⃣ Extract first & last name
	nameParts := strings.Fields(strings.ToLower(name))
	if len(nameParts) < 2 {
		return false
	}

	first := regexp.QuoteMeta(nameParts[0])
	last := regexp.QuoteMeta(nameParts[len(nameParts)-1])

	// 4️⃣ Regex: firstname + lastname + optional number
	pattern := fmt.Sprintf("^%s%s\\d*$", first, last)

	re := regexp.MustCompile(pattern)

	return re.MatchString(localPart)
}

// IsSafeHTML returns true if the string doesn't contain potentially dangerous HTML/JS content
// This helps prevent stored XSS attacks
func IsSafeHTML(s string) bool {
	// Check for common XSS patterns
	dangerousPatterns := []string{
		"<script",
		"</script>",
		"<img",
		"<svg",
		"onload=",
		"onerror=",
		"onclick=",
		"javascript:",
		"vbscript:",
		"expression(",
	}

	sLower := strings.ToLower(s)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(sLower, pattern) {
			return false
		}
	}
	return true
}

// IsStrongPassword validates password strength: minimum 8 characters, at least one uppercase,
// one lowercase, and one digit
func IsStrongPassword(password string) bool {
	if len(password) < 8 {
		return false
	}
	
	var hasUpper, hasLower, hasDigit bool
	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasDigit = true
		}
	}
	
	return hasUpper && hasLower && hasDigit
}
