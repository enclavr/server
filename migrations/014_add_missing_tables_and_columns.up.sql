-- +migrate Up
-- Create missing tables referenced in migration 013

-- Create attachments table for message/file attachments
-- This table stores metadata about files attached to messages
CREATE TABLE IF NOT EXISTS attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID REFERENCES messages(id) ON DELETE CASCADE,
    file_id UUID REFERENCES files(id) ON DELETE SET NULL,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    file_name VARCHAR(255) NOT NULL,
    file_size BIGINT NOT NULL,
    content_type VARCHAR(100) NOT NULL,
    thumbnail_url VARCHAR(500),
    width INTEGER,
    height INTEGER,
    duration INTEGER,
    alt_text VARCHAR(500),
    is_voice_memo BOOLEAN DEFAULT false,
    waveform_data TEXT,
    metadata JSONB,
    is_shared BOOLEAN DEFAULT false,
    share_count INTEGER DEFAULT 0,
    download_count INTEGER DEFAULT 0,
    view_count INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

-- Create index for attachments
CREATE INDEX IF NOT EXISTS idx_attachments_message ON attachments(message_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_attachments_user ON attachments(user_id, created_at DESC);

-- Rename message_read to message_reads if it exists, or create it
-- Check if message_read exists and has correct schema
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'message_reads') THEN
        CREATE TABLE IF NOT EXISTS message_reads (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
            message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
            read_at TIMESTAMPTZ DEFAULT now()
        );
        
        CREATE INDEX IF NOT EXISTS idx_message_reads_user_room ON message_reads(user_id, room_id);
        CREATE INDEX IF NOT EXISTS idx_message_reads_room ON message_reads(room_id, read_at DESC);
    END IF;
END $$;

-- Create user_statuses table if it doesn't exist
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'user_statuses') THEN
        CREATE TABLE IF NOT EXISTS user_statuses (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
            status VARCHAR(20) DEFAULT 'offline',
            status_text VARCHAR(150),
            status_emoji VARCHAR(10),
            expires_at TIMESTAMPTZ,
            updated_at TIMESTAMPTZ DEFAULT now()
        );
        
        CREATE INDEX IF NOT EXISTS idx_user_statuses_user ON user_statuses(user_id);
        CREATE INDEX IF NOT EXISTS idx_user_statuses_status ON user_statuses(status);
    END IF;
END $$;

-- Add missing columns to audit_logs if they don't exist
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'audit_logs' AND column_name = 'old_value') THEN
        ALTER TABLE audit_logs ADD COLUMN old_value JSONB;
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'audit_logs' AND column_name = 'new_value') THEN
        ALTER TABLE audit_logs ADD COLUMN new_value JSONB;
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'audit_logs' AND column_name = 'user_agent') THEN
        ALTER TABLE audit_logs ADD COLUMN user_agent VARCHAR(500);
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'audit_logs' AND column_name = 'success') THEN
        ALTER TABLE audit_logs ADD COLUMN success BOOLEAN DEFAULT true;
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'audit_logs' AND column_name = 'error_message') THEN
        ALTER TABLE audit_logs ADD COLUMN error_message VARCHAR(500);
    END IF;
END $$;

-- Create category_permissions table if it doesn't exist
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'category_permissions') THEN
        CREATE TABLE IF NOT EXISTS category_permissions (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
            user_id UUID,
            role_id UUID,
            permission VARCHAR(20) NOT NULL DEFAULT 'view',
            can_view BOOLEAN DEFAULT true,
            can_create BOOLEAN DEFAULT false,
            can_edit BOOLEAN DEFAULT false,
            can_delete BOOLEAN DEFAULT false,
            created_at TIMESTAMPTZ DEFAULT now(),
            updated_at TIMESTAMPTZ DEFAULT now(),
            deleted_at TIMESTAMPTZ
        );
        
        CREATE INDEX IF NOT EXISTS idx_category_permissions_category ON category_permissions(category_id);
        CREATE INDEX IF NOT EXISTS idx_category_permissions_user ON category_permissions(user_id);
    END IF;
END $$;

-- Create user_devices table if it doesn't exist
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'user_devices') THEN
        CREATE TABLE IF NOT EXISTS user_devices (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            device_id VARCHAR(100) NOT NULL,
            device_name VARCHAR(100),
            device_type VARCHAR(20) NOT NULL,
            os_version VARCHAR(20),
            app_version VARCHAR(20),
            push_token VARCHAR(500),
            fcm_token VARCHAR(500),
            apns_token VARCHAR(500),
            last_active_at TIMESTAMPTZ DEFAULT now(),
            is_active BOOLEAN DEFAULT true,
            created_at TIMESTAMPTZ DEFAULT now(),
            updated_at TIMESTAMPTZ DEFAULT now(),
            deleted_at TIMESTAMPTZ
        );
        
        CREATE INDEX IF NOT EXISTS idx_user_devices_user ON user_devices(user_id);
    END IF;
END $$;

-- +migrate Down
-- Drop columns from audit_logs
ALTER TABLE audit_logs DROP COLUMN IF EXISTS old_value;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS new_value;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS user_agent;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS success;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS error_message;

-- Drop tables
DROP TABLE IF EXISTS attachments;
DROP TABLE IF EXISTS message_reads;
DROP TABLE IF EXISTS user_statuses;
