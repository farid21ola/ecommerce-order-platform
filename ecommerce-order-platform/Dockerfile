FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

ARG SERVICE=api-gateway
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/service ./cmd/${SERVICE}

FROM alpine:3.22

RUN addgroup -S app && adduser -S app -G app
USER app

COPY --from=builder /out/service /service

ENTRYPOINT ["/service"]
