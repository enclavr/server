-- +migrate Up
-- Migration 024: No-op
-- All tables originally created here (api_keys, roles, role_permissions,
-- user_roles, user_notifications, category_permissions, attachments) are
-- already created in earlier migrations (001, 006, 014).
-- This migration is intentionally left empty to avoid schema conflicts.

-- +migrate Down
-- Nothing to roll back - all tables are managed by earlier migrations.
