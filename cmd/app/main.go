package main

import (
	"context"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"srez/internal/config"
	"srez/internal/handler"
	"srez/internal/security"
	"srez/internal/storage"
	"srez/internal/usecase"
	"srez/migrations"
)

func main() {
	cfg := config.Must()
	authCfg, err := config.LoadAuth()
	if err != nil {
		log.Fatal(err)
	}
	tokens, err := security.NewTokens(authCfg.JWTKey, authCfg.CSRFKey, authCfg.Issuer, authCfg.Audience)
	if err != nil {
		log.Fatal("invalid security keys")
	}
	if cfg.DatabaseURL == "" {
		log.Fatal("задайте DATABASE_URL или запустите проект через docker compose up --build")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	db, err := sqlx.ConnectContext(ctx, "postgres", cfg.DatabaseURL)
	cancel()
	if err != nil {
		log.Fatal("не удалось подключиться к PostgreSQL: проверьте DATABASE_URL и доступность базы")
	}
	defer db.Close()
	migrationCtx, migrationCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer migrationCancel()
	if err := migrations.Apply(migrationCtx, db); err != nil {
		log.Fatalf("не удалось применить миграции: %v", err)
	}
	brandsRepo := storage.NewBrands(db)
	brands := usecase.NewBrands(brandsRepo)
	products := usecase.NewProducts(storage.NewProducts(db), brandsRepo)
	users := usecase.NewUsers(storage.NewUsers(db))
	h := handler.New(users, brands, products)
	h.SetManagement(usecase.NewManagement(storage.NewManagement(db)))
	auth := usecase.NewAuth(storage.NewAuth(db), security.NewPasswords(authCfg.HashWorkers), tokens)
	if err := h.EnableAuth(auth, tokens, authCfg); err != nil {
		log.Fatal("invalid trusted proxies")
	}

	log.Printf("HTTP-сервер запускается на %s", cfg.Addr)
	if err := h.Run(cfg.Addr); err != nil {
		log.Fatalf("не удалось запустить HTTP-сервер: %v", err)
	}
}
