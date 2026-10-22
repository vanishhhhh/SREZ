package main

import (
	"log"

	"srez/internal/config"
	"srez/internal/handler"
)

func main() {
	cfg := config.Must()
	h := handler.New()

	log.Printf("HTTP-сервер запускается на %s", cfg.Addr)
	if err := h.Run(cfg.Addr); err != nil {
		log.Fatalf("не удалось запустить HTTP-сервер: %v", err)
	}
}
