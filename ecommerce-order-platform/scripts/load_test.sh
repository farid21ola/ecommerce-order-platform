#!/usr/bin/env bash
set -euo pipefail

API_URL="${API_URL:-http://localhost:8080}"
SUCCESS_SKU1_COUNT="${SUCCESS_SKU1_COUNT:-20}"
FAIL_PAYMENT_SKU1_COUNT="${FAIL_PAYMENT_SKU1_COUNT:-10}"
SUCCESS_SKU2_COUNT="${SUCCESS_SKU2_COUNT:-10}"
FAIL_PAYMENT_SKU2_COUNT="${FAIL_PAYMENT_SKU2_COUNT:-5}"
WAIT_SECONDS="${WAIT_SECONDS:-20}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

require_cmd curl
require_cmd jq
require_cmd docker

tmp_dir="$(mktemp -d)"
orders_file="$tmp_dir/orders.tsv"
responses_file="$tmp_dir/responses.log"
touch "$orders_file" "$responses_file"

cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

check_api() {
  echo "Checking API Gateway: $API_URL/healthz"
  curl -fsS "$API_URL/healthz" >/dev/null
}

create_order() {
  local scenario_name="$1"
  local sku="$2"
  local payment_scenario="$3"
  local index="$4"

  local response http_code body order_id
  response="$({
    curl -sS -w '\n%{http_code}' -X POST "$API_URL/orders" \
      -H 'Content-Type: application/json' \
      -d "{\"customer_id\":\"load-${scenario_name}-${index}\",\"items\":[{\"sku\":\"${sku}\",\"quantity\":1,\"price\":1000}],\"delivery_address\":\"Moscow, Load Test street, 1\",\"payment_scenario\":\"${payment_scenario}\"}"
  } 2>&1)" || true

  http_code="$(printf '%s' "$response" | tail -n 1)"
  body="$(printf '%s' "$response" | sed '$d')"
  printf '%s\t%s\t%s\t%s\n' "$scenario_name" "$sku" "$payment_scenario" "$http_code" >> "$responses_file"

  if [ "$http_code" = "201" ]; then
    order_id="$(printf '%s' "$body" | jq -r '.order_id // empty')"
    if [ -n "$order_id" ]; then
      printf '%s\t%s\t%s\t%s\n' "$order_id" "$scenario_name" "$sku" "$payment_scenario" >> "$orders_file"
    fi
  else
    echo "Request failed: scenario=$scenario_name sku=$sku payment=$payment_scenario http=$http_code body=$body" >&2
  fi
}

run_scenario() {
  local scenario_name="$1"
  local sku="$2"
  local payment_scenario="$3"
  local count="$4"

  echo "Creating $count orders: $scenario_name sku=$sku payment=$payment_scenario"
  for i in $(seq 1 "$count"); do
    create_order "$scenario_name" "$sku" "$payment_scenario" "$i" &
  done
  wait
}

print_http_summary() {
  echo
  echo "HTTP summary"
  awk -F '\t' '{ count[$4]++ } END { for (code in count) printf "  %s: %d\n", code, count[code] }' "$responses_file" | sort
}

print_order_summary() {
  echo
  echo "Created orders: $(wc -l < "$orders_file" | tr -d ' ')"

  if [ ! -s "$orders_file" ]; then
    echo "No orders were created."
    return
  fi

  local ids_csv
  ids_csv="$(awk -F '\t' '{ printf "'\''%s'\'',", $1 }' "$orders_file" | sed 's/,$//')"

  echo
  echo "Order status summary"
  docker compose exec -T postgres psql -U ecommerce -d read_db -At -c \
    "SELECT order_status, count(*) FROM order_view WHERE order_id IN (${ids_csv}) GROUP BY order_status ORDER BY order_status;" \
    | sed 's/^/  /'

  echo
  echo "Payment status summary"
  docker compose exec -T postgres psql -U ecommerce -d read_db -At -c \
    "SELECT coalesce(payment_status, 'NULL'), count(*) FROM order_view WHERE order_id IN (${ids_csv}) GROUP BY payment_status ORDER BY payment_status NULLS FIRST;" \
    | sed 's/^/  /'

  echo
  echo "Scenario breakdown"
  for scenario_line in \
    "success-sku1|SKU-001|success" \
    "fail-payment-sku1|SKU-001|fail" \
    "success-sku2|SKU-002|success" \
    "fail-payment-sku2|SKU-002|fail"; do
    IFS='|' read -r scenario sku payment <<< "$scenario_line"
    local scenario_ids
    scenario_ids="$(awk -F '\t' -v s="$scenario" -v sku="$sku" -v p="$payment" '$2 == s && $3 == sku && $4 == p { printf "'\''%s'\'',", $1 }' "$orders_file" | sed 's/,$//')"
    if [ -n "$scenario_ids" ]; then
      echo "  $scenario sku=$sku payment=$payment"
      docker compose exec -T postgres psql -U ecommerce -d read_db -At -c \
        "SELECT order_status, count(*) FROM order_view WHERE order_id IN (${scenario_ids}) GROUP BY order_status ORDER BY order_status;" \
        | sed 's/^/    /'
    fi
  done

  echo
  echo "Inventory stock"
  docker compose exec -T postgres psql -U ecommerce -d inventory_db -At -c \
    "SELECT sku, available_quantity, reserved_quantity FROM stock_items ORDER BY sku;" \
    | sed 's/^/  /'
}

check_api
run_scenario "success-sku1" "SKU-001" "success" "$SUCCESS_SKU1_COUNT"
run_scenario "fail-payment-sku1" "SKU-001" "fail" "$FAIL_PAYMENT_SKU1_COUNT"
run_scenario "success-sku2" "SKU-002" "success" "$SUCCESS_SKU2_COUNT"
run_scenario "fail-payment-sku2" "SKU-002" "fail" "$FAIL_PAYMENT_SKU2_COUNT"

print_http_summary

echo
echo "Waiting ${WAIT_SECONDS}s for asynchronous Saga processing..."
sleep "$WAIT_SECONDS"

print_order_summary
