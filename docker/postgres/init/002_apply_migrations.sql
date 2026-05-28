\connect order_db
\i /migrations/order/000001_init.up.sql
\i /migrations/order/000002_processed_events.up.sql

\connect inventory_db
\i /migrations/inventory/000001_init.up.sql
\i /migrations/inventory/000002_unique_reservation_order.up.sql

\connect payment_db
\i /migrations/payment/000001_init.up.sql
\i /migrations/payment/000002_unique_payment_order.up.sql

\connect delivery_db
\i /migrations/delivery/000001_init.up.sql
\i /migrations/delivery/000002_unique_delivery_order.up.sql

\connect notification_db
\i /migrations/notification/000001_init.up.sql
\i /migrations/notification/000002_unique_task_event.up.sql

\connect read_db
\i /migrations/readmodel/000001_init.up.sql
