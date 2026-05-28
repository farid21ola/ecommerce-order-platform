package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	notificationkafka "ecommerce-order-platform/internal/notification/kafka"
	"ecommerce-order-platform/internal/notification/repository"
	"ecommerce-order-platform/internal/notification/service"
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
		Host:            config.String("NOTIFICATION_DB_HOST", "localhost"),
		Port:            config.Int("NOTIFICATION_DB_PORT", 5432),
		User:            config.String("NOTIFICATION_DB_USER", "ecommerce"),
		Password:        config.String("NOTIFICATION_DB_PASSWORD", "ecommerce"),
		Database:        config.String("NOTIFICATION_DB_NAME", "notification_db"),
		SSLMode:         config.String("NOTIFICATION_DB_SSLMODE", "disable"),
		MaxConns:        int32(config.Int("NOTIFICATION_DB_MAX_CONNS", 10)),
		ConnectTimeout:  config.Duration("NOTIFICATION_DB_CONNECT_TIMEOUT", 5*time.Second),
		HealthcheckTime: config.Duration("NOTIFICATION_DB_HEALTHCHECK_INTERVAL", 30*time.Second),
	})
	if err != nil {
		log.Fatalf("connect notification db: %v", err)
	}
	defer pool.Close()

	kafkaBrokers := config.CSV("KAFKA_BROKERS", []string{"localhost:9092"})
	producer := sharedkafka.NewProducer(sharedkafka.ProducerConfig{
		Brokers: kafkaBrokers,
		Topic:   events.TopicNotifications,
	})
	defer func() { _ = producer.Close() }()

	repo := repository.New(pool)
	handler := service.New(repo, producer)
	consumers := newConsumerRunners(kafkaBrokers, handler)
	for _, runner := range consumers {
		go runner.Run(ctx)
	}
	defer closeConsumers(consumers)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:              config.String("NOTIFICATION_SERVICE_ADDR", ":8086"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown notification-service http server: %v", err)
		}
	}()

	log.Printf("notification-service listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("run notification-service http server: %v", err)
	}
}

type closeableRunner struct {
	*notificationkafka.ConsumerRunner
	consumer *sharedkafka.Consumer
}

func newConsumerRunners(brokers []string, handler notificationkafka.EventHandler) []closeableRunner {
	topics := []string{events.TopicOrders, events.TopicPayments, events.TopicDelivery}
	runners := make([]closeableRunner, 0, len(topics))

	for _, topic := range topics {
		consumer := sharedkafka.NewConsumer(sharedkafka.ConsumerConfig{
			Brokers: brokers,
			Topic:   topic,
			GroupID: config.String("KAFKA_GROUP_ID", "notification-service"),
		})
		runners = append(runners, closeableRunner{
			ConsumerRunner: notificationkafka.NewConsumerRunner(topic, consumer, handler),
			consumer:       consumer,
		})
	}

	return runners
}

func closeConsumers(runners []closeableRunner) {
	for _, runner := range runners {
		if err := runner.consumer.Close(); err != nil {
			log.Printf("close notification kafka consumer: %v", err)
		}
	}
}
