# Progress - Server

## What Works
- WebSocket hub with room management
- Client connection/disconnection handling
- Basic typing indicators
- Presence management
- Blocking (with database support via blocked_users table)
- Read receipts (with in-memory tracking)
- Message editing (with edit history in database)
- Threading (with thread support columns)
- Notification system (CRUD with read/archive/delete)
- Room bookmarks (CRUD with position ordering)
- Message edit history viewing
- Notification preferences management
- Room settings management
- Scheduled messages (CRUD + cancel)
- Sticker packs
- Room ratings
- User activity logging
- Room metrics

## What's Left to Build
- [ ] WebSocket DM hub integration (DMHub defined but not instantiated)
- [ ] Room templates handler
- [ ] Category permissions handler
- [ ] Attachment sharing endpoints

## What's Completed (2026-03-27)
- [x] NEW: Fix NotificationHandler - remove gin.Context, use http.HandlerFunc
- [x] NEW: Register RoomSettingsHandler routes
- [x] NEW: Register ScheduledMessageHandler routes
- [x] NEW: Register StickerPackHandler routes
- [x] NEW: Register RoomRatingHandler routes
- [x] NEW: Register UserActivityLogHandler routes
- [x] NEW: Register RoomMetricHandler routes
- [x] NEW: Register NotificationHandler routes
- [x] NEW: Add EditHistoryHandler (message edit history API)
- [x] NEW: Add RoomBookmarkHandler (room bookmarks CRUD)
- [x] NEW: Add NotificationPreferencesHandler (notification preferences CRUD)
- [x] NEW: 18 new test cases for all new handlers

## What's Completed (2026-03-23)
- [x] MAINTENANCE: Improve reconnection logic with exponential backoff
- [x] MAINTENANCE: Add comprehensive message validation
- [x] MAINTENANCE: Clean up error handling and add proper logging
- [x] NEW: Persist read receipts to database
- [x] NEW: Persist message edits to database  
- [x] NEW: Persist threads to database
- [x] NEW: Persist blocks to database
- [x] NEW: Add async message acknowledgment system
- [x] NEW: Add message delivery tracking with status updates

## Database Migrations (024)
- Added `message_edit_history` table for tracking edits
- Added `edited_at` column to messages
- Added `reply_count` column to messages
- Added `thread_id` column to messages
