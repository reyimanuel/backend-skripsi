package policy

import (
	"testing"
	"time"

	"github.com/reyimanuel/letter-administration/internal/migration"
)

func TestCanStudentSubmitLetter_EmailUnverified_Fails(t *testing.T) {
	student := &migration.Student{ID: 1, UserID: 10, AdminVerificationStatus: "approved"}
	user := &migration.User{ID: 10, IsActive: true, EmailVerifiedAt: nil}

	if err := CanStudentSubmitLetter(user, student); err == nil {
		t.Fatalf("expected error for unverified email")
	}
}

func TestCanStudentSubmitLetter_AdminPending_Fails(t *testing.T) {
	now := time.Now()
	student := &migration.Student{ID: 1, UserID: 10, AdminVerificationStatus: "pending"}
	user := &migration.User{ID: 10, IsActive: true, EmailVerifiedAt: &now}

	if err := CanStudentSubmitLetter(user, student); err == nil {
		t.Fatalf("expected error for pending admin verification")
	}
}

func TestCanStudentSubmitLetter_Approved_Succeeds(t *testing.T) {
	now := time.Now()
	student := &migration.Student{ID: 1, UserID: 10, AdminVerificationStatus: "approved"}
	user := &migration.User{ID: 10, IsActive: true, EmailVerifiedAt: &now}

	if err := CanStudentSubmitLetter(user, student); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCanOfficialAct_EmailUnverified_Fails(t *testing.T) {
	official := &migration.Official{ID: 2, UserID: 20, IsOnDuty: true}
	user := &migration.User{ID: 20, IsActive: true, EmailVerifiedAt: nil}

	if err := CanOfficialAct(user, official); err == nil {
		t.Fatalf("expected error for official with unverified email")
	}
}

func TestCanOfficialAct_ActiveAndVerified_Succeeds(t *testing.T) {
	now := time.Now()
	official := &migration.Official{ID: 2, UserID: 20, IsOnDuty: true}
	user := &migration.User{ID: 20, IsActive: true, EmailVerifiedAt: &now}

	if err := CanOfficialAct(user, official); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
