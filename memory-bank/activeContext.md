# Active Context - Server

## Current Work Focus
Added new API endpoints and registered orphaned handlers

## Latest Changes (2026-03-27)
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
