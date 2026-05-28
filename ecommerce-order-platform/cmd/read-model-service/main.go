package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	readhandlers "ecommerce-order-platform/internal/readmodel/handlers"
	readkafka "ecommerce-order-platform/internal/readmodel/kafka"
	"ecommerce-order-platform/internal/readmodel/repository"
	"ecommerce-order-platform/internal/readmodel/service"
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
		Host:            config.String("READ_DB_HOST", "localhost"),
		Port:            config.Int("READ_DB_PORT", 5432),
		User:            config.String("READ_DB_USER", "ecommerce"),
		Password:        config.String("READ_DB_PASSWORD", "ecommerce"),
		Database:        config.String("READ_DB_NAME", "read_db"),
		SSLMode:         config.String("READ_DB_SSLMODE", "disable"),
		MaxConns:        int32(config.Int("READ_DB_MAX_CONNS", 10)),
		ConnectTimeout:  config.Duration("READ_DB_CONNECT_TIMEOUT", 5*time.Second),
		HealthcheckTime: config.Duration("READ_DB_HEALTHCHECK_INTERVAL", 30*time.Second),
	})
	if err != nil {
		log.Fatalf("connect read db: %v", err)
	}
	defer pool.Close()

	kafkaBrokers := config.CSV("KAFKA_BROKERS", []string{"localhost:9092"})
	repo := repository.New(pool)
	queryService := service.New(repo)

	consumers := newConsumerRunners(kafkaBrokers, queryService)
	for _, runner := range consumers {
		go runner.Run(ctx)
	}
	defer closeConsumers(consumers)

	mux := http.NewServeMux()
	readhandlers.New(queryService).Register(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:              config.String("READ_MODEL_SERVICE_ADDR", ":8085"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown read-model-service http server: %v", err)
		}
	}()

	log.Printf("read-model-service listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("run read-model-service http server: %v", err)
	}
}

type closeableRunner struct {
	*readkafka.ConsumerRunner
	consumer *sharedkafka.Consumer
}

func newConsumerRunners(brokers []string, handler readkafka.EventHandler) []closeableRunner {
	topics := []string{
		events.TopicOrders,
		events.TopicInventory,
		events.TopicPayments,
		events.TopicDelivery,
		events.TopicNotifications,
	}
	runners := make([]closeableRunner, 0, len(topics))

	for _, topic := range topics {
		consumer := sharedkafka.NewConsumer(sharedkafka.ConsumerConfig{
			Brokers: brokers,
			Topic:   topic,
			GroupID: config.String("KAFKA_GROUP_ID", "read-model-service"),
		})
		runners = append(runners, closeableRunner{
			ConsumerRunner: readkafka.NewConsumerRunner(topic, consumer, handler),
			consumer:       consumer,
		})
	}

	return runners
}

func closeConsumers(runners []closeableRunner) {
	for _, runner := range runners {
		if err := runner.consumer.Close(); err != nil {
			log.Printf("close readmodel kafka consumer: %v", err)
		}
	}
}
