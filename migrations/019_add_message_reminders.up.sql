-- Migration: Add Message Reminders
-- Allows users to set reminders for specific messages

CREATE TABLE IF NOT EXISTS message_reminders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    remind_at TIMESTAMP NOT NULL,
    note VARCHAR(255),
    is_triggered BOOLEAN DEFAULT FALSE,
    triggered_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_message_reminders_user_id ON message_reminders(user_id);
CREATE INDEX IF NOT EXISTS idx_message_reminders_message_id ON message_reminders(message_id);
CREATE INDEX IF NOT EXISTS idx_message_reminders_remind_at ON message_reminders(remind_at);
CREATE INDEX IF NOT EXISTS idx_message_reminders_pending ON message_reminders(user_id, remind_at) WHERE is_triggered = FALSE AND deleted_at IS NULL;
