-- Add 2FA and password history fields to users table
-- This migration adds security features for enhanced account protection

-- Add 2FA fields
ALTER TABLE users ADD COLUMN IF NOT EXISTS tfa_enabled BOOLEAN DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS tfa_secret TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS recovery_codes TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS tfa_updated_at TIMESTAMPTZ;

-- Add password management fields
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_changed_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_history TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS failed_login_attempts INT DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ;

-- Add email verification fields
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified BOOLEAN DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS verification_token TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS verification_expires_at TIMESTAMPTZ;

-- Create indexes for new fields
CREATE INDEX IF NOT EXISTS idx_users_tfa_enabled ON users(tfa_enabled) WHERE tfa_enabled = TRUE;
CREATE INDEX IF NOT EXISTS idx_users_email_verified ON users(email_verified) WHERE email_verified = FALSE;
CREATE INDEX IF NOT EXISTS idx_users_locked_until ON users(locked_until) WHERE locked_until IS NOT NULL;
