package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/devanmcgeer/whatsapp-sidecar/backend/internal/api"
	"github.com/devanmcgeer/whatsapp-sidecar/backend/internal/store"
	"github.com/devanmcgeer/whatsapp-sidecar/backend/internal/summarizer"
	"github.com/devanmcgeer/whatsapp-sidecar/backend/internal/whatsapp"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	ollamaURL := envOr("OLLAMA_URL", "http://localhost:11434")
	ollamaModel := envOr("OLLAMA_MODEL", "gemma3:1b")
	listenAddr := envOr("LISTEN_ADDR", "0.0.0.0:8080")
	dbPath := envOr("DB_PATH", "whatsapp-sidecar.db")
	sessionDBPath := envOr("SESSION_DB_PATH", "whatsapp-session.db")
	summaryInterval := envOr("SUMMARY_INTERVAL", "6h")

	interval, err := time.ParseDuration(summaryInterval)
	if err != nil {
		log.Fatalf("invalid SUMMARY_INTERVAL: %v", err)
	}

	s, err := store.New(ctx, dbPath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer s.Close()

	sum := summarizer.New(s, ollamaURL, ollamaModel)

	handler := api.New(s, sum)

	// Start HTTP server first so the UI is accessible during pairing.
	srv := &http.Server{Addr: listenAddr, Handler: handler}
	go func() {
		log.Printf("listening on %s", listenAddr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
	}()

	wac, err := whatsapp.New(ctx, sessionDBPath, s)
	if err != nil {
		log.Fatalf("whatsapp: %v", err)
	}
	defer wac.Disconnect()

	if !wac.IsLoggedIn() {
		qrChan, err := wac.GetQRChannel(ctx)
		if err != nil {
			log.Fatalf("qr channel: %v", err)
		}
		if err := wac.Connect(); err != nil {
			log.Fatalf("connect: %v", err)
		}
		log.Println("waiting for QR scan — open /setup in the UI")
		for evt := range qrChan {
			if evt.Event == "code" {
				handler.SetQRCode(evt.Code)
			} else {
				log.Printf("QR event: %s", evt.Event)
				handler.SetQRCode("")
			}
		}
	} else {
		if err := wac.Connect(); err != nil {
			log.Fatalf("connect: %v", err)
		}
		log.Println("already logged in")
	}

	go sum.RunPeriodic(ctx, interval)

	<-ctx.Done()
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
