-- Rollback 2FA and password history fields

-- Drop indexes
DROP INDEX IF EXISTS idx_users_tfa_enabled;
DROP INDEX IF EXISTS idx_users_email_verified;
DROP INDEX IF EXISTS idx_users_locked_until;

-- Drop 2FA columns
ALTER TABLE users DROP COLUMN IF EXISTS tfa_enabled;
ALTER TABLE users DROP COLUMN IF EXISTS tfa_secret;
ALTER TABLE users DROP COLUMN IF EXISTS recovery_codes;
ALTER TABLE users DROP COLUMN IF EXISTS tfa_updated_at;

-- Drop password management columns
ALTER TABLE users DROP COLUMN IF EXISTS password_changed_at;
ALTER TABLE users DROP COLUMN IF EXISTS password_history;
ALTER TABLE users DROP COLUMN IF EXISTS failed_login_attempts;
ALTER TABLE users DROP COLUMN IF EXISTS locked_until;

-- Drop email verification columns
ALTER TABLE users DROP COLUMN IF EXISTS email_verified;
ALTER TABLE users DROP COLUMN IF EXISTS verification_token;
ALTER TABLE users DROP COLUMN IF EXISTS verification_expires_at;
