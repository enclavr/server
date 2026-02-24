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

	authHandler := handlers.NewAuthHandler(db, authService)
	roomHandler := handlers.NewRoomHandler(db)
	voiceHandler := handlers.NewVoiceHandler(db, hub)
	oidcHandler := handlers.NewOIDCHandler(db, &cfg.Auth)

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

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
	})

	addr := fmt.Sprintf(":%s", cfg.Server.Port)
	log.Printf("Server starting on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
