package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	inventorykafka "ecommerce-order-platform/internal/inventory/kafka"
	"ecommerce-order-platform/internal/inventory/repository"
	"ecommerce-order-platform/internal/inventory/service"
	"ecommerce-order-platform/pkg/config"
	"ecommerce-order-platform/pkg/db"
	"ecommerce-order-platform/pkg/events"
	sharedkafka "ecommerce-order-platform/pkg/kafka"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPostgresPool(ctx, db.PostgresConfig{
		URL:             config.String("DATABASE_URL", ""),
		Host:            config.String("INVENTORY_DB_HOST", "localhost"),
		Port:            config.Int("INVENTORY_DB_PORT", 5432),
		User:            config.String("INVENTORY_DB_USER", "ecommerce"),
		Password:        config.String("INVENTORY_DB_PASSWORD", "ecommerce"),
		Database:        config.String("INVENTORY_DB_NAME", "inventory_db"),
		SSLMode:         config.String("INVENTORY_DB_SSLMODE", "disable"),
		MaxConns:        int32(config.Int("INVENTORY_DB_MAX_CONNS", 10)),
		ConnectTimeout:  config.Duration("INVENTORY_DB_CONNECT_TIMEOUT", 5*time.Second),
		HealthcheckTime: config.Duration("INVENTORY_DB_HEALTHCHECK_INTERVAL", 30*time.Second),
	})
	if err != nil {
		log.Fatalf("connect inventory db: %v", err)
	}
	defer pool.Close()

	kafkaBrokers := config.CSV("KAFKA_BROKERS", []string{"localhost:9092"})
	producer := sharedkafka.NewProducer(sharedkafka.ProducerConfig{
		Brokers: kafkaBrokers,
		Topic:   events.TopicInventory,
	})
	defer func() { _ = producer.Close() }()

	consumer := sharedkafka.NewConsumer(sharedkafka.ConsumerConfig{
		Brokers: kafkaBrokers,
		Topic:   events.TopicOrders,
		GroupID: config.String("KAFKA_GROUP_ID", "inventory-service"),
	})
	defer func() { _ = consumer.Close() }()

	repo := repository.New(pool)
	handler := service.New(repo, producer)
	runner := inventorykafka.NewConsumerRunner(events.TopicOrders, consumer, handler)
	go runner.Run(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:              config.String("INVENTORY_SERVICE_ADDR", ":8082"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown inventory-service http server: %v", err)
		}
	}()

	log.Printf("inventory-service listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("run inventory-service http server: %v", err)
	}
}
