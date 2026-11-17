package main

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"srez/internal/config"
	"srez/internal/handler"
	"srez/internal/mailer"
	"srez/internal/objectstorage"
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
	mailCfg, err := config.LoadMail(authCfg.Origin, authCfg.LocalHTTP)
	if err != nil {
		log.Fatal("invalid mail configuration")
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
	// Bearer revocation checks add one indexed read per private request. Bound
	// the shared pool; waiting authentication requests have their own deadline.
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)
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
	h.SetShopping(usecase.NewShopping(storage.NewShopping(db)))
	orders := usecase.NewOrders(storage.NewOrders(db))
	h.SetOrders(orders)
	wishlistKey, err := config.LoadWishlistKey()
	if err != nil {
		log.Fatal("configure an independent SREZ_WISHLIST_KEY")
	}
	links, err := security.NewWishlistLinks(wishlistKey)
	if err != nil {
		log.Fatal("invalid wishlist encryption key")
	}
	h.SetWishlists(usecase.NewWishlists(storage.NewWishlists(db), links))
	if mediaCfg, enabled := config.LoadMedia(); enabled {
		objects, err := objectstorage.New(mediaCfg)
		if err != nil {
			log.Fatal("invalid media configuration")
		}
		readyCtx, readyCancel := context.WithTimeout(context.Background(), 25*time.Second)
		err = objects.EnsureBucket(readyCtx)
		if err == nil {
			err = objects.RetireStaging(readyCtx)
		}
		readyCancel()
		if err != nil {
			log.Fatal("media storage unavailable or bucket initialization failed")
		}
		media := usecase.NewMedia(storage.NewMedia(db), objects, mediaCfg.Bucket, usecase.NewManagement(storage.NewManagement(db)))
		h.SetMedia(media)
		go media.RunCleanup(context.Background(), func(e error) {
			if errors.Is(e, usecase.ErrMediaBucketMismatch) {
				log.Print("media_bucket_configuration_mismatch")
			} else {
				log.Print("media_cleanup_failed")
			}
		}, func(b usecase.MediaGCBacklog) {
			log.Printf("media_gc pending=%d oldest_seconds=%.0f reconciliation=%d", b.Pending, b.OldestSeconds, b.Reconciliation)
		})
	}
	authRepo := storage.NewAuth(db)
	sender := mailer.NewSMTP(mailCfg)
	passwords := security.NewPasswords(authCfg.HashWorkers)
	policy := usecase.VerificationPolicy{IPMinute: mailCfg.IPMinute, IPHour: mailCfg.IPHour, AccountHour: 5}
	verification := usecase.NewEmailVerification(authRepo, sender, mailCfg.FrontendBaseURL, authCfg.CSRFKey, policy)
	verification.OnDeliveryError = func() { log.Print("verification_delivery_failed") }
	auth := usecase.NewAuth(authRepo, passwords, tokens, verification)
	recovery := usecase.NewPasswordRecovery(authRepo, sender, passwords, mailCfg.FrontendBaseURL, authCfg.CSRFKey, policy)
	recovery.OnDeliveryError = func() { log.Print("recovery_delivery_failed") }
	auth.SetRecovery(recovery)
	if err := h.EnableAuth(auth, tokens, authCfg); err != nil {
		log.Fatal("invalid trusted proxies")
	}

	expiryCtx, stopExpiry := context.WithCancel(context.Background())
	defer stopExpiry()
	go orders.RunReservationExpiry(expiryCtx, 5*time.Second, func(error) { log.Print("order_reservation_expiry_failed") })

	log.Printf("HTTP-сервер запускается на %s", cfg.Addr)
	if err := h.Run(cfg.Addr); err != nil {
		log.Fatalf("не удалось запустить HTTP-сервер: %v", err)
	}
}
