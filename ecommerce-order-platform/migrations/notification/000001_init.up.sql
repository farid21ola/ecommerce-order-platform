CREATE TABLE notification_tasks (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL,
    event_id UUID NOT NULL,
    status TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sending_log (
    id UUID PRIMARY KEY,
    notification_task_id UUID NOT NULL REFERENCES notification_tasks(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE processed_events (
    event_id UUID PRIMARY KEY,
    event_type TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_notification_tasks_order_id ON notification_tasks(order_id);
CREATE INDEX idx_sending_log_notification_task_id ON sending_log(notification_task_id);
