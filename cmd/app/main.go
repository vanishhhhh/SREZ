package main

import (
	"context"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"srez/internal/config"
	"srez/internal/handler"
	"srez/internal/storage"
	"srez/internal/usecase"
	"srez/migrations"
)

func main() {
	cfg := config.Must()
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
	h := handler.New(users, brands, products, cfg.DevCatalogWrite)

	if cfg.DevCatalogWrite {
		log.Print("ВНИМАНИЕ: включён локальный POST /products без авторизации")
	}
	log.Printf("HTTP-сервер запускается на %s", cfg.Addr)
	if err := h.Run(cfg.Addr); err != nil {
		log.Fatalf("не удалось запустить HTTP-сервер: %v", err)
	}
}
