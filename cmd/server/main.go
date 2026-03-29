package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/enclavr/server/internal/auth"
	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/handlers"
	"github.com/enclavr/server/internal/metrics"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/internal/services"
	"github.com/enclavr/server/internal/websocket"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gorm.io/gorm"
)

var startTime = time.Now()

func initSentry(cfg *config.Config) {
	if cfg.Sentry.DSN == "" {
		log.Println("Sentry DSN not configured, skipping Sentry initialization")
		return
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              cfg.Sentry.DSN,
		Environment:      cfg.Sentry.Environment,
		Release:          getReleaseVersion(),
		TracesSampleRate: 1.0,
		EnableTracing:    true,
		Debug:            cfg.Sentry.Environment == "development",
		SampleRate:       1.0,
		MaxBreadcrumbs:   50,
		AttachStacktrace: true,
		SendDefaultPII:   false,
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			return filterSentryEvent(event, hint)
		},
	})
	if err != nil {
		log.Printf("Failed to initialize Sentry: %v", err)
		return
	}

	log.Printf("Sentry initialized with environment: %s, release: %s", cfg.Sentry.Environment, getReleaseVersion())
}

func getReleaseVersion() string {
	version := os.Getenv("VERSION")
	if version != "" {
		return version
	}
	if commit := os.Getenv("COMMIT_SHA"); commit != "" {
		return commit
	}
	return fmt.Sprintf("dev-%d", startTime.Unix())
}

func filterSentryEvent(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	if event.Request != nil {
		if event.Request.URL == "/health" || event.Request.URL == "/status" || event.Request.URL == "/metrics" {
			return nil
		}
	}

	return event
}

func main() {
	cfg := config.Load()

	initSentry(cfg)

	db, err := database.New(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	if err := db.Migrate(); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	var authService *auth.AuthService
	if cfg.Auth.EncryptionKey != "" {
		encryptor, err := auth.NewEncryptor(cfg.Auth.EncryptionKey)
		if err != nil {
			log.Printf("Warning: Failed to create encryptor, 2FA secrets will not be encrypted: %v", err)
			authService = auth.NewAuthService(&cfg.Auth)
		} else {
			authService = auth.NewAuthServiceWithEncryption(&cfg.Auth, encryptor)
			log.Println("Encryption enabled for sensitive data (2FA secrets)")
		}
	} else {
		authService = auth.NewAuthService(&cfg.Auth)
		log.Println("Warning: ENCRYPTION_KEY not set, 2FA secrets will not be encrypted. Set ENCRYPTION_KEY environment variable for production.")
	}
	bootstrapAdminUser(db, authService, &cfg.Admin)

	var hub *websocket.Hub
	if cfg.Redis.Host != "" && cfg.Redis.Host != "localhost" {
		log.Printf("Initializing WebSocket hub with Redis pub/sub at %s", cfg.Redis.Host)
		var err error
		hub, err = websocket.NewHubWithRedis(cfg.Redis.Host, cfg.Redis.Password, cfg.Redis.DB)
		if err != nil {
			log.Printf("Failed to initialize Redis pub/sub, falling back to in-memory: %v", err)
			hub = websocket.NewHub()
		}
	} else {
		hub = websocket.NewHub()
	}

	if hub.IsRedisEnabled() {
		metrics.RedisEnabled.Set(1)
	} else {
		metrics.RedisEnabled.Set(0)
	}

	dmHub := websocket.NewDMHub()

	inviteHandler := handlers.NewInviteHandler(db)
	inviteLinkHandler := handlers.NewInviteLinkHandler(db)

	emailServiceConfig := &services.EmailConfig{
		Provider:     services.EmailProvider(cfg.Email.SMTPHost),
		SMTPHost:     cfg.Email.SMTPHost,
		SMTPPort:     587,
		SMTPUsername: cfg.Email.SMTPUsername,
		SMTPPassword: cfg.Email.SMTPPassword,
		FromName:     "Enclavr",
		FromEmail:    cfg.Email.SMTPFrom,
		UseTLS:       cfg.Email.UseTLS,
	}
	emailService := services.NewEmailService(emailServiceConfig)
	oauthService := services.NewOAuthService(&cfg.Auth)
	loginTracker := auth.NewLoginAttemptTracker(5, 15*time.Minute, 15*time.Minute)

	authHandler := handlers.NewAuthHandler(db, authService, emailService, oauthService, cfg, cfg.Admin.FirstIsAdmin, loginTracker)
	roomHandler := handlers.NewRoomHandler(db)
	voiceHandler := handlers.NewVoiceHandler(db, hub, cfg)
	oidcHandler := handlers.NewOIDCHandler(db, &cfg.Auth)
	oauthHandler := handlers.NewOAuthHandler(db, authService, &cfg.Auth)
	passwordResetHandler := handlers.NewPasswordResetHandler(db, authService, &cfg.Auth, &cfg.Email)
	emailVerificationHandler := handlers.NewEmailVerificationHandler(db, &cfg.Auth, &cfg.Email)
	twoFactorHandler := handlers.NewTwoFactorHandler(db, authService, &cfg.Auth)
	sessionHandler := handlers.NewSessionHandler(db, &cfg.Auth, authService)
	messageHandler := handlers.NewMessageHandler(db, hub)
	presenceHandler := handlers.NewPresenceHandler(db)
	dmHandler := handlers.NewDirectMessageHandler(db)
	userHandler := handlers.NewUserHandler(db)
	bookmarkHandler := handlers.NewBookmarkHandler(db)
	categoryHandler := handlers.NewCategoryHandler(db)
	pinnedMessageHandler := handlers.NewPinnedMessageHandler(db, hub)
	reactionHandler := handlers.NewReactionHandler(db, hub)
	settingsHandler := handlers.NewSettingsHandler(db)
	roleHandler := handlers.NewRoleHandler(db)
	webhookHandler := handlers.NewWebhookHandler(db)
	fileHandler := handlers.NewFileHandler(db, cfg.Server.UploadDir, cfg.Server.MaxUploadSizeMB)
	threadHandler := handlers.NewThreadHandler(db, hub)
	pollHandler := handlers.NewPollHandler(db, hub)
	emojiHandler := handlers.NewEmojiHandler(db)
	stickerHandler := handlers.NewStickerHandler(db)
	soundboardHandler := handlers.NewSoundboardHandler(db, hub)
	analyticsHandler := handlers.NewAnalyticsHandler(db)
	auditHandler := handlers.NewAuditHandler(db)
	attachmentHandler := handlers.NewAttachmentHandler(db)
	exportHandler := handlers.NewExportHandler(db)
	pushHandler := handlers.NewPushHandler(db)
	banHandler := handlers.NewBanHandler(db)
	reportHandler := handlers.NewReportHandler(db)
	blockHandler := handlers.NewBlockHandler(db)
	connectionHandler := handlers.NewConnectionHandler(db)
	readReceiptHandler := handlers.NewReadReceiptHandler(db, hub)
	preferencesHandler := handlers.NewPreferencesHandler(db)
	privacyHandler := handlers.NewPrivacyHandler(db)
	statusHandler := handlers.NewStatusHandler(db)
	reminderHandler := handlers.NewReminderHandler(db)
	roomSettingsHandler := handlers.NewRoomSettingsHandler(db)
	scheduledMessageHandler := handlers.NewScheduledMessageHandler(db, hub)
	stickerPackHandler := handlers.NewStickerPackHandler(db)
	roomRatingHandler := handlers.NewRoomRatingHandler(db)
	userActivityLogHandler := handlers.NewUserActivityLogHandler(db)
	roomMetricHandler := handlers.NewRoomMetricHandler(db)
	notificationHandler := handlers.NewNotificationHandler(db)
	editHistoryHandler := handlers.NewEditHistoryHandler(db)
	roomBookmarkHandler := handlers.NewRoomBookmarkHandler(db)
	notificationPrefsHandler := handlers.NewNotificationPreferencesHandler(db)
	roomTemplateHandler := handlers.NewRoomTemplateHandler(db)
	categoryPermissionHandler := handlers.NewCategoryPermissionHandler(db)
	dmWebSocketHandler := handlers.NewDMWebSocketHandler(db, dmHub, cfg)
	dmReactionHandler := handlers.NewDMReactionHandler(db, dmHub)
	dmReadReceiptHandler := handlers.NewDMReadReceiptHandler(db, dmHub)
	mentionHandler := handlers.NewMentionHandler(db)
	roomTransferHandler := handlers.NewRoomTransferHandler(db)
	announcementHandler := handlers.NewAnnouncementHandler(db)
	voiceChannelHandler := handlers.NewVoiceChannelHandler(db)
	voiceChannelPermHandler := handlers.NewVoiceChannelPermissionHandler(db)
	groupDMHandler := handlers.NewGroupDMHandler(db)
	typingHandler := handlers.NewTypingIndicatorHandler(db)
	voiceSessionHandler := handlers.NewVoiceSessionHandler(db)
	webAuthnRPID := os.Getenv("WEBAUTHN_RPID")
	if webAuthnRPID == "" {
		webAuthnRPID = "localhost"
	}
	webAuthnService := services.NewWebAuthnService(db.DB, webAuthnRPID)
	webAuthnHandler := handlers.NewWebAuthnHandler(db, webAuthnService)
	_ = services.NewPushService(db, cfg)

	go hub.Run()
	go handlers.CleanupExpiredTypingIndicators(db.DB, 30*time.Second)

	middleware.InitRateLimiter(60)

	authRateLimiter := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			middleware.RateLimit(http.Handler(next)).ServeHTTP(w, r)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/register", authRateLimiter(authHandler.Register))
	mux.HandleFunc("/api/auth/login", authRateLimiter(authHandler.Login))
	mux.HandleFunc("/api/auth/refresh", authHandler.RefreshToken)
	mux.HandleFunc("/api/auth/me", middleware.RequireAuth(authService, authHandler.GetMe))

	mux.HandleFunc("/api/auth/oidc/login", oidcHandler.Login)
	mux.HandleFunc("/api/auth/oidc/callback", oidcHandler.Callback)
	mux.HandleFunc("/api/auth/oidc/config", oidcHandler.GetConfig)

	mux.HandleFunc("/api/auth/oauth/providers", oauthHandler.GetProviders)
	mux.HandleFunc("/api/auth/oauth/login", oauthHandler.Login)
	mux.HandleFunc("/api/auth/oauth/google/callback", oauthHandler.Callback)
	mux.HandleFunc("/api/auth/oauth/github/callback", oauthHandler.Callback)
	mux.HandleFunc("/api/auth/oauth/discord/callback", oauthHandler.Callback)

	mux.HandleFunc("/api/auth/password/forgot", passwordResetHandler.ForgotPassword)
	mux.HandleFunc("/api/auth/password/reset", passwordResetHandler.ResetPassword)
	mux.HandleFunc("/api/auth/password/validate-token", passwordResetHandler.ValidateToken)
	mux.HandleFunc("/api/auth/password/change", middleware.RequireAuth(authService, passwordResetHandler.ChangePassword))

	mux.HandleFunc("/api/auth/email/verify/send", middleware.RequireAuth(authService, emailVerificationHandler.SendVerification))
	mux.HandleFunc("/api/auth/email/verify", emailVerificationHandler.VerifyEmail)

	mux.HandleFunc("/api/auth/2fa/status", middleware.RequireAuth(authService, twoFactorHandler.GetStatus))
	mux.HandleFunc("/api/auth/2fa/setup", middleware.RequireAuth(authService, twoFactorHandler.Setup))
	mux.HandleFunc("/api/auth/2fa/enable", middleware.RequireAuth(authService, twoFactorHandler.Enable))
	mux.HandleFunc("/api/auth/2fa/disable", middleware.RequireAuth(authService, twoFactorHandler.Disable))
	mux.HandleFunc("/api/auth/2fa/verify", twoFactorHandler.Verify)
	mux.HandleFunc("/api/auth/2fa/recovery-codes", middleware.RequireAuth(authService, twoFactorHandler.GetRecoveryCodes))

	mux.HandleFunc("/api/auth/sessions", middleware.RequireAuth(authService, sessionHandler.GetSessions))
	mux.HandleFunc("/api/auth/sessions/revoke", middleware.RequireAuth(authService, sessionHandler.RevokeSession))
	mux.HandleFunc("/api/auth/sessions/revoke-all", middleware.RequireAuth(authService, sessionHandler.RevokeAllSessions))
	mux.HandleFunc("/api/auth/sessions/rotate", middleware.RequireAuth(authService, sessionHandler.RotateToken))
	mux.HandleFunc("/api/auth/sessions/count", middleware.RequireAuth(authService, sessionHandler.GetActiveSessionsCount))

	mux.HandleFunc("/api/auth/webauthn/register/begin", middleware.RequireAuth(authService, webAuthnHandler.BeginRegistration))
	mux.HandleFunc("/api/auth/webauthn/register/finish", middleware.RequireAuth(authService, webAuthnHandler.FinishRegistration))
	mux.HandleFunc("/api/auth/webauthn/login/begin", middleware.RequireAuth(authService, webAuthnHandler.BeginLogin))
	mux.HandleFunc("/api/auth/webauthn/login/finish", middleware.RequireAuth(authService, webAuthnHandler.FinishLogin))
	mux.HandleFunc("/api/auth/webauthn/credentials", middleware.RequireAuth(authService, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			webAuthnHandler.GetCredentials(w, r)
		case http.MethodDelete:
			webAuthnHandler.DeleteCredential(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/auth/webauthn/status", middleware.RequireAuth(authService, webAuthnHandler.GetStatus))

	mux.HandleFunc("/api/rooms", middleware.RequireAuth(authService, roomHandler.GetRooms))
	mux.HandleFunc("/api/room/create", middleware.RequireAuth(authService, roomHandler.CreateRoom))
	mux.HandleFunc("/api/room", middleware.RequireAuth(authService, roomHandler.GetRoom))
	mux.HandleFunc("/api/room/join", middleware.RequireAuth(authService, roomHandler.JoinRoom))
	mux.HandleFunc("/api/room/leave", middleware.RequireAuth(authService, roomHandler.LeaveRoom))
	mux.HandleFunc("/api/rooms/search", middleware.RequireAuth(authService, roomHandler.SearchRooms))

	mux.HandleFunc("/api/voice/ws", middleware.RequireAuth(authService, voiceHandler.HandleWebSocket))
	mux.HandleFunc("/api/voice/ice", voiceHandler.GetICEConfig)

	mux.HandleFunc("/api/voice-channels", middleware.RequireAuth(authService, voiceChannelHandler.GetRoomChannels))
	mux.HandleFunc("/api/voice-channel", middleware.RequireAuth(authService, voiceChannelHandler.GetChannel))
	mux.HandleFunc("/api/voice-channel/create", middleware.RequireAuth(authService, voiceChannelHandler.CreateChannel))
	mux.HandleFunc("/api/voice-channel/update", middleware.RequireAuth(authService, voiceChannelHandler.UpdateChannel))
	mux.HandleFunc("/api/voice-channel/delete", middleware.RequireAuth(authService, voiceChannelHandler.DeleteChannel))
	mux.HandleFunc("/api/voice-channel/join", middleware.RequireAuth(authService, voiceChannelHandler.JoinChannel))
	mux.HandleFunc("/api/voice-channel/leave", middleware.RequireAuth(authService, voiceChannelHandler.LeaveChannel))
	mux.HandleFunc("/api/voice-channel/participants", middleware.RequireAuth(authService, voiceChannelHandler.GetParticipants))
	mux.HandleFunc("/api/voice-channel/participant/update", middleware.RequireAuth(authService, voiceChannelHandler.UpdateParticipant))

	mux.HandleFunc("/api/voice-channel/permission", middleware.RequireAuth(authService, voiceChannelPermHandler.GetUserPermission))
	mux.HandleFunc("/api/voice-channel/permissions", middleware.RequireAuth(authService, voiceChannelPermHandler.GetPermissions))
	mux.HandleFunc("/api/voice-channel/permission/set", middleware.RequireAuth(authService, voiceChannelPermHandler.SetPermission))
	mux.HandleFunc("/api/voice-channel/permission/delete", middleware.RequireAuth(authService, voiceChannelPermHandler.DeletePermission))
	mux.HandleFunc("/api/voice-channel/permission/check", middleware.RequireAuth(authService, voiceChannelPermHandler.CheckPermission))
	mux.HandleFunc("/api/voice-channel/permissions/bulk", middleware.RequireAuth(authService, voiceChannelPermHandler.BulkSetPermissions))

	mux.HandleFunc("/api/dm/ws", middleware.RequireAuth(authService, dmWebSocketHandler.HandleDMWebSocket))

	mux.HandleFunc("/api/messages", middleware.RequireAuth(authService, messageHandler.GetMessages))
	mux.HandleFunc("/api/message/send", middleware.RequireAuth(authService, messageHandler.SendMessage))
	mux.HandleFunc("/api/message/update", middleware.RequireAuth(authService, messageHandler.UpdateMessage))
	mux.HandleFunc("/api/message/delete", middleware.RequireAuth(authService, messageHandler.DeleteMessage))
	mux.HandleFunc("/api/message/forward", middleware.RequireAuth(authService, messageHandler.ForwardMessage))

	mux.HandleFunc("/api/presence/update", middleware.RequireAuth(authService, presenceHandler.UpdatePresence))
	mux.HandleFunc("/api/presence/room", middleware.RequireAuth(authService, presenceHandler.GetPresence))
	mux.HandleFunc("/api/presence/user", middleware.RequireAuth(authService, presenceHandler.GetUserPresence))

	mux.HandleFunc("/api/dm/send", middleware.RequireAuth(authService, dmHandler.SendDM))
	mux.HandleFunc("/api/dm/conversations", middleware.RequireAuth(authService, dmHandler.GetConversations))
	mux.HandleFunc("/api/dm/messages", middleware.RequireAuth(authService, dmHandler.GetMessages))
	mux.HandleFunc("/api/dm/update", middleware.RequireAuth(authService, dmHandler.UpdateDM))
	mux.HandleFunc("/api/dm/delete", middleware.RequireAuth(authService, dmHandler.DeleteDM))

	mux.HandleFunc("/api/dm/reaction/add", middleware.RequireAuth(authService, dmReactionHandler.AddReaction))
	mux.HandleFunc("/api/dm/reaction/remove", middleware.RequireAuth(authService, dmReactionHandler.RemoveReaction))
	mux.HandleFunc("/api/dm/reactions", middleware.RequireAuth(authService, dmReactionHandler.GetReactions))

	mux.HandleFunc("/api/dm/read", middleware.RequireAuth(authService, dmReadReceiptHandler.MarkRead))
	mux.HandleFunc("/api/dm/read/status", middleware.RequireAuth(authService, dmReadReceiptHandler.GetReadStatus))
	mux.HandleFunc("/api/dm/read/all", middleware.RequireAuth(authService, dmReadReceiptHandler.MarkAllRead))

	mux.HandleFunc("/api/group-dm/create", middleware.RequireAuth(authService, groupDMHandler.CreateGroupDM))
	mux.HandleFunc("/api/group-dms", middleware.RequireAuth(authService, groupDMHandler.GetGroupDMs))
	mux.HandleFunc("/api/group-dm/messages", middleware.RequireAuth(authService, groupDMHandler.GetGroupDMMessages))
	mux.HandleFunc("/api/group-dm/message/send", middleware.RequireAuth(authService, groupDMHandler.SendGroupDMMessage))
	mux.HandleFunc("/api/group-dm/member/add", middleware.RequireAuth(authService, groupDMHandler.AddMember))
	mux.HandleFunc("/api/group-dm/member/remove", middleware.RequireAuth(authService, groupDMHandler.RemoveMember))
	mux.HandleFunc("/api/group-dm/leave", middleware.RequireAuth(authService, groupDMHandler.LeaveGroupDM))

	mux.HandleFunc("/api/users/search", middleware.RequireAuth(authService, userHandler.SearchUsers))
	mux.HandleFunc("/api/user/update", middleware.RequireAuth(authService, userHandler.UpdateUser))
	mux.HandleFunc("/api/user/profile", middleware.RequireAuth(authService, userHandler.GetProfile))

	mux.HandleFunc("/api/bookmarks", middleware.RequireAuth(authService, bookmarkHandler.GetBookmarks))
	mux.HandleFunc("/api/bookmark/create", middleware.RequireAuth(authService, bookmarkHandler.CreateBookmark))
	mux.HandleFunc("/api/bookmark/", middleware.RequireAuth(authService, bookmarkHandler.GetBookmark))
	mux.HandleFunc("/api/bookmark/update/", middleware.RequireAuth(authService, bookmarkHandler.UpdateBookmark))
	mux.HandleFunc("/api/bookmark/delete/", middleware.RequireAuth(authService, bookmarkHandler.DeleteBookmark))

	mux.HandleFunc("/api/categories", middleware.RequireAuth(authService, categoryHandler.GetCategories))
	mux.HandleFunc("/api/category/create", middleware.RequireAuth(authService, categoryHandler.CreateCategory))
	mux.HandleFunc("/api/category/update", middleware.RequireAuth(authService, categoryHandler.UpdateCategory))
	mux.HandleFunc("/api/category/delete", middleware.RequireAuth(authService, categoryHandler.DeleteCategory))

	mux.HandleFunc("/api/pinnedmessages", middleware.RequireAuth(authService, pinnedMessageHandler.GetPinnedMessages))
	mux.HandleFunc("/api/pinnedmessage/pin", middleware.RequireAuth(authService, pinnedMessageHandler.PinMessage))
	mux.HandleFunc("/api/pinnedmessage/unpin", middleware.RequireAuth(authService, pinnedMessageHandler.UnpinMessage))

	mux.HandleFunc("/api/reaction/add", middleware.RequireAuth(authService, reactionHandler.AddReaction))
	mux.HandleFunc("/api/reaction/remove", middleware.RequireAuth(authService, reactionHandler.RemoveReaction))
	mux.HandleFunc("/api/reactions", middleware.RequireAuth(authService, reactionHandler.GetReactions))

	mux.HandleFunc("/api/settings", middleware.RequireAuth(authService, settingsHandler.GetSettings))
	mux.HandleFunc("/api/settings/update", middleware.RequireAuth(authService, settingsHandler.UpdateSettings))

	mux.HandleFunc("/api/preferences", middleware.RequireAuth(authService, preferencesHandler.GetPreferences))
	mux.HandleFunc("/api/preferences/update", middleware.RequireAuth(authService, preferencesHandler.UpdatePreferences))

	mux.HandleFunc("/api/privacy", middleware.RequireAuth(authService, privacyHandler.GetPrivacySettings))
	mux.HandleFunc("/api/privacy/update", middleware.RequireAuth(authService, privacyHandler.UpdatePrivacySettings))
	mux.HandleFunc("/api/privacy/export", middleware.RequireAuth(authService, privacyHandler.ExportPrivacySettings))
	mux.HandleFunc("/api/privacy/reset", middleware.RequireAuth(authService, privacyHandler.ResetPrivacySettings))

	mux.HandleFunc("/api/status", middleware.RequireAuth(authService, statusHandler.GetStatus))
	mux.HandleFunc("/api/status/update", middleware.RequireAuth(authService, statusHandler.UpdateStatus))
	mux.HandleFunc("/api/status/user", middleware.RequireAuth(authService, statusHandler.GetUserStatus))

	mux.HandleFunc("/api/invite/create", middleware.RequireAuth(authService, inviteHandler.CreateInvite))
	mux.HandleFunc("/api/invites", middleware.RequireAuth(authService, inviteHandler.GetInvites))
	mux.HandleFunc("/api/invite/use", middleware.RequireAuth(authService, inviteHandler.UseInvite))
	mux.HandleFunc("/api/invite/revoke", middleware.RequireAuth(authService, inviteHandler.RevokeInvite))

	mux.HandleFunc("/api/invite-link/create", middleware.RequireAuth(authService, inviteLinkHandler.CreateInviteLink))
	mux.HandleFunc("/api/invite-links", middleware.RequireAuth(authService, inviteLinkHandler.GetInviteLinks))
	mux.HandleFunc("/api/invite-link/update", middleware.RequireAuth(authService, inviteLinkHandler.UpdateInviteLink))
	mux.HandleFunc("/api/invite-link/delete", middleware.RequireAuth(authService, inviteLinkHandler.DeleteInviteLink))
	mux.HandleFunc("/api/invite-link/resolve", inviteLinkHandler.ResolveInviteLink)
	mux.HandleFunc("/api/invite-link/use", middleware.RequireAuth(authService, inviteLinkHandler.UseInviteLink))

	mux.HandleFunc("/api/roles", middleware.RequireAuth(authService, roleHandler.GetRoles))
	mux.HandleFunc("/api/role/members", middleware.RequireAuth(authService, roleHandler.GetMembers))
	mux.HandleFunc("/api/role/user", middleware.RequireAuth(authService, roleHandler.GetUserRole))
	mux.HandleFunc("/api/role/update", middleware.RequireAuth(authService, roleHandler.UpdateRole))
	mux.HandleFunc("/api/role/kick", middleware.RequireAuth(authService, roleHandler.KickUser))

	mux.HandleFunc("/api/webhook/room/:room_id", middleware.RequireAuth(authService, webhookHandler.GetWebhooks))
	mux.HandleFunc("/api/webhook/create/:room_id", middleware.RequireAuth(authService, webhookHandler.CreateWebhook))
	mux.HandleFunc("/api/webhook/:webhook_id", middleware.RequireAuth(authService, webhookHandler.DeleteWebhook))
	mux.HandleFunc("/api/webhook/toggle/:webhook_id", middleware.RequireAuth(authService, webhookHandler.ToggleWebhook))
	mux.HandleFunc("/api/webhook/logs/:webhook_id", middleware.RequireAuth(authService, webhookHandler.GetWebhookLogs))

	mux.HandleFunc("/api/messages/search", middleware.RequireAuth(authService, messageHandler.SearchMessages))

	mux.HandleFunc("/api/files/upload", middleware.RequireAuth(authService, fileHandler.UploadFile))
	mux.HandleFunc("/api/files", middleware.RequireAuth(authService, fileHandler.GetRoomFiles))
	mux.HandleFunc("/api/files/delete", middleware.RequireAuth(authService, fileHandler.DeleteFile))
	mux.HandleFunc("/api/files/", fileHandler.GetFile)

	mux.HandleFunc("/api/thread/create", middleware.RequireAuth(authService, threadHandler.CreateThread))
	mux.HandleFunc("/api/thread", middleware.RequireAuth(authService, threadHandler.GetThread))
	mux.HandleFunc("/api/threads", middleware.RequireAuth(authService, threadHandler.GetThreadsForMessage))
	mux.HandleFunc("/api/thread/message", middleware.RequireAuth(authService, threadHandler.AddThreadMessage))
	mux.HandleFunc("/api/thread/message/update", middleware.RequireAuth(authService, threadHandler.UpdateThreadMessage))
	mux.HandleFunc("/api/thread/message/delete", middleware.RequireAuth(authService, threadHandler.DeleteThreadMessage))

	mux.HandleFunc("/api/poll/create", middleware.RequireAuth(authService, pollHandler.CreatePoll))
	mux.HandleFunc("/api/polls", middleware.RequireAuth(authService, pollHandler.GetPolls))
	mux.HandleFunc("/api/poll", middleware.RequireAuth(authService, pollHandler.GetPoll))
	mux.HandleFunc("/api/poll/vote", middleware.RequireAuth(authService, pollHandler.Vote))
	mux.HandleFunc("/api/poll/delete", middleware.RequireAuth(authService, pollHandler.DeletePoll))

	mux.HandleFunc("/api/emoji", middleware.RequireAuth(authService, emojiHandler.GetEmojis))
	mux.HandleFunc("/api/emoji/create", middleware.RequireAuth(authService, emojiHandler.CreateEmoji))
	mux.HandleFunc("/api/emoji/delete", middleware.RequireAuth(authService, emojiHandler.DeleteEmoji))

	mux.HandleFunc("/api/sticker", middleware.RequireAuth(authService, stickerHandler.GetStickers))
	mux.HandleFunc("/api/sticker/create", middleware.RequireAuth(authService, stickerHandler.CreateSticker))
	mux.HandleFunc("/api/sticker/delete", middleware.RequireAuth(authService, stickerHandler.DeleteSticker))

	mux.HandleFunc("/api/soundboard", middleware.RequireAuth(authService, soundboardHandler.GetSounds))
	mux.HandleFunc("/api/soundboard/create", middleware.RequireAuth(authService, soundboardHandler.CreateSound))
	mux.HandleFunc("/api/soundboard/play", middleware.RequireAuth(authService, soundboardHandler.PlaySound))
	mux.HandleFunc("/api/soundboard/delete", middleware.RequireAuth(authService, soundboardHandler.DeleteSound))

	mux.HandleFunc("/api/analytics/overview", middleware.RequireAuth(authService, analyticsHandler.GetOverview))
	mux.HandleFunc("/api/analytics/daily", middleware.RequireAuth(authService, analyticsHandler.GetDailyActivity))
	mux.HandleFunc("/api/analytics/channels", middleware.RequireAuth(authService, analyticsHandler.GetChannelStats))
	mux.HandleFunc("/api/analytics/hourly", middleware.RequireAuth(authService, analyticsHandler.GetHourlyActivity))
	mux.HandleFunc("/api/analytics/users", middleware.RequireAuth(authService, analyticsHandler.GetTopUsers))

	mux.HandleFunc("/api/audit/logs", middleware.RequireAuth(authService, auditHandler.GetAuditLogs))
	mux.HandleFunc("/api/attachments", middleware.RequireAuth(authService, attachmentHandler.GetMessageAttachments))
	mux.HandleFunc("/api/attachment/create", middleware.RequireAuth(authService, attachmentHandler.CreateAttachment))
	mux.HandleFunc("/api/attachment", middleware.RequireAuth(authService, attachmentHandler.GetAttachment))
	mux.HandleFunc("/api/attachment/update", middleware.RequireAuth(authService, attachmentHandler.UpdateAttachment))
	mux.HandleFunc("/api/attachment/delete", middleware.RequireAuth(authService, attachmentHandler.DeleteAttachment))
	mux.HandleFunc("/api/export", middleware.RequireAuth(authService, exportHandler.ExportServer))

	mux.HandleFunc("/api/push/subscribe", middleware.RequireAuth(authService, pushHandler.Subscribe))
	mux.HandleFunc("/api/push/unsubscribe", middleware.RequireAuth(authService, pushHandler.Unsubscribe))
	mux.HandleFunc("/api/push/subscriptions", middleware.RequireAuth(authService, pushHandler.GetSubscriptions))
	mux.HandleFunc("/api/push/settings", middleware.RequireAuth(authService, pushHandler.GetNotificationSettings))
	mux.HandleFunc("/api/push/settings/update", middleware.RequireAuth(authService, pushHandler.UpdateNotificationSettings))
	mux.HandleFunc("/api/push/test", middleware.RequireAuth(authService, pushHandler.TestNotification))

	mux.HandleFunc("/api/ban", middleware.RequireAuth(authService, banHandler.CreateBan))
	mux.HandleFunc("/api/ban/room", middleware.RequireAuth(authService, banHandler.GetBans))
	mux.HandleFunc("/api/ban/check", middleware.RequireAuth(authService, banHandler.CheckUserBan))
	mux.HandleFunc("/api/ban/", middleware.RequireAuth(authService, banHandler.GetBan))
	mux.HandleFunc("/api/ban/update", middleware.RequireAuth(authService, banHandler.UpdateBan))
	mux.HandleFunc("/api/ban/delete", middleware.RequireAuth(authService, banHandler.DeleteBan))

	mux.HandleFunc("/api/report", middleware.RequireAuth(authService, reportHandler.CreateReport))
	mux.HandleFunc("/api/reports", middleware.RequireAuth(authService, reportHandler.GetReports))
	mux.HandleFunc("/api/report/", middleware.RequireAuth(authService, reportHandler.GetReport))
	mux.HandleFunc("/api/report/review", middleware.RequireAuth(authService, reportHandler.ReviewReport))
	mux.HandleFunc("/api/report/delete", middleware.RequireAuth(authService, reportHandler.DeleteReport))
	mux.HandleFunc("/api/reports/my", middleware.RequireAuth(authService, reportHandler.GetMyReports))

	mux.HandleFunc("/api/block", middleware.RequireAuth(authService, blockHandler.BlockUser))
	mux.HandleFunc("/api/block/unblock", middleware.RequireAuth(authService, blockHandler.UnblockUser))
	mux.HandleFunc("/api/block/list", middleware.RequireAuth(authService, blockHandler.GetBlockedUsers))
	mux.HandleFunc("/api/block/check", middleware.RequireAuth(authService, blockHandler.IsBlocked))
	mux.HandleFunc("/api/block/blocked-by", middleware.RequireAuth(authService, blockHandler.GetBlockedByUsers))

	mux.HandleFunc("/api/connections", middleware.RequireAuth(authService, connectionHandler.GetConnections))
	mux.HandleFunc("/api/connections/request", middleware.RequireAuth(authService, connectionHandler.SendRequest))
	mux.HandleFunc("/api/connections/accept", middleware.RequireAuth(authService, connectionHandler.AcceptRequest))
	mux.HandleFunc("/api/connections/reject", middleware.RequireAuth(authService, connectionHandler.RejectRequest))
	mux.HandleFunc("/api/connections/remove", middleware.RequireAuth(authService, connectionHandler.RemoveConnection))
	mux.HandleFunc("/api/connections/pending", middleware.RequireAuth(authService, connectionHandler.GetPendingRequests))
	mux.HandleFunc("/api/connections/sent", middleware.RequireAuth(authService, connectionHandler.GetSentRequests))
	mux.HandleFunc("/api/connections/status", middleware.RequireAuth(authService, connectionHandler.GetStatus))
	mux.HandleFunc("/api/connections/block", middleware.RequireAuth(authService, connectionHandler.BlockConnection))

	mux.HandleFunc("/api/reminders", middleware.RequireAuth(authService, reminderHandler.GetReminders))
	mux.HandleFunc("/api/reminder/create", middleware.RequireAuth(authService, reminderHandler.CreateReminder))
	mux.HandleFunc("/api/reminder/", middleware.RequireAuth(authService, reminderHandler.GetReminder))
	mux.HandleFunc("/api/reminder/update/", middleware.RequireAuth(authService, reminderHandler.UpdateReminder))
	mux.HandleFunc("/api/reminder/delete/", middleware.RequireAuth(authService, reminderHandler.DeleteReminder))
	mux.HandleFunc("/api/reminder/pending", middleware.RequireAuth(authService, reminderHandler.GetPendingReminders))

	mux.HandleFunc("/api/message/read", middleware.RequireAuth(authService, readReceiptHandler.MarkMessageRead))
	mux.HandleFunc("/api/message/read/receipts", middleware.RequireAuth(authService, readReceiptHandler.GetReadReceipts))
	mux.HandleFunc("/api/message/read/last", middleware.RequireAuth(authService, readReceiptHandler.GetLastReadMessage))

	mux.HandleFunc("/api/room/settings", middleware.RequireAuth(authService, roomSettingsHandler.GetRoomSettings))
	mux.HandleFunc("/api/room/settings/update", middleware.RequireAuth(authService, roomSettingsHandler.UpdateRoomSettings))

	mux.HandleFunc("/api/scheduled-messages", middleware.RequireAuth(authService, scheduledMessageHandler.GetScheduledMessages))
	mux.HandleFunc("/api/scheduled-message/create", middleware.RequireAuth(authService, scheduledMessageHandler.CreateScheduledMessage))
	mux.HandleFunc("/api/scheduled-message", middleware.RequireAuth(authService, scheduledMessageHandler.GetScheduledMessage))
	mux.HandleFunc("/api/scheduled-message/update", middleware.RequireAuth(authService, scheduledMessageHandler.UpdateScheduledMessage))
	mux.HandleFunc("/api/scheduled-message/delete", middleware.RequireAuth(authService, scheduledMessageHandler.DeleteScheduledMessage))
	mux.HandleFunc("/api/scheduled-message/cancel", middleware.RequireAuth(authService, scheduledMessageHandler.CancelScheduledMessage))

	mux.HandleFunc("/api/sticker-pack/create", middleware.RequireAuth(authService, stickerPackHandler.CreateStickerPack))
	mux.HandleFunc("/api/sticker-packs", middleware.RequireAuth(authService, stickerPackHandler.GetStickerPacks))

	mux.HandleFunc("/api/room/rating/create", middleware.RequireAuth(authService, roomRatingHandler.CreateRating))
	mux.HandleFunc("/api/room/ratings", middleware.RequireAuth(authService, roomRatingHandler.GetRoomRatings))

	mux.HandleFunc("/api/activity/log", middleware.RequireAuth(authService, userActivityLogHandler.LogActivity))
	mux.HandleFunc("/api/activity/history", middleware.RequireAuth(authService, userActivityLogHandler.GetUserActivity))

	mux.HandleFunc("/api/room/metric/update", middleware.RequireAuth(authService, roomMetricHandler.UpdateMetric))
	mux.HandleFunc("/api/room/metrics", middleware.RequireAuth(authService, roomMetricHandler.GetRoomMetrics))

	mux.HandleFunc("/api/notifications", middleware.RequireAuth(authService, notificationHandler.GetNotifications))
	mux.HandleFunc("/api/notification/unread-count", middleware.RequireAuth(authService, notificationHandler.GetUnreadCount))
	mux.HandleFunc("/api/notification/read", middleware.RequireAuth(authService, notificationHandler.MarkAsRead))
	mux.HandleFunc("/api/notification/read-all", middleware.RequireAuth(authService, notificationHandler.MarkAllAsRead))
	mux.HandleFunc("/api/notification/archive", middleware.RequireAuth(authService, notificationHandler.ArchiveNotification))
	mux.HandleFunc("/api/notification/delete", middleware.RequireAuth(authService, notificationHandler.DeleteNotification))
	mux.HandleFunc("/api/notification/create", middleware.RequireAuth(authService, notificationHandler.CreateNotification))

	mux.HandleFunc("/api/message/edit-history", middleware.RequireAuth(authService, editHistoryHandler.GetMessageEditHistory))

	mux.HandleFunc("/api/room-bookmarks", middleware.RequireAuth(authService, roomBookmarkHandler.GetRoomBookmarks))
	mux.HandleFunc("/api/room-bookmark/create", middleware.RequireAuth(authService, roomBookmarkHandler.CreateRoomBookmark))
	mux.HandleFunc("/api/room-bookmark/update", middleware.RequireAuth(authService, roomBookmarkHandler.UpdateRoomBookmark))
	mux.HandleFunc("/api/room-bookmark/delete", middleware.RequireAuth(authService, roomBookmarkHandler.DeleteRoomBookmark))

	mux.HandleFunc("/api/notification-preferences", middleware.RequireAuth(authService, notificationPrefsHandler.GetPreferences))
	mux.HandleFunc("/api/notification-preferences/update", middleware.RequireAuth(authService, notificationPrefsHandler.UpdatePreferences))

	mux.HandleFunc("/api/attachment/share", middleware.RequireAuth(authService, attachmentHandler.ShareAttachment))
	mux.HandleFunc("/api/attachment/shares", middleware.RequireAuth(authService, attachmentHandler.GetAttachmentShares))
	mux.HandleFunc("/api/attachment/shared", attachmentHandler.GetSharedAttachment)
	mux.HandleFunc("/api/attachment/share/delete", middleware.RequireAuth(authService, attachmentHandler.DeleteShare))

	mux.HandleFunc("/api/room-template/create", middleware.RequireAuth(authService, roomTemplateHandler.CreateTemplate))
	mux.HandleFunc("/api/room-templates", middleware.RequireAuth(authService, roomTemplateHandler.GetTemplates))
	mux.HandleFunc("/api/room-template", middleware.RequireAuth(authService, roomTemplateHandler.GetTemplate))
	mux.HandleFunc("/api/room-template/update", middleware.RequireAuth(authService, roomTemplateHandler.UpdateTemplate))
	mux.HandleFunc("/api/room-template/delete", middleware.RequireAuth(authService, roomTemplateHandler.DeleteTemplate))
	mux.HandleFunc("/api/room-template/create-room", middleware.RequireAuth(authService, roomTemplateHandler.CreateRoomFromTemplate))

	mux.HandleFunc("/api/category-permission/create", middleware.RequireAuth(authService, categoryPermissionHandler.CreatePermission))
	mux.HandleFunc("/api/category-permissions", middleware.RequireAuth(authService, categoryPermissionHandler.GetCategoryPermissions))
	mux.HandleFunc("/api/category-permission/update", middleware.RequireAuth(authService, categoryPermissionHandler.UpdatePermission))
	mux.HandleFunc("/api/category-permission/delete", middleware.RequireAuth(authService, categoryPermissionHandler.DeletePermission))
	mux.HandleFunc("/api/category-permission/check", middleware.RequireAuth(authService, categoryPermissionHandler.CheckPermission))

	mux.HandleFunc("/api/mentions", middleware.RequireAuth(authService, mentionHandler.GetUserMentions))
	mux.HandleFunc("/api/mentions/message", middleware.RequireAuth(authService, mentionHandler.GetMessageMentions))

	mux.HandleFunc("/api/room/transfer-ownership", middleware.RequireAuth(authService, roomTransferHandler.TransferOwnership))

	mux.HandleFunc("/api/announcements", middleware.RequireAuth(authService, announcementHandler.GetAnnouncements))
	mux.HandleFunc("/api/announcement/active", middleware.RequireAuth(authService, announcementHandler.GetActiveAnnouncement))
	mux.HandleFunc("/api/announcement/create", middleware.RequireAuth(authService, announcementHandler.CreateAnnouncement))
	mux.HandleFunc("/api/announcement/update", middleware.RequireAuth(authService, announcementHandler.UpdateAnnouncement))
	mux.HandleFunc("/api/announcement/delete", middleware.RequireAuth(authService, announcementHandler.DeleteAnnouncement))
	mux.HandleFunc("/api/announcement/deactivate", middleware.RequireAuth(authService, announcementHandler.DeactivateAnnouncement))

	mux.HandleFunc("/api/typing/start", middleware.RequireAuth(authService, typingHandler.StartTyping))
	mux.HandleFunc("/api/typing/stop", middleware.RequireAuth(authService, typingHandler.StopTyping))
	mux.HandleFunc("/api/typing/users", middleware.RequireAuth(authService, typingHandler.GetTypingUsers))

	mux.HandleFunc("/api/voice-session/create", middleware.RequireAuth(authService, voiceSessionHandler.CreateVoiceSession))
	mux.HandleFunc("/api/voice-session/end", middleware.RequireAuth(authService, voiceSessionHandler.EndVoiceSession))
	mux.HandleFunc("/api/voice-sessions", middleware.RequireAuth(authService, voiceSessionHandler.GetUserVoiceSessions))
	mux.HandleFunc("/api/voice-sessions/room", middleware.RequireAuth(authService, voiceSessionHandler.GetRoomVoiceSessions))
	mux.HandleFunc("/api/voice-sessions/stats", middleware.RequireAuth(authService, voiceSessionHandler.GetVoiceSessionStats))

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprint(w, "OK"); err != nil {
			log.Printf("Error writing health response: %v", err)
		}
	})

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		metrics := map[string]interface{}{
			"uptime":         time.Since(startTime).String(),
			"active_clients": hub.GetClientCount(),
			"room_count":     hub.GetRoomCount(),
			"redis_enabled":  hub.IsRedisEnabled(),
			"dm_clients":     dmHub.GetActiveClients(),
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(metrics); err != nil {
			log.Printf("Error encoding metrics: %v", err)
		}
	})

	mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
		info := map[string]interface{}{
			"version":       "1.0.0",
			"server_time":   time.Now().UTC().Format(time.RFC3339),
			"uptime":        time.Since(startTime).String(),
			"go_version":    runtime.Version(),
			"ws_clients":    hub.GetClientCount(),
			"ws_rooms":      hub.GetRoomCount(),
			"redis_enabled": hub.IsRedisEnabled(),
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(info); err != nil {
			log.Printf("Error encoding info: %v", err)
		}
	})

	if os.Getenv("ENABLE_PPROF") == "true" {
		log.Println("WARNING: pprof debug endpoints are enabled. Do not use in production.")
		mux.Handle("/debug/pprof/", middleware.RequireAuth(authService, http.DefaultServeMux.ServeHTTP))
		mux.Handle("/debug/pprof/heap", middleware.RequireAuth(authService, http.DefaultServeMux.ServeHTTP))
		mux.Handle("/debug/pprof/goroutine", middleware.RequireAuth(authService, http.DefaultServeMux.ServeHTTP))
		mux.Handle("/debug/pprof/block", middleware.RequireAuth(authService, http.DefaultServeMux.ServeHTTP))
		mux.Handle("/debug/pprof/mutex", middleware.RequireAuth(authService, http.DefaultServeMux.ServeHTTP))
	}

	mux.Handle("/metrics", middleware.RequireAuth(authService, promhttp.Handler().ServeHTTP))

	corsMiddleware := middleware.NewCORSMiddleware(cfg.Server.AllowedOrigins)

	var handler http.Handler = mux
	handler = middleware.RequestID()(handler)
	handler = middleware.GzipCompression()(handler)
	handler = middleware.LimitRequestBody(1 << 20)(handler)
	handler = middleware.SentryRecovery()(handler)
	if cfg.Sentry.DSN != "" {
		sentryHandler := sentryhttp.New(sentryhttp.Options{
			Repanic:         true,
			WaitForDelivery: false,
			Timeout:         5 * time.Second,
		})
		handler = sentryHandler.Handle(handler)
	}
	handler = corsMiddleware.Handler(handler)
	handler = middleware.SecurityHeaders(handler)

	addr := fmt.Sprintf(":%s", cfg.Server.Port)
	log.Printf("Server starting on %s", addr)

	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	log.Printf("Server started successfully")
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	if cfg.Sentry.DSN != "" {
		sentry.Flush(2 * time.Second)
	}

	hub.Shutdown()
	dmHub.Shutdown()
	if err := hub.ShutdownRedis(); err != nil {
		log.Printf("Error shutting down Redis: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited properly")
}

func bootstrapAdminUser(db *database.Database, authService *auth.AuthService, adminCfg *config.AdminConfig) {
	if adminCfg.Username == "" {
		log.Println("Admin username not configured, skipping admin creation")
		return
	}

	var userCount int64
	db.DB.Model(&models.User{}).Count(&userCount)

	if userCount == 0 && adminCfg.FirstIsAdmin {
		log.Println("No users found - first user registration will grant admin access")
		return
	}

	if adminCfg.Password == "" {
		log.Println("ADMIN_PASSWORD not set, skipping default admin creation")
		return
	}

	var existingAdmin models.User
	result := db.DB.Where("username = ?", adminCfg.Username).First(&existingAdmin)

	if result.Error == nil {
		log.Printf("Admin user '%s' already exists", adminCfg.Username)
		return
	}

	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		log.Printf("Error checking for admin user: %v", result.Error)
		return
	}

	hashedPassword, err := authService.HashPassword(adminCfg.Password)
	if err != nil {
		log.Printf("Failed to hash admin password: %v", err)
		return
	}

	admin := models.User{
		Username:     adminCfg.Username,
		Email:        adminCfg.Email,
		PasswordHash: hashedPassword,
		DisplayName:  adminCfg.Username,
		IsAdmin:      true,
	}

	if err := db.DB.Create(&admin).Error; err != nil {
		log.Printf("Failed to create admin user: %v", err)
		return
	}

	log.Printf("Created default admin user: %s", adminCfg.Username)
}
