CREATE TABLE order_view (
    order_id UUID PRIMARY KEY,
    customer_id TEXT NOT NULL,
    order_status TEXT NOT NULL,
    payment_status TEXT,
    delivery_status TEXT,
    total_amount BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE order_event_history (
    event_id UUID PRIMARY KEY,
    order_id UUID NOT NULL,
    event_type TEXT NOT NULL,
    service_name TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL
);

CREATE TABLE processed_events (
    event_id UUID PRIMARY KEY,
    event_type TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_order_event_history_order_id_occurred_at ON order_event_history(order_id, occurred_at);
