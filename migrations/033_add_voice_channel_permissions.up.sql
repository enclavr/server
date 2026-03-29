CREATE TABLE IF NOT EXISTS voice_channel_permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id UUID NOT NULL,
    user_id UUID NOT NULL,
    can_join BOOLEAN NOT NULL DEFAULT true,
    can_speak BOOLEAN NOT NULL DEFAULT true,
    can_mute_others BOOLEAN NOT NULL DEFAULT false,
    can_deafen_others BOOLEAN NOT NULL DEFAULT false,
    can_move_users BOOLEAN NOT NULL DEFAULT false,
    is_priority BOOLEAN NOT NULL DEFAULT false,
    granted_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT fk_voice_channel_permissions_channel FOREIGN KEY (channel_id) REFERENCES voice_channels(id) ON DELETE CASCADE,
    CONSTRAINT fk_voice_channel_permissions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_voice_channel_permissions_granted_by FOREIGN KEY (granted_by) REFERENCES users(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_user_perm ON voice_channel_permissions(channel_id, user_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_vcp_channel_id ON voice_channel_permissions(channel_id);
CREATE INDEX IF NOT EXISTS idx_vcp_user_id ON voice_channel_permissions(user_id);
CREATE INDEX IF NOT EXISTS idx_vcp_deleted_at ON voice_channel_permissions(deleted_at);
