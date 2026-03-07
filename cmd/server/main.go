package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/enclavr/server/internal/auth"
	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/handlers"
	"github.com/enclavr/server/internal/metrics"
	"github.com/enclavr/server/internal/services"
	"github.com/enclavr/server/internal/websocket"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var startTime = time.Now()

func main() {
	cfg := config.Load()

	db, err := database.New(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	if err := db.Migrate(); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	authService := auth.NewAuthService(&cfg.Auth)

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
	inviteHandler := handlers.NewInviteHandler(db)

	authHandler := handlers.NewAuthHandler(db, authService)
	roomHandler := handlers.NewRoomHandler(db)
	voiceHandler := handlers.NewVoiceHandler(db, hub, cfg)
	oidcHandler := handlers.NewOIDCHandler(db, &cfg.Auth)
	messageHandler := handlers.NewMessageHandler(db, hub)
	presenceHandler := handlers.NewPresenceHandler(db)
	dmHandler := handlers.NewDirectMessageHandler(db)
	userHandler := handlers.NewUserHandler(db)
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
	exportHandler := handlers.NewExportHandler(db)
	pushHandler := handlers.NewPushHandler(db)
	banHandler := handlers.NewBanHandler(db)
	reportHandler := handlers.NewReportHandler(db)
	_ = services.NewPushService(db, cfg)

	go hub.Run()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/register", authHandler.Register)
	mux.HandleFunc("/api/auth/login", authHandler.Login)
	mux.HandleFunc("/api/auth/refresh", authHandler.RefreshToken)
	mux.HandleFunc("/api/auth/me", authHandler.GetMe)

	mux.HandleFunc("/api/auth/oidc/login", oidcHandler.Login)
	mux.HandleFunc("/api/auth/oidc/callback", oidcHandler.Callback)
	mux.HandleFunc("/api/auth/oidc/config", oidcHandler.GetConfig)

	mux.HandleFunc("/api/rooms", middleware.RequireAuth(authService, roomHandler.GetRooms))
	mux.HandleFunc("/api/room/create", middleware.RequireAuth(authService, roomHandler.CreateRoom))
	mux.HandleFunc("/api/room", middleware.RequireAuth(authService, roomHandler.GetRoom))
	mux.HandleFunc("/api/room/join", middleware.RequireAuth(authService, roomHandler.JoinRoom))
	mux.HandleFunc("/api/room/leave", middleware.RequireAuth(authService, roomHandler.LeaveRoom))

	mux.HandleFunc("/api/voice/ws", voiceHandler.HandleWebSocket)
	mux.HandleFunc("/api/voice/ice", voiceHandler.GetICEConfig)

	mux.HandleFunc("/api/messages", middleware.RequireAuth(authService, messageHandler.GetMessages))
	mux.HandleFunc("/api/message/send", middleware.RequireAuth(authService, messageHandler.SendMessage))
	mux.HandleFunc("/api/message/update", middleware.RequireAuth(authService, messageHandler.UpdateMessage))
	mux.HandleFunc("/api/message/delete", middleware.RequireAuth(authService, messageHandler.DeleteMessage))

	mux.HandleFunc("/api/presence/update", middleware.RequireAuth(authService, presenceHandler.UpdatePresence))
	mux.HandleFunc("/api/presence/room", middleware.RequireAuth(authService, presenceHandler.GetPresence))
	mux.HandleFunc("/api/presence/user", middleware.RequireAuth(authService, presenceHandler.GetUserPresence))

	mux.HandleFunc("/api/dm/send", middleware.RequireAuth(authService, dmHandler.SendDM))
	mux.HandleFunc("/api/dm/conversations", middleware.RequireAuth(authService, dmHandler.GetConversations))
	mux.HandleFunc("/api/dm/messages", middleware.RequireAuth(authService, dmHandler.GetMessages))
	mux.HandleFunc("/api/dm/update", middleware.RequireAuth(authService, dmHandler.UpdateDM))
	mux.HandleFunc("/api/dm/delete", middleware.RequireAuth(authService, dmHandler.DeleteDM))

	mux.HandleFunc("/api/users/search", middleware.RequireAuth(authService, userHandler.SearchUsers))

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

	mux.HandleFunc("/api/settings", settingsHandler.GetSettings)
	mux.HandleFunc("/api/settings/update", middleware.RequireAuth(authService, settingsHandler.UpdateSettings))

	mux.HandleFunc("/api/invite/create", middleware.RequireAuth(authService, inviteHandler.CreateInvite))
	mux.HandleFunc("/api/invites", middleware.RequireAuth(authService, inviteHandler.GetInvites))
	mux.HandleFunc("/api/invite/use", middleware.RequireAuth(authService, inviteHandler.UseInvite))
	mux.HandleFunc("/api/invite/revoke", middleware.RequireAuth(authService, inviteHandler.RevokeInvite))

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
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(metrics); err != nil {
			log.Printf("Error encoding metrics: %v", err)
		}
	})

	mux.Handle("/debug/pprof/", http.HandlerFunc(http.DefaultServeMux.ServeHTTP))
	mux.Handle("/debug/pprof/heap", http.HandlerFunc(http.DefaultServeMux.ServeHTTP))
	mux.Handle("/debug/pprof/goroutine", http.HandlerFunc(http.DefaultServeMux.ServeHTTP))
	mux.Handle("/debug/pprof/block", http.HandlerFunc(http.DefaultServeMux.ServeHTTP))
	mux.Handle("/debug/pprof/mutex", http.HandlerFunc(http.DefaultServeMux.ServeHTTP))

	mux.Handle("/metrics", promhttp.Handler())

	corsMiddleware := middleware.NewCORSMiddleware(cfg.Server.AllowedOrigins)

	var handler http.Handler = mux
	handler = middleware.RequestID()(handler)
	handler = middleware.GzipCompression()(handler)
	handler = corsMiddleware.Handler(handler)
	handler = middleware.SecurityHeaders(handler)

	addr := fmt.Sprintf(":%s", cfg.Server.Port)
	log.Printf("Server starting on %s", addr)

	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
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

	hub.Shutdown()
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
