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
- Room templates (CRUD + create room from template)
- Category permissions (CRUD + check permission)
- Attachment sharing (share, get shares, get shared, delete share)
- WebSocket DM hub integration (real-time DM connections)

## What's Left to Build
- [ ] Additional WebSocket events for DM hub
- [ ] Rate limiting per user for DM connections
- [ ] DM hub message persistence

## What's Completed (2026-03-27)
- [x] NEW: RoomTemplateHandler - CRUD for room templates + create room from template
- [x] NEW: CategoryPermissionHandler - CRUD for category permissions + check permission
- [x] NEW: Register attachment sharing routes (4 endpoints)
- [x] NEW: Integrate DMHub in main.go with shutdown handling
- [x] NEW: DMWebSocketHandler for real-time DM WebSocket connections
- [x] NEW: Exported methods for DMHub and DMClient
- [x] NEW: 16 new test cases for RoomTemplate and CategoryPermission handlers
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
