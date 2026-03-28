-- +migrate Up
-- Fix TIMESTAMP to TIMESTAMPTZ in message_reminders table
-- Migration 019 used TIMESTAMP (without timezone) which causes incorrect
-- reminder delivery for users in non-UTC timezones.

ALTER TABLE message_reminders
    ALTER COLUMN remind_at TYPE TIMESTAMPTZ USING remind_at AT TIME ZONE 'UTC';

ALTER TABLE message_reminders
    ALTER COLUMN triggered_at TYPE TIMESTAMPTZ USING triggered_at AT TIME ZONE 'UTC';

ALTER TABLE message_reminders
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC';

ALTER TABLE message_reminders
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'UTC';

ALTER TABLE message_reminders
    ALTER COLUMN deleted_at TYPE TIMESTAMPTZ USING deleted_at AT TIME ZONE 'UTC';

-- Update defaults to use NOW() instead of CURRENT_TIMESTAMP for consistency
ALTER TABLE message_reminders
    ALTER COLUMN created_at SET DEFAULT NOW();

ALTER TABLE message_reminders
    ALTER COLUMN updated_at SET DEFAULT NOW();

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
