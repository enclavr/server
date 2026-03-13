-- +migrate Up
-- Create roles table for role-based access control (must be first due to FK dependencies)
CREATE TABLE IF NOT EXISTS roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(50) NOT NULL UNIQUE,
    display_name VARCHAR(100) NOT NULL,
    description VARCHAR(255),
    permissions TEXT NOT NULL,
    is_default BOOLEAN DEFAULT false,
    is_admin BOOLEAN DEFAULT false,
    room_id UUID REFERENCES rooms(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_roles_name ON roles (name);
CREATE INDEX IF NOT EXISTS idx_roles_room_id ON roles (room_id);
CREATE INDEX IF NOT EXISTS idx_roles_is_default ON roles (is_default) WHERE is_default = true;
CREATE INDEX IF NOT EXISTS idx_roles_deleted_at ON roles (deleted_at) WHERE deleted_at IS NULL;

-- Create role_permissions table for role permissions
CREATE TABLE IF NOT EXISTS role_permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission VARCHAR(50) NOT NULL,
    resource VARCHAR(50) NOT NULL,
    action VARCHAR(20) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_role_permissions_role_id ON role_permissions (role_id);
CREATE INDEX IF NOT EXISTS idx_role_permissions_deleted_at ON role_permissions (deleted_at) WHERE deleted_at IS NULL;

-- Create user_roles table for user role assignments
CREATE TABLE IF NOT EXISTS user_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    room_id UUID REFERENCES rooms(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_user_roles_user_id ON user_roles (user_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_role_id ON user_roles (role_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_room_id ON user_roles (room_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_user_role ON user_roles (user_id, role_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_deleted_at ON user_roles (deleted_at) WHERE deleted_at IS NULL;

-- Create attachments table for rich file attachments in messages
CREATE TABLE IF NOT EXISTS attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
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
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_attachments_message_id ON attachments (message_id);
CREATE INDEX IF NOT EXISTS idx_attachments_user_id ON attachments (user_id);
CREATE INDEX IF NOT EXISTS idx_attachments_file_id ON attachments (file_id);
CREATE INDEX IF NOT EXISTS idx_attachments_deleted_at ON attachments (deleted_at) WHERE deleted_at IS NULL;

-- Create category_permissions table for fine-grained category access control
CREATE TABLE IF NOT EXISTS category_permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    role_id UUID REFERENCES roles(id) ON DELETE CASCADE,
    permission VARCHAR(20) NOT NULL,
    can_view BOOLEAN DEFAULT true,
    can_create BOOLEAN DEFAULT false,
    can_edit BOOLEAN DEFAULT false,
    can_delete BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_category_permissions_category_id ON category_permissions (category_id);
CREATE INDEX IF NOT EXISTS idx_category_permissions_user_id ON category_permissions (user_id);
CREATE INDEX IF NOT EXISTS idx_category_permissions_role_id ON category_permissions (role_id);
CREATE INDEX IF NOT EXISTS idx_category_permissions_deleted_at ON category_permissions (deleted_at) WHERE deleted_at IS NULL;

-- Create user_devices table for device management and push notifications
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
    last_active_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_user_devices_user_id ON user_devices (user_id);
CREATE INDEX IF NOT EXISTS idx_user_devices_device_id ON user_devices (device_id);
CREATE INDEX IF NOT EXISTS idx_user_devices_is_active ON user_devices (is_active) WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_user_devices_deleted_at ON user_devices (deleted_at) WHERE deleted_at IS NULL;

-- Create audit_log_exclusions table for audit log exclusions
CREATE TABLE IF NOT EXISTS audit_log_exclusions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action VARCHAR(50) NOT NULL,
    reason VARCHAR(255),
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_audit_log_exclusions_user_id ON audit_log_exclusions (user_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_exclusions_action ON audit_log_exclusions (action);
CREATE INDEX IF NOT EXISTS idx_audit_log_exclusions_deleted_at ON audit_log_exclusions (deleted_at) WHERE deleted_at IS NULL;

-- Create api_keys table for API key management
CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    key_hash VARCHAR(64) NOT NULL UNIQUE,
    key_prefix VARCHAR(8) NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permissions TEXT,
    expires_at TIMESTAMP WITH TIME ZONE,
    last_used_at TIMESTAMP WITH TIME ZONE,
    ip_whitelist VARCHAR(500),
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys (user_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys (key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_is_active ON api_keys (is_active) WHERE is_active = true AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_api_keys_deleted_at ON api_keys (deleted_at) WHERE deleted_at IS NULL;

-- Create user_notifications table for in-app notifications
CREATE TABLE IF NOT EXISTS user_notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL,
    title VARCHAR(200) NOT NULL,
    body TEXT,
    data JSONB DEFAULT '{}'::jsonb,
    is_read BOOLEAN DEFAULT false,
    read_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_user_notifications_user_id ON user_notifications (user_id);
CREATE INDEX IF NOT EXISTS idx_user_notifications_is_read ON user_notifications (is_read) WHERE is_read = false AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_user_notifications_expires_at ON user_notifications (expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_user_notifications_created_at ON user_notifications (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_notifications_deleted_at ON user_notifications (deleted_at) WHERE deleted_at IS NULL;

-- Add composite index for audit logs - common query pattern
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_action_created ON audit_logs (user_id, action, created_at DESC);

-- Add composite index for messages - common query pattern for recent messages
CREATE INDEX IF NOT EXISTS idx_messages_room_created_deleted ON messages (room_id, created_at DESC) WHERE is_deleted = false;

-- +migrate Down
DROP TABLE IF EXISTS user_notifications CASCADE;
DROP TABLE IF EXISTS user_roles CASCADE;
DROP TABLE IF EXISTS role_permissions CASCADE;
DROP TABLE IF EXISTS roles CASCADE;
DROP TABLE IF EXISTS api_keys CASCADE;
DROP TABLE IF EXISTS audit_log_exclusions CASCADE;
DROP TABLE IF EXISTS user_devices CASCADE;
DROP TABLE IF EXISTS category_permissions CASCADE;
DROP TABLE IF EXISTS attachments CASCADE;

DROP INDEX IF EXISTS idx_audit_logs_user_action_created;
DROP INDEX IF EXISTS idx_messages_room_created_deleted;
