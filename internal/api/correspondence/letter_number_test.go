package correspondence

import (
	"testing"
	"time"

	"github.com/reyimanuel/letter-administration/internal/migration"
)

func TestBuildLetterNumberFormatsSequenceWorkCodeClassificationAndYear(t *testing.T) {
	got, err := buildLetterNumber("123", migration.LetterType{
		WorkCode:           "1",
		ClassificationCode: "km",
	}, time.Date(2026, time.June, 16, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("buildLetterNumber returned error: %v", err)
	}

	want := "123/UN12.2.1/KM/2026"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestBuildLetterNumberNormalizesSpacesInNumberSegments(t *testing.T) {
	got, err := buildLetterNumber(" 007 ", migration.LetterType{
		WorkCode:           " 5 1 ",
		ClassificationCode: " ll ",
	}, time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("buildLetterNumber returned error: %v", err)
	}

	want := "007/UN12.2.5.1/LL/2027"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestBuildLetterNumberAcceptsWorkCodeWithUN122Prefix(t *testing.T) {
	got, err := buildLetterNumber("123", migration.LetterType{
		WorkCode:           "UN12.2.5.2",
		ClassificationCode: "KM",
	}, time.Date(2026, time.June, 16, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("buildLetterNumber returned error: %v", err)
	}

	want := "123/UN12.2.5.2/KM/2026"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestBuildLetterNumberRejectsFullNumberInput(t *testing.T) {
	_, err := buildLetterNumber("123/UN12.2.FT/AKD/2026", migration.LetterType{
		WorkCode:           "FT",
		ClassificationCode: "AKD",
	}, time.Date(2026, time.June, 16, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected error for full letter number input")
	}
}

func TestBuildLetterNumberRequiresTemplateNumberCodes(t *testing.T) {
	_, err := buildLetterNumber("123", migration.LetterType{
		ClassificationCode: "AKD",
	}, time.Date(2026, time.June, 16, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected error for missing work code")
	}

	_, err = buildLetterNumber("123", migration.LetterType{
		WorkCode: "FT",
	}, time.Date(2026, time.June, 16, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected error for missing classification code")
	}
}
