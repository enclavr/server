-- Migration 021: Rollback featured rooms and session activity tables
-- Date: 2026-03-21

DROP TABLE IF EXISTS session_activities;
DROP TABLE IF EXISTS room_featured;
