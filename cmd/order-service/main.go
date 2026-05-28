package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	orderhandlers "ecommerce-order-platform/internal/order/handlers"
	orderkafka "ecommerce-order-platform/internal/order/kafka"
	"ecommerce-order-platform/internal/order/outbox"
	"ecommerce-order-platform/internal/order/repository"
	"ecommerce-order-platform/internal/order/saga"
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
		Host:            config.String("ORDER_DB_HOST", "localhost"),
		Port:            config.Int("ORDER_DB_PORT", 5432),
		User:            config.String("ORDER_DB_USER", "ecommerce"),
		Password:        config.String("ORDER_DB_PASSWORD", "ecommerce"),
		Database:        config.String("ORDER_DB_NAME", "order_db"),
		SSLMode:         config.String("ORDER_DB_SSLMODE", "disable"),
		MaxConns:        int32(config.Int("ORDER_DB_MAX_CONNS", 10)),
		ConnectTimeout:  config.Duration("ORDER_DB_CONNECT_TIMEOUT", 5*time.Second),
		HealthcheckTime: config.Duration("ORDER_DB_HEALTHCHECK_INTERVAL", 30*time.Second),
	})
	if err != nil {
		log.Fatalf("connect order db: %v", err)
	}
	defer pool.Close()

	kafkaBrokers := config.CSV("KAFKA_BROKERS", []string{"localhost:9092"})
	repo := repository.New(pool)
	service := saga.NewService(repo)

	ordersProducer := sharedkafka.NewProducer(sharedkafka.ProducerConfig{
		Brokers: kafkaBrokers,
		Topic:   events.TopicOrders,
	})
	defer func() { _ = ordersProducer.Close() }()

	outboxWorker := outbox.NewWorker(repo, map[string]*sharedkafka.Producer{
		events.TopicOrders: ordersProducer,
	}, config.Duration("OUTBOX_INTERVAL", time.Second), config.Int("OUTBOX_BATCH_SIZE", 50))
	go outboxWorker.Run(ctx)

	consumers := newConsumerRunners(kafkaBrokers, service)
	for _, runner := range consumers {
		go runner.Run(ctx)
	}
	defer closeConsumers(consumers)

	mux := http.NewServeMux()
	orderhandlers.NewOrderHandler(service).Register(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:              config.String("ORDER_SERVICE_ADDR", ":8081"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown order-service http server: %v", err)
		}
	}()

	log.Printf("order-service listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("run order-service http server: %v", err)
	}
}

type closeableRunner struct {
	*orderkafka.ConsumerRunner
	consumer *sharedkafka.Consumer
}

func newConsumerRunners(brokers []string, handler orderkafka.EventHandler) []closeableRunner {
	topics := []string{events.TopicInventory, events.TopicPayments, events.TopicDelivery}
	runners := make([]closeableRunner, 0, len(topics))

	for _, topic := range topics {
		consumer := sharedkafka.NewConsumer(sharedkafka.ConsumerConfig{
			Brokers: brokers,
			Topic:   topic,
			GroupID: config.String("KAFKA_GROUP_ID", "order-service"),
		})
		runners = append(runners, closeableRunner{
			ConsumerRunner: orderkafka.NewConsumerRunner(topic, consumer, handler),
			consumer:       consumer,
		})
	}

	return runners
}

func closeConsumers(runners []closeableRunner) {
	for _, runner := range runners {
		if err := runner.consumer.Close(); err != nil {
			log.Printf("close order kafka consumer: %v", err)
		}
	}
}
