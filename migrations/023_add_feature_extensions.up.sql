-- Create new feature tables for audit, preferences, categories, and attachments

-- 1. Preference Overrides table (room-specific preference settings)
CREATE TABLE IF NOT EXISTS preference_overrides (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    room_id UUID NOT NULL,
    setting_key VARCHAR(50) NOT NULL,
    setting_value TEXT,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_preference_overrides_user_id ON preference_overrides(user_id);
CREATE INDEX IF NOT EXISTS idx_preference_overrides_room_id ON preference_overrides(room_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_preference_overrides_user_room_key ON preference_overrides(user_id, room_id, setting_key);

-- 2. Category Settings table (per-category permissions and settings)
CREATE TABLE IF NOT EXISTS category_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id UUID NOT NULL UNIQUE,
    min_role_to_post UUID,
    min_role_to_voice UUID,
    min_role_to_invite UUID,
    is_nsfw BOOLEAN DEFAULT false,
    age_restricted BOOLEAN DEFAULT false,
    min_age INT DEFAULT 0,
    requires_approval BOOLEAN DEFAULT false,
    rate_limit INT DEFAULT 0,
    auto_archive_days INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_category_settings_category_id ON category_settings(category_id);

-- 3. Attachment Tags table (tags for file attachments)
CREATE TABLE IF NOT EXISTS attachment_tags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tag VARCHAR(50) NOT NULL,
    attachment_id UUID NOT NULL,
    color VARCHAR(7),
    created_by UUID,
    created_at TIMESTAMPTZ DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_attachment_tags_tag ON attachment_tags(tag);
CREATE INDEX IF NOT EXISTS idx_attachment_tags_attachment ON attachment_tags(attachment_id);
CREATE INDEX IF NOT EXISTS idx_attachment_tags_attachment_tag ON attachment_tags(attachment_id, tag);

-- 4. Add is_active column to audit_log_exclusions if not exists
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'audit_log_exclusions' AND column_name = 'is_active'
    ) THEN
        ALTER TABLE audit_log_exclusions ADD COLUMN is_active BOOLEAN DEFAULT true;
    END IF;
END $$;

-- 5. Add more preference columns to user_preferences
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'user_preferences' AND column_name = 'notification_sound'
    ) THEN
        ALTER TABLE user_preferences ADD COLUMN notification_sound VARCHAR(50) DEFAULT 'default';
    END IF;
    
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'user_preferences' AND column_name = 'push_notifications'
    ) THEN
        ALTER TABLE user_preferences ADD COLUMN push_notifications BOOLEAN DEFAULT true;
    END IF;
    
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'user_preferences' AND column_name = 'email_notifications'
    ) THEN
        ALTER TABLE user_preferences ADD COLUMN email_notifications BOOLEAN DEFAULT true;
    END IF;
    
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'user_preferences' AND column_name = 'ttt_language'
    ) THEN
        ALTER TABLE user_preferences ADD COLUMN ttt_language VARCHAR(10) DEFAULT 'en';
    END IF;
END $$;