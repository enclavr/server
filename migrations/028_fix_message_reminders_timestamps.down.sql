-- +migrate Down
-- Revert TIMESTAMPTZ back to TIMESTAMP
ALTER TABLE message_reminders
    ALTER COLUMN remind_at TYPE TIMESTAMP USING remind_at::TIMESTAMP;

ALTER TABLE message_reminders
    ALTER COLUMN triggered_at TYPE TIMESTAMP USING triggered_at::TIMESTAMP;

ALTER TABLE message_reminders
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at::TIMESTAMP;

ALTER TABLE message_reminders
    ALTER COLUMN updated_at TYPE TIMESTAMP USING updated_at::TIMESTAMP;

ALTER TABLE message_reminders
    ALTER COLUMN deleted_at TYPE TIMESTAMP USING deleted_at::TIMESTAMP;

ALTER TABLE message_reminders
    ALTER COLUMN created_at SET DEFAULT CURRENT_TIMESTAMP;

ALTER TABLE message_reminders
    ALTER COLUMN updated_at SET DEFAULT CURRENT_TIMESTAMP;
