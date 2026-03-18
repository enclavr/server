-- Migration 015: Rollback sticker packs, room ratings, activity logs, and room metrics
-- Created: 2026-03-18

DROP TABLE IF EXISTS room_metrics CASCADE;
DROP TABLE IF EXISTS user_activity_logs CASCADE;
DROP TABLE IF EXISTS room_ratings CASCADE;
DROP TABLE IF EXISTS sticker_packs CASCADE;
