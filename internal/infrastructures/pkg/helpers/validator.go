package helpers

import (
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
)

var validate *validator.Validate

// ValidateStruct validates the struct using the validator package.
// It returns an error if the validation fails, or nil if it succeeds.
func ValidateStruct(payload any) error {
	validate = validator.New(validator.WithRequiredStructEnabled())

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
