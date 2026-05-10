# Fixes Applied for Medium Priority Issues

## Summary
All medium priority security and functionality issues identified in the audit have been resolved.

## Fixes Implemented

### 1. Error Handling Information Leakage (Fixed)
- **Location**: Multiple files in `internal/api/users/service.go` and other services
- **Issue**: Error messages were exposing sensitive information like email addresses
- **Fix**: Removed sensitive data from error logs, replaced with generic messages
- **Examples**:
  - Changed `log.Printf("error sending verification email: %v to %s", err, email)` to `log.Printf("error sending verification email: %v", err)`
  - Similar fixes applied throughout the codebase

### 2. Pagination for Admin List Endpoints (Implemented)
- **Location**: `internal/api/users/service.go` and `internal/api/users/handler.go`
- **Issue**: Admin endpoints were returning all records without pagination
- **Fix**: Added limit and offset parameters with default values
- **Functions Modified**:
  - `GetAllUsers` in service layer now accepts `limit` and `offset` parameters
  - `GetPendingStudents` in service layer now accepts `limit` and `offset` parameters
  - Handler layer updated to pass query parameters to service layer

### 3. Standardized External Call Timeouts (Implemented)
- **Location**: `internal/constants/constants.go` and multiple service files
- **Issue**: Hardcoded timeout values scattered throughout the code
- **Fix**: Created centralized timeout constants and replaced hardcoded values
- **Constants Added**:
  - `ExternalServiceTimeout = 5 * time.Second` (for FCM calls)
  - `EmailTimeout = 8 * time.Second` (for email service calls)
  - `DatabaseQueryTimeout = 10 * time.Second` (for database queries)
- **Usage Updated**:
  - All `context.WithTimeout(context.Background(), 4*time.Second)` replaced with `constants.ExternalServiceTimeout`
  - Email service calls now use `SendVerificationEmailWithContext` with proper timeouts

### 4. File Upload Race Condition (Fixed)
- **Location**: `internal/api/letters/service.go` (UploadTemplateV2 function)
- **Issue**: File deletion happened outside database transaction, creating race conditions
- **Fix**: Moved file removal inside the database transaction
- **Key Changes**:
  - Old file removal now happens within the transaction callback
  - Uses `helpers.RemoveOldFile(oldPath, newPath)` inside transaction
  - Ensures atomicity of database update and file operation

### 5. Strengthened Password Policy (Implemented)
- **Location**: `internal/api/users/dto.go` and `internal/infrastructures/pkg/helpers/validator.go`
- **Issue**: Weak password requirements (minimum 6 characters, no complexity)
- **Fix**: Enhanced validation to require stronger passwords
- **Changes**:
  - Increased minimum length from 6 to 8 characters
  - Added custom validation rule `strongpassword` requiring:
    - At least one uppercase letter
    - At least one lowercase letter
    - At least one digit
  - Updated both `RegisterStudentRequest` and `RegisterWithKRSRequest` DTOs

## Files Modified
- `internal/constants/constants.go` - Added timeout constants
- `internal/api/users/service.php` - Implemented pagination, fixed error handling, standardized timeouts
- `internal/api/users/handler.go` - Updated to support pagination parameters
- `internal/api/users/dto.go` - Strengthened password validation rules
- `internal/api/users/repository.go` - Added paginated query methods
- `internal/api/users/routes.go` - Updated route handlers for pagination
- `internal/api/letters/service.go` - Fixed file upload race condition
- `internal/api/correspondence/service.go` - Standardized timeouts for push notifications
- `internal/api/notifications/service.go` - Standardized timeouts for push notifications
- `internal/infrastructures/pkg/helpers/email.go` - Added context timeout support
- `internal/infrastructures/pkg/helpers/validator.go` - Added strong password validation
- `internal/infrastructures/pkg/helpers/file.go` - Minor improvements to file handling
- `internal/infrastructures/pkg/token/load.go` - Security improvements
- `internal/api/users/service.go` - Fixed password handling in student registration
- `internal/api/correspondence/service.go` - Fixed import issues and standardized timeouts

## Verification
- All code builds successfully: `go build -v ./...`
- All tests pass: `go test ./... -v`
- No regressions introduced