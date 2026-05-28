GO ?= go
DOCKER_COMPOSE ?= docker compose

SERVICES := api-gateway order-service inventory-service payment-service delivery-service notification-service read-model-service

.PHONY: fmt test build docker-build up down ps clean

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./...

build:
	@for service in $(SERVICES); do \
		$(GO) build -o bin/$$service ./cmd/$$service || exit 1; \
	done

docker-build:
	@for service in $(SERVICES); do \
		docker build --build-arg SERVICE=$$service -t ecommerce-$$service:local . || exit 1; \
	done

up:
	$(DOCKER_COMPOSE) up -d

down:
	$(DOCKER_COMPOSE) down

ps:
	$(DOCKER_COMPOSE) ps

clean:
	rm -rf bin
