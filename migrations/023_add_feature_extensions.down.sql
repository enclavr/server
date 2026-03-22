-- Rollback migration 023_add_feature_extensions

-- Drop attachment_tags
DROP TABLE IF EXISTS attachment_tags CASCADE;

-- Drop category_settings
DROP TABLE IF EXISTS category_settings CASCADE;

-- Drop preference_overrides
DROP TABLE IF EXISTS preference_overrides CASCADE;

-- Revert audit_log_exclusions column (PostgreSQL doesn't support DROP COLUMN easily with dependencies, so we leave it as is for safety)