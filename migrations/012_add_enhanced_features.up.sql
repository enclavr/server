-- +migrate Up
-- Enhanced models: audit logs, preferences, categories, attachments

-- Add new columns to categories
ALTER TABLE categories 
ADD COLUMN IF NOT EXISTS description VARCHAR(500),
ADD COLUMN IF NOT EXISTS icon VARCHAR(100),
ADD COLUMN IF NOT EXISTS color VARCHAR(20),
ADD COLUMN IF NOT EXISTS is_private BOOLEAN DEFAULT false,
ADD COLUMN IF NOT EXISTS created_by UUID;

-- Add new columns to audit_logs
ALTER TABLE audit_logs 
ADD COLUMN IF NOT EXISTS old_value JSONB,
ADD COLUMN IF NOT EXISTS new_value JSONB,
ADD COLUMN IF NOT EXISTS user_agent VARCHAR(500),
ADD COLUMN IF NOT EXISTS success BOOLEAN DEFAULT true,
ADD COLUMN IF NOT EXISTS error_message VARCHAR(500);

-- Add new columns to user_preferences
ALTER TABLE user_preferences 
ADD COLUMN IF NOT EXISTS notification_sound VARCHAR(50) DEFAULT 'default',
ADD COLUMN IF NOT EXISTS desktop_notification BOOLEAN DEFAULT true,
ADD COLUMN IF NOT EXISTS mobile_notification BOOLEAN DEFAULT true,
ADD COLUMN IF NOT EXISTS mention_notification BOOLEAN DEFAULT true,
ADD COLUMN IF NOT EXISTS dm_notification BOOLEAN DEFAULT true,
ADD COLUMN IF NOT EXISTS show_typing_indicator BOOLEAN DEFAULT true,
ADD COLUMN IF NOT EXISTS show_read_receipts BOOLEAN DEFAULT true,
ADD COLUMN IF NOT EXISTS auto_scroll_messages BOOLEAN DEFAULT true,
ADD COLUMN IF NOT EXISTS use_24_hour_format BOOLEAN DEFAULT false,
ADD COLUMN IF NOT EXISTS display_mode VARCHAR(20) DEFAULT 'card';

-- Add new columns to attachments
ALTER TABLE attachments 
ADD COLUMN IF NOT EXISTS is_shared BOOLEAN DEFAULT false,
ADD COLUMN IF NOT EXISTS share_count INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS download_count INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS view_count INTEGER DEFAULT 0;

-- Create attachment_shares table
CREATE TABLE IF NOT EXISTS attachment_shares (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attachment_id UUID NOT NULL,
    shared_by UUID NOT NULL,
    share_url VARCHAR(500) UNIQUE NOT NULL,
    password VARCHAR(255),
    expires_at TIMESTAMP WITH TIME ZONE,
    max_downloads INTEGER DEFAULT 0,
    download_count INTEGER DEFAULT 0,
    view_count INTEGER DEFAULT 0,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_attachment_share_url ON attachment_shares(share_url);
CREATE INDEX IF NOT EXISTS idx_attachment_share_attachment ON attachment_shares(attachment_id);
CREATE INDEX IF NOT EXISTS idx_attachment_share_active ON attachment_shares(is_active) WHERE is_active = true;

-- Additional performance indexes
CREATE INDEX IF NOT EXISTS idx_audit_logs_target ON audit_logs(target_type, target_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_preferences_theme ON user_preferences(theme);
CREATE INDEX IF NOT EXISTS idx_categories_color ON categories(color);
CREATE INDEX IF NOT EXISTS idx_attachments_user_created ON attachments(user_id, created_at DESC);

-- +migrate Down
-- Drop attachment_shares table
DROP INDEX IF EXISTS idx_attachment_share_url;
DROP INDEX IF EXISTS idx_attachment_share_attachment;
DROP INDEX IF EXISTS idx_attachment_share_active;
DROP TABLE IF EXISTS attachment_shares;

-- Remove new columns from attachments
ALTER TABLE attachments DROP COLUMN IF EXISTS is_shared;
ALTER TABLE attachments DROP COLUMN IF EXISTS share_count;
ALTER TABLE attachments DROP COLUMN IF EXISTS download_count;
ALTER TABLE attachments DROP COLUMN IF EXISTS view_count;

-- Remove new columns from user_preferences
ALTER TABLE user_preferences DROP COLUMN IF EXISTS notification_sound;
ALTER TABLE user_preferences DROP COLUMN IF EXISTS desktop_notification;
ALTER TABLE user_preferences DROP COLUMN IF EXISTS mobile_notification;
ALTER TABLE user_preferences DROP COLUMN IF EXISTS mention_notification;
ALTER TABLE user_preferences DROP COLUMN IF EXISTS dm_notification;
ALTER TABLE user_preferences DROP COLUMN IF EXISTS show_typing_indicator;
ALTER TABLE user_preferences DROP COLUMN IF EXISTS show_read_receipts;
ALTER TABLE user_preferences DROP COLUMN IF EXISTS auto_scroll_messages;
ALTER TABLE user_preferences DROP COLUMN IF EXISTS use_24_hour_format;
ALTER TABLE user_preferences DROP COLUMN IF EXISTS display_mode;

-- Remove new columns from audit_logs
ALTER TABLE audit_logs DROP COLUMN IF EXISTS old_value;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS new_value;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS user_agent;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS success;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS error_message;

-- Remove new columns from categories
ALTER TABLE categories DROP COLUMN IF EXISTS description;
ALTER TABLE categories DROP COLUMN IF EXISTS icon;
ALTER TABLE categories DROP COLUMN IF EXISTS color;
ALTER TABLE categories DROP COLUMN IF EXISTS is_private;
ALTER TABLE categories DROP COLUMN IF EXISTS created_by;
