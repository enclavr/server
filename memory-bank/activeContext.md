# Active Context - Server

## Current Work Focus
Implemented all 4 missing features from progress.md - Room Templates, Category Permissions, Attachment Sharing routes, and DM Hub integration

## Latest Changes (2026-03-27)
- NEW: Add RoomTemplateHandler with full CRUD (Create, Get, GetByID, Update, Delete, CreateRoomFromTemplate)
- NEW: Add CategoryPermissionHandler with CRUD and CheckPermission (admin bypass, role-based)
- NEW: Register attachment sharing routes: /api/attachment/share, /api/attachment/shares, /api/attachment/shared, /api/attachment/share/delete
- NEW: Integrate DMHub in main.go - instantiate, add shutdown, add to status endpoint
- NEW: Add DMWebSocketHandler for real-time DM connections at /api/dm/ws
- NEW: Add exported methods to DMHub and DMClient (RegisterClient, UnregisterClient, GetActiveClients, SetUserID, etc.)
- NEW: 16 new test cases for RoomTemplate and CategoryPermission handlers
- All tests pass (100+)
- All lint passes

## Previous Changes (2026-03-27)
- Fix WebSocket hub race conditions: map mutation under read lock changed to write lock
- Fix goroutine leak in NewHubWithRedis and ReadPump blocking on unregister after shutdown
- Fix double-close panic: Shutdown() no longer duplicates gracefulShutdown() logic
- Fix metrics double-decremented on WebSocket disconnect
- Fix WritePump double-counting packets/bytes on write error
- Fix getErrorCategory broken range case (4000-4999 evaluated to -999)
- Add authorization checks to ban handler (owner/admin role required)
- Hash room and attachment share passwords with bcrypt
- Validate roomID in SendMessage handler (reject nil UUID and non-existent rooms)
- Fix DM GetConversations scan type mismatch (string vs time.Time) and error handling
- Add dedicated EmailVerified bool field to User model (was misusing OIDCIssuer)
- Fix RotateToken handler function signature mismatch (missing sessionID)
- Register nowFormat function in email templates
- Handle json.Marshal errors in WebSocket broadcast payloads
- Fix N+1 queries in ban, reaction, readreceipt, and DM handlers (batch user fetch)
- Add SQLite-compatible fallback for SearchMessages (LIKE instead of to_tsvector)
- Make database migration SQLite-compatible (tableExists, skip GIN index)
- Fix SearchRooms membership error handling (check result.Error == nil)
- Add typing cleanup goroutine to DMHub
- Renumber migration 003 -> 024 to fix version collision
- All tests pass
- All lint passes

## Previous Changes (2026-03-27)
- Fix NotificationHandler: removed gin.Context dependency, use standard http.HandlerFunc
- Register 10 orphaned handlers in main.go (RoomSettings, ScheduledMessage, StickerPack, RoomRating, UserActivityLog, RoomMetric, Notification)
- Add EditHistoryHandler: GET /api/message/edit-history
- Add RoomBookmarkHandler: CRUD for /api/room-bookmarks
- Add NotificationPreferencesHandler: GET/PUT /api/notification-preferences
- Register 35+ new API endpoints for existing but unregistered handlers
- Add comprehensive tests for all new handlers (18 test cases)
- All tests pass
- All lint passes

## Previous Changes (2026-03-26)
- Debugging completed

## Previous Changes (2026-03-23)
- Release v2026.03.24 created
- Complete UpdatePreferences handler - all preference fields apply
- Add GetBlockedByUsers endpoint
- Add ImportPrivacySettings handler
- Add new database models (PreferenceOverride, CategorySettings, AttachmentTag)
- Fix N+1 query in GetBlockedUsers
- All tests pass
- All lint passes
