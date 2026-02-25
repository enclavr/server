package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/enclavr/server/internal/auth"
	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/handlers"
	"github.com/enclavr/server/internal/websocket"
	"github.com/enclavr/server/pkg/middleware"
)

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
	hub := websocket.NewHub()
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

	mux.HandleFunc("/api/messages/search", middleware.RequireAuth(authService, messageHandler.SearchMessages))

	mux.HandleFunc("/api/files/upload", middleware.RequireAuth(authService, fileHandler.UploadFile))
	mux.HandleFunc("/api/files", middleware.RequireAuth(authService, fileHandler.GetRoomFiles))
	mux.HandleFunc("/api/files/delete", middleware.RequireAuth(authService, fileHandler.DeleteFile))
	mux.HandleFunc("/api/files/", fileHandler.GetFile)

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
	})

	corsMiddleware := middleware.NewCORSMiddleware(cfg.Server.AllowedOrigins)

	var handler http.Handler = mux
	handler = corsMiddleware.Handler(handler)
	handler = middleware.SecurityHeaders(handler)

	addr := fmt.Sprintf(":%s", cfg.Server.Port)
	log.Printf("Server starting on %s", addr)
	log.Fatal(http.ListenAndServe(addr, handler))
}
