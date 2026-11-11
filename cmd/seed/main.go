// Command seed explicitly loads local demo data. It is never run by the server.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"srez/internal/devseed"
	"srez/migrations"
)

func main() {
	if os.Getenv("SREZ_DEV_SEED") != "true" {
		log.Fatal("seed разрешён только при SREZ_DEV_SEED=true для локальной базы")
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("задайте DATABASE_URL локальной базы")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "postgres", url)
	if err != nil {
		log.Fatal("не удалось подключиться к локальной базе")
	}
	defer db.Close()
	if err = migrations.Apply(ctx, db); err != nil {
		log.Fatalf("миграции: %v", err)
	}
	if err = devseed.Apply(ctx, db); err != nil {
		log.Fatalf("seed: %v", err)
	}
	log.Print("dev seed применён; существующие записи сохранены")
}
