package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/api"
	"github.com/291-Group/LAN-Orangutan/internal/auth"
	"github.com/291-Group/LAN-Orangutan/internal/network"
	"github.com/291-Group/LAN-Orangutan/internal/storage"
	"github.com/291-Group/LAN-Orangutan/internal/web"
)

var (
	servePort          int
	serveBind          string
	serveAllowInsecure bool
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
	serveCmd.Flags().BoolVar(&serveAllowInsecure, "allow-insecure", false,
		"Permit binding to the network without a password (not recommended)")
}

func runServe(cmd *cobra.Command, args []string) error {
	// Flags parsed cleanly, so anything that fails from here is a runtime
	// problem, not a usage error. Report it without the flag listing.
	cmd.SilenceUsage = true

	// Use flags or config
	port := cfg.Server.Port
	if servePort > 0 {
		port = servePort
	}
	bind := cfg.Server.BindAddress
	if serveBind != "" {
		bind = serveBind
	}

	// Flags win over the config file and environment.
	cfg.Server.BindAddress = bind
	if serveAllowInsecure {
		cfg.Server.AllowInsecure = true
	}

	// Initialize storage
	store, err := storage.New(cfg.DevicesFile(), cfg.StateFile())
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}

	// A password from the config file or environment wins. Otherwise fall back
	// to one created earlier through the setup page.
	password := cfg.Server.Password
	if password == "" {
		password = auth.LoadHash(cfg.PasswordFile())
		cfg.Server.Password = password
	}

	authn, err := auth.New(password, cfg.SessionTTL())
	if err != nil {
		return fmt.Errorf("failed to set up authentication: %w", err)
	}
	authn.SetSetupRequired(cfg.RequiresSetup())

	// Create HTTP handler
	mux := http.NewServeMux()

	webHandler := web.NewHandler(store, cfg, authn, Version)
	apiHandler := api.NewHandler(store, cfg)

	// Protected routes.
	mux.Handle("/api/", authn.Middleware(apiHandler))
	mux.Handle("/", authn.Middleware(webHandler))

	// Public routes. Static assets stay open so the login and setup pages can
	// style themselves, and those forms must be reachable while signed out.
	mux.Handle("/static/", webHandler.StaticHandler())
	mux.HandleFunc(auth.LoginPath, webHandler.HandleLogin)
	mux.HandleFunc(auth.LogoutPath, webHandler.HandleLogout)
	mux.HandleFunc(auth.SetupPath, webHandler.HandleSetup(func(hash string) error {
		return auth.SaveHash(cfg.PasswordFile(), hash)
	}))

	// Create server. JoinHostPort rather than "host:port": an IPv6 address
	// contains colons of its own and has to be bracketed.
	addr := net.JoinHostPort(bind, strconv.Itoa(port))
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Claim the port before announcing anything. Printing "Starting..." and the
	// whole status banner first means a failure to bind reads as though the
	// server came up and then died.
	listener, err := net.Listen(listenNetwork(bind), addr)
	if err != nil {
		if isAddrInUse(err) {
			return fmt.Errorf("port %d is already in use. Stop whatever is using it, or choose another with --port", port)
		}
		return fmt.Errorf("could not listen on %s: %w", addr, err)
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

	// Be explicit about who can reach this and whether it is protected, so
	// nobody has to guess at their own exposure.
	if cfg.IsLoopbackBind() {
		fmt.Println("Reachable from: this machine only")
	} else {
		// Name the actual addresses. "0.0.0.0" tells the user nothing about
		// which networks they have just published this on.
		reachable := listenAddresses(bind, port)
		if len(reachable) == 0 {
			fmt.Printf("Reachable from: any machine that can reach %s\n", bind)
		} else {
			fmt.Println("Reachable at:")
			for _, a := range reachable {
				fmt.Printf("  http://%s\n", a)
			}
		}
	}

	switch {
	case authn.NeedsSetup():
		fmt.Println("Password:       not set yet, open the page above to create one")
	case authn.Enabled():
		fmt.Println("Password:       required")
	case cfg.IsLoopbackBind():
		fmt.Println("Password:       not set (not needed while local only)")
	default:
		fmt.Println("Password:       NOT SET, and this server is exposed to the network")
	}

	// A container attached to Docker's own network cannot see the LAN, and the
	// virtual gateway answers probes for addresses that do not exist. Say so
	// before the user runs a scan and trusts the results.
	if detected, err := network.DetectNetworks(); err == nil {
		detected = network.WithConfigured(detected, cfg.Scanning.Networks)
		if warning := network.IsolationWarning(detected); warning != "" {
			fmt.Println()
			fmt.Println("  ┌─────────────────────────────────────────────────────────────┐")
			fmt.Println("  │  WARNING: no access to your local network                   │")
			fmt.Println("  └─────────────────────────────────────────────────────────────┘")
			fmt.Println()
			fmt.Println("  This container can only see Docker's own network.")
			fmt.Println("  Scans will miss your real devices, and may report devices that")
			fmt.Println("  do not exist, because Docker's gateway answers on their behalf.")
			fmt.Println()
			fmt.Println("  Linux:           use host networking (see docker-compose.yml)")
			fmt.Println("  macOS / Windows: Docker cannot reach your LAN at all.")
			fmt.Println("                   Run LAN Orangutan directly instead.")
			fmt.Println()
		}
	}

	fmt.Println("Press Ctrl+C to stop")

	if err := server.Serve(listener); err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	<-done
	fmt.Println("Server stopped")
	return nil
}

// isAddrInUse reports whether err is the operating system refusing a port
// because something else already holds it.
func isAddrInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}

// listenNetwork picks the address family to listen on.
//
// Go treats "0.0.0.0" as a wildcard and answers it with a dual-stack IPv6
// socket, so a user who asked for an IPv4 address would also be listening on
// every IPv6 address the machine holds. On a home network IPv4 sits behind NAT
// while IPv6 frequently does not, so that quietly turns a LAN service into one
// that may be reachable from the internet.
//
// Bind the family that was actually requested. "::" still gives both, for
// anyone who wants that.
func listenNetwork(bind string) string {
	ip := net.ParseIP(strings.Trim(bind, "[]"))
	switch {
	case ip == nil:
		return "tcp" // a hostname; let the resolver decide
	case ip.To4() != nil:
		return "tcp4"
	default:
		return "tcp6"
	}
}

// listenAddresses returns the addresses this machine can actually be reached on
// for a wildcard bind, so the startup banner can name them instead of printing
// "0.0.0.0" and leaving the user to guess.
func listenAddresses(bind string, port int) []string {
	ip := net.ParseIP(strings.Trim(bind, "[]"))
	if ip != nil && !ip.IsUnspecified() {
		return []string{net.JoinHostPort(bind, fmt.Sprint(port))}
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}

	wantV4 := ip == nil || ip.To4() != nil
	var out []string
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok || n.IP.IsLoopback() || n.IP.IsLinkLocalUnicast() {
			continue
		}
		isV4 := n.IP.To4() != nil
		if isV4 != wantV4 {
			continue
		}
		out = append(out, net.JoinHostPort(n.IP.String(), fmt.Sprint(port)))
	}
	return out
}
