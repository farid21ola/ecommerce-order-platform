package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"ecommerce-order-platform/internal/api_gateway/clients"
	"ecommerce-order-platform/internal/api_gateway/frontend"
	"ecommerce-order-platform/internal/api_gateway/handlers"
	"ecommerce-order-platform/pkg/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	orderProxy, err := clients.NewReverseProxy(config.String("ORDER_SERVICE_URL", "http://localhost:8081"))
	if err != nil {
		log.Fatalf("create order-service proxy: %v", err)
	}

	readProxy, err := clients.NewReverseProxy(config.String("READ_MODEL_SERVICE_URL", "http://localhost:8085"))
	if err != nil {
		log.Fatalf("create read-model-service proxy: %v", err)
	}

	mux := http.NewServeMux()
	handlers.New(orderProxy, readProxy, frontend.Handler()).Register(mux)

	server := &http.Server{
		Addr:              config.String("API_GATEWAY_ADDR", ":8080"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown api-gateway http server: %v", err)
		}
	}()

	log.Printf("api-gateway listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("run api-gateway http server: %v", err)
	}
}
