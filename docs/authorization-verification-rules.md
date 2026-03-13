# Authorization & Verification Rules

## Verification Model
- Email verification is centralized in `users.email_verified_at`.
- Student admin verification is separated in `students.admin_verification_status`.
- Official accounts are created by admin and do not require extra admin verification status.

## Role Rules

### Student
A student can submit/process letters only when:
1. `users.is_active = true`
2. `users.email_verified_at IS NOT NULL`
3. `students.admin_verification_status = 'approved'`

### Official
An official can act only when:
1. `users.is_active = true`
2. `users.email_verified_at IS NOT NULL`
3. `officials.is_active = true`

### Admin
Admin role is authorized by role membership and standard auth middleware.

## Backward Compatibility Notes
- Deprecated columns (`users.verified`, `students.email_verified`, `officials.email_verified`) are no longer used in business logic.
- Existing clients that relied on old response fields must switch to:
  - `email_verified_at`
  - `admin_verification_status`
  - `admin_verified_at`
  - `rejection_reason`

## Migration Assumption
- Backfill sets student admin status to `approved` if old `users.verified=true`.
- Backfill sets `users.email_verified_at` only from old role-specific email flags (`students.email_verified` or `officials.email_verified`).
- If old data had `users.verified=true` but no email-verified evidence, user must verify email again (security-first).
