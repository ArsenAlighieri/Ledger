package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"ledger/internal/app"
)

func main() {
	log.Println("==================================================")
	log.Println("  LEDGER — Kişisel Finans & Varlık Yönetimi")
	log.Println("==================================================")

	cfg := app.LoadConfig()
	application, err := app.NewApp(cfg)
	if err != nil {
		log.Fatalf("[LEDGER HATA] Uygulama başlatılamadı: %v", err)
	}
	defer application.Close()

	// Graceful shutdown channel
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := application.Start(); err != nil {
			log.Printf("[LEDGER] Sunucu durduruldu: %v", err)
		}
	}()

	<-stop
	log.Println("[LEDGER] Güvenli kapatma sinyali alındı, işlemler tamamlanıyor...")
	application.Close()
	log.Println("[LEDGER] Uygulama başarıyla kapatıldı.")
}
