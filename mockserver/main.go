package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	s_v1 "cape-project.eu/sdk-generator/mockserver/foundation/storage/v1"
	ws_v1 "cape-project.eu/sdk-generator/mockserver/foundation/workspace/v1"
	"github.com/gin-gonic/gin"
)

func main() {
	var port int
	flag.IntVar(&port, "port", resolvePort(), "server port")
	flag.Parse()

	router := gin.Default()

	ws_v1.RegisterServer(router)
	s_v1.RegisterServer(router)

	addr := net.JoinHostPort("", strconv.Itoa(port))
	server := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
	}()

	log.Printf("mock server listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server failed: %v", err)
	}
}

func resolvePort() int {
	const defaultPort = 8080

	portValue := os.Getenv("PORT")
	if portValue == "" {
		return defaultPort
	}

	port, err := strconv.Atoi(portValue)
	if err != nil || port <= 0 || port > 65535 {
		log.Printf("invalid PORT %q, using %d", portValue, defaultPort)
		return defaultPort
	}

	return port
}
