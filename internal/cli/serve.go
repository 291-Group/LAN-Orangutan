package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/api"
	"github.com/291-Group/LAN-Orangutan/internal/storage"
	"github.com/291-Group/LAN-Orangutan/internal/web"
)

var (
	servePort int
	serveBind string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the web server",
	Long:  `Start the LAN Orangutan web server with REST API and dashboard.`,
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 0, "Port to listen on (default from config)")
	serveCmd.Flags().StringVarP(&serveBind, "bind", "b", "", "Address to bind to (default from config)")
}

func runServe(cmd *cobra.Command, args []string) error {
	// Use flags or config
	port := cfg.Server.Port
	if servePort > 0 {
		port = servePort
	}
	bind := cfg.Server.BindAddress
	if serveBind != "" {
		bind = serveBind
	}

	// Initialize storage
	store, err := storage.New(cfg.DevicesFile(), cfg.StateFile())
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Create HTTP handler
	mux := http.NewServeMux()

	// Register API routes
	apiHandler := api.NewHandler(store, cfg)
	mux.Handle("/api/", apiHandler)

	// Register web routes
	webHandler := web.NewHandler(store, cfg)
	mux.Handle("/", webHandler)

	// Create server
	addr := fmt.Sprintf("%s:%d", bind, port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Handle shutdown gracefully
	done := make(chan bool, 1)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		fmt.Println("\nShutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		if err := server.Shutdown(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Error shutting down: %v\n", err)
		}
		close(done)
	}()

	fmt.Printf("Starting LAN Orangutan server on http://%s\n", addr)
	fmt.Println("Press Ctrl+C to stop")

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	<-done
	fmt.Println("Server stopped")
	return nil
}
