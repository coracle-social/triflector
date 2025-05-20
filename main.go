package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

  "frith/common"
	eventstore "github.com/fiatjaf/eventstore/badger"
	"github.com/fiatjaf/khatru"
	"github.com/fiatjaf/khatru/blossom"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/afero"
)

var relay *khatru.Relay

func main() {
	common.SetupEnvironment()
	common.SetupDatabase()

	// Create context that we'll cancel on shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

  // Run GC
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		for {
			select {
			case <-ticker.C:
			again:
				err := common.Db.RunValueLogGC(0.7)
				if err == nil {
					goto again
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	defer ticker.Stop()
	defer common.Db.Close()

	// Set up our relay
	relay = khatru.NewRelay()
	relay.Info.Name = common.RELAY_NAME
	relay.Info.Icon = common.RELAY_ICON
	// relay.Info.Self = common.RELAY_SELF
	relay.Info.PubKey = common.RELAY_ADMIN
	relay.Info.Description = common.RELAY_DESCRIPTION

	// Set up our relay backend
	backend := &eventstore.BadgerBackend{Path: common.GetDataDir("events")}
	if err := backend.Init(); err != nil {
		log.Fatal("Failed to initialize backend:", err)
	}

	relay.OnConnect = append(relay.OnConnect, khatru.RequestAuth)
	relay.StoreEvent = append(relay.StoreEvent, backend.SaveEvent)
	relay.DeleteEvent = append(relay.DeleteEvent, backend.DeleteEvent)
	relay.QueryEvents = append(relay.QueryEvents, backend.QueryEvents)
	relay.QueryEvents = append(relay.QueryEvents, common.QueryEvents)
	relay.RejectEvent = append(relay.RejectEvent, common.RejectEvent)
	relay.RejectFilter = append(relay.RejectFilter, common.RejectFilter)

	// Blossom

	fs := afero.NewOsFs()
	blossomPath := common.GetDataDir("media")

	if err := fs.MkdirAll(blossomPath, 0755); err != nil {
		log.Fatal("🚫 error creating blossom path:", err)
	}

	bldb := &eventstore.BadgerBackend{Path: common.GetDataDir("blossom")}
	if err := bldb.Init(); err != nil {
		log.Fatal("Failed to initialize blossom backend:", err)
	}

	bl := blossom.New(relay, "https://"+common.RELAY_URL)

	bl.Store = blossom.EventStoreBlobIndexWrapper{Store: bldb, ServiceURL: bl.ServiceURL}

	bl.StoreBlob = append(bl.StoreBlob, func(ctx context.Context, sha256 string, body []byte) error {
		file, err := fs.Create(blossomPath + sha256)
		if err != nil {
			return err
		}

		if _, err := io.Copy(file, bytes.NewReader(body)); err != nil {
			return err
		}

		return nil
	})

	bl.LoadBlob = append(bl.LoadBlob, func(ctx context.Context, sha256 string) (io.ReadSeeker, error) {
		return fs.Open(blossomPath + sha256)
	})

	bl.DeleteBlob = append(bl.DeleteBlob, func(ctx context.Context, sha256 string) error {
		return fs.Remove(blossomPath + sha256)
	})

	bl.RejectUpload = append(bl.RejectUpload, func(ctx context.Context, auth *nostr.Event, size int, ext string) (bool, string, int) {
		if size > 10*1024*1024 {
			return true, "file too large", 413
		}

		if auth == nil || !common.HasAccess(auth.PubKey) {
			return true, "unauthorized", 403
		}

		return false, ext, size
	})

	bl.RejectGet = append(bl.RejectGet, func(ctx context.Context, auth *nostr.Event, sha256 string) (bool, string, int) {
		if auth == nil || !common.HasAccess(auth.PubKey) {
			return true, "unauthorized", 403
		}

		return false, "", 200
	})

	bl.RejectList = append(bl.RejectList, func(ctx context.Context, auth *nostr.Event, pubkey string) (bool, string, int) {
		if auth == nil || !common.HasAccess(auth.PubKey) {
			return true, "unauthorized", 403
		}

		return false, "", 200
	})

	bl.RejectDelete = append(bl.RejectDelete, func(ctx context.Context, auth *nostr.Event, sha256 string) (bool, string, int) {
		if auth == nil || !common.HasAccess(auth.PubKey) {
			return true, "unauthorized", 403
		}

		return false, "", 200
	})

	// Merge everything into a single handler and start the server

	mux := relay.Router()

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/html")
		fmt.Fprintf(w, `This is a nostr relay, please connect using wss://`)
	})

	// Create server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", common.PORT),
		Handler: relay,
	}

	// Start server in goroutine
	go func() {
		fmt.Printf("running on :%s\n", common.PORT)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v\n", err)
		}
	}()

	// Wait for interrupt signal
	<-sigChan
	fmt.Println("\nShutting down gracefully...")

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Shutdown the HTTP server
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v\n", err)
	}

	// Cancel context to stop background tasks
	cancel()
}
