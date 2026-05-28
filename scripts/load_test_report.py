#!/usr/bin/env python3
import argparse
import concurrent.futures
import json
import subprocess
import time
import urllib.error
import urllib.request
from collections import Counter, defaultdict
from datetime import datetime
from html import escape
from pathlib import Path


def parse_args():
    parser = argparse.ArgumentParser(description="Run API load scenarios and generate an HTML report with SVG charts.")
    parser.add_argument("--api-url", default="http://localhost:8080")
    parser.add_argument("--success-sku1", type=int, default=20)
    parser.add_argument("--fail-sku1", type=int, default=10)
    parser.add_argument("--success-sku2", type=int, default=10)
    parser.add_argument("--fail-sku2", type=int, default=5)
    parser.add_argument("--concurrency", type=int, default=10)
    parser.add_argument("--wait", type=int, default=20)
    parser.add_argument("--output", default="reports/load_test_report.html")
    parser.add_argument("--json-output", default="", help="Path for JSON analytics. Defaults to the HTML output path with .json extension.")
    return parser.parse_args()


def request_json(method, url, payload=None, timeout=10):
    data = None
    headers = {}
    if payload is not None:
        data = json.dumps(payload).encode("utf-8")
        headers["Content-Type"] = "application/json"

    request = urllib.request.Request(url, data=data, headers=headers, method=method)
    started = time.perf_counter()
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            body = response.read().decode("utf-8")
            elapsed_ms = (time.perf_counter() - started) * 1000
            try:
                parsed = json.loads(body) if body else {}
            except json.JSONDecodeError:
                parsed = {"raw": body}
            return response.status, parsed, elapsed_ms, None
    except urllib.error.HTTPError as error:
        body = error.read().decode("utf-8")
        elapsed_ms = (time.perf_counter() - started) * 1000
        try:
            parsed = json.loads(body) if body else {}
        except json.JSONDecodeError:
            parsed = {"raw": body}
        return error.code, parsed, elapsed_ms, None
    except Exception as error:
        elapsed_ms = (time.perf_counter() - started) * 1000
        return 0, {}, elapsed_ms, str(error)


def create_order(api_url, scenario, sku, payment_scenario, index):
    payload = {
        "customer_id": f"load-report-{scenario}-{index}-{int(time.time() * 1000)}",
        "items": [{"sku": sku, "quantity": 1, "price": 1000}],
        "delivery_address": "Moscow, Load Report street, 1",
        "payment_scenario": payment_scenario,
    }
    started_at = time.time()
    status, body, elapsed_ms, error = request_json("POST", f"{api_url}/orders", payload)
    return {
        "scenario": scenario,
        "sku": sku,
        "payment_scenario": payment_scenario,
        "http_status": status,
        "elapsed_ms": elapsed_ms,
        "started_at": started_at,
        "error": error,
        "order_id": body.get("order_id") if isinstance(body, dict) else None,
    }


def get_order(api_url, order_id):
    status, body, _, _ = request_json("GET", f"{api_url}/orders/{order_id}")
    return body if status == 200 else {}


def get_history(api_url, order_id):
    status, body, _, _ = request_json("GET", f"{api_url}/orders/{order_id}/history")
    return body.get("events", []) if status == 200 else []


def psql(database, sql):
    command = [
        "docker", "compose", "exec", "-T", "postgres",
        "psql", "-U", "ecommerce", "-d", database, "-At", "-c", sql,
    ]
    completed = subprocess.run(command, check=True, capture_output=True, text=True)
    return completed.stdout.strip()


def check_api(api_url):
    status, _, _, error = request_json("GET", f"{api_url}/healthz")
    if status != 200:
        raise RuntimeError(f"API Gateway is not healthy: status={status}, error={error}")


def run_load(args):
    scenarios = [
        ("success-sku1", "SKU-001", "success", args.success_sku1),
        ("fail-payment-sku1", "SKU-001", "fail", args.fail_sku1),
        ("success-sku2", "SKU-002", "success", args.success_sku2),
        ("fail-payment-sku2", "SKU-002", "fail", args.fail_sku2),
    ]
    jobs = []
    for scenario, sku, payment, count in scenarios:
        for index in range(1, count + 1):
            jobs.append((scenario, sku, payment, index))

    results = []
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as executor:
        futures = [executor.submit(create_order, args.api_url, *job) for job in jobs]
        for future in concurrent.futures.as_completed(futures):
            results.append(future.result())
    return results


def collect_final_state(api_url, results):
    created = [result for result in results if result.get("order_id")]
    orders = []
    histories = {}
    for result in created:
        order = get_order(api_url, result["order_id"])
        if order:
            order.update({
                "scenario": result["scenario"],
                "sku": result["sku"],
                "payment_scenario": result["payment_scenario"],
            })
            orders.append(order)
        histories[result["order_id"]] = get_history(api_url, result["order_id"])
    return orders, histories


def inventory_rows():
    output = psql("inventory_db", "SELECT sku, available_quantity, reserved_quantity FROM stock_items ORDER BY sku;")
    rows = []
    for line in output.splitlines():
        sku, available, reserved = line.split("|")
        rows.append({"sku": sku, "available": int(available), "reserved": int(reserved)})
    return rows


def scenario_label(scenario):
    labels = {
        "success-sku1": "Товар 1, оплата успешна",
        "fail-payment-sku1": "Товар 1, ошибка оплаты",
        "success-sku2": "Товар 2, оплата успешна",
        "fail-payment-sku2": "Товар 2, ошибка оплаты",
    }
    return labels.get(scenario, scenario)


def russian_status(value):
    labels = {
        "NULL": "null",
        "UNKNOWN": "Неизвестно",
        "CREATED": "Создан",
        "COMPLETED": "Завершён",
        "CONFIRMED": "Подтверждён",
        "CANCELLED": "Отменён",
        "FAILED": "Ошибка",
        "STOCK_RESERVED": "Товар зарезервирован",
        "PAYMENT_PENDING": "Ожидает оплаты",
        "DELIVERY_PENDING": "Ожидает доставки",
        "PENDING": "В обработке",
        "RESERVED": "Зарезервирован",
        "PAID": "Оплачен",
        "DECLINED": "Отклонён",
        "REFUNDED": "Возврат",
        "CREATED_DELIVERY": "Доставка создана",
        "SHIPPED": "Отправлен",
        "DELIVERED": "Доставлен",
    }
    return labels.get(value, value)


def bar_chart(title, data, width=760, height=300):
    if not data:
        return f"<h3>{escape(title)}</h3><p>Нет данных</p>"
    max_value = max(data.values()) or 1
    padding_left = 160
    padding_right = 32
    row_height = 42
    chart_height = max(height, 70 + row_height * len(data))
    bar_max_width = width - padding_left - padding_right
    parts = [f"<h3>{escape(title)}</h3>", f"<svg viewBox='0 0 {width} {chart_height}' role='img'>"]
    parts.append(f"<text x='0' y='24' class='chart-title'>{escape(title)}</text>")
    y = 58
    colors = ["#0f766e", "#ea580c", "#2563eb", "#9333ea", "#b91c1c", "#15803d"]
    for index, (label, value) in enumerate(data.items()):
        bar_width = int((value / max_value) * bar_max_width)
        color = colors[index % len(colors)]
        parts.append(f"<text x='0' y='{y + 22}' class='axis-label'>{escape(str(label))}</text>")
        parts.append(f"<rect x='{padding_left}' y='{y}' width='{bar_width}' height='28' rx='8' fill='{color}'></rect>")
        parts.append(f"<text x='{padding_left + bar_width + 8}' y='{y + 20}' class='value-label'>{value}</text>")
        y += row_height
    parts.append("</svg>")
    return "\n".join(parts)


def latency_chart(results):
    return bar_chart("Распределение задержки HTTP", latency_buckets(results))


def latency_buckets(results):
    buckets = Counter()
    for result in results:
        elapsed = result["elapsed_ms"]
        if elapsed < 50:
            buckets["<50ms"] += 1
        elif elapsed < 100:
            buckets["50-100ms"] += 1
        elif elapsed < 250:
            buckets["100-250ms"] += 1
        elif elapsed < 500:
            buckets["250-500ms"] += 1
        else:
            buckets[">=500ms"] += 1
    return {label: buckets[label] for label in ["<50ms", "50-100ms", "100-250ms", "250-500ms", ">=500ms"]}


def parse_event_time(value):
    if not value:
        return None
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def order_duration_rows(orders, histories):
    order_by_id = {order.get("order_id"): order for order in orders}
    rows = []
    final_events = {"OrderCompleted", "OrderFailed", "OrderCancelled"}

    for order_id, events in histories.items():
        started_at = None
        finished_at = None
        final_event = None
        for event in events:
            event_type = event.get("event_type")
            occurred_at = parse_event_time(event.get("occurred_at"))
            if event_type == "OrderCreated" and occurred_at:
                started_at = occurred_at
            if event_type in final_events and occurred_at:
                finished_at = occurred_at
                final_event = event_type

        if started_at and finished_at:
            duration_ms = (finished_at - started_at).total_seconds() * 1000
            order = order_by_id.get(order_id, {})
            rows.append({
                "order_id": order_id,
                "scenario": order.get("scenario"),
                "final_status": order.get("order_status"),
                "final_event": final_event,
                "duration_ms": max(duration_ms, 0),
            })

    return rows


def total_processing_time_ms(histories):
    first_started_at = None
    last_finished_at = None
    final_events = {"OrderCompleted", "OrderFailed", "OrderCancelled"}

    for events in histories.values():
        for event in events:
            occurred_at = parse_event_time(event.get("occurred_at"))
            if not occurred_at:
                continue
            if event.get("event_type") == "OrderCreated":
                if first_started_at is None or occurred_at < first_started_at:
                    first_started_at = occurred_at
            if event.get("event_type") in final_events:
                if last_finished_at is None or occurred_at > last_finished_at:
                    last_finished_at = occurred_at

    if first_started_at is None or last_finished_at is None:
        return 0
    return max((last_finished_at - first_started_at).total_seconds() * 1000, 0)


def format_duration(ms):
    if ms >= 1000:
        return f"{ms / 1000:.2f} с"
    return f"{ms:.0f} мс"


def nice_step(max_value, target_ticks=6):
    if max_value <= 0:
        return 100

    raw_step = max_value / target_ticks
    magnitude = 1
    while magnitude * 10 <= raw_step:
        magnitude *= 10

    for multiplier in [1, 2, 5, 10]:
        step = multiplier * magnitude
        if step >= raw_step:
            return step
    return 10 * magnitude


def format_axis_duration(ms):
    if ms >= 1000:
        value = ms / 1000
        return f"{value:.0f}с" if value.is_integer() else f"{value:.1f}с"
    return f"{int(ms)}мс"


def order_duration_chart(duration_rows):
    if not duration_rows:
        return "<h3>Время выполнения заказа</h3><p>Нет данных</p>"

    chart_data = order_duration_chart_data(duration_rows)
    chart_points = chart_data["points"]
    chart_points_json = json.dumps(chart_points, ensure_ascii=False)
    x_max = chart_data["x_max_ms"]
    x_step = chart_data["x_step_ms"]

    return f"""
<h3>Время выполнения заказа</h3>
<div class="chart-canvas-wrap">
  <canvas id="orderDurationChart"></canvas>
</div>
<script>
(() => {{
  const ctx = document.getElementById('orderDurationChart');
  const points = {chart_points_json};
  new Chart(ctx, {{
    type: 'line',
    data: {{
      datasets: [{{
        label: 'Количество заказов',
        data: points,
        borderColor: '#2563eb',
        backgroundColor: '#2563eb',
        pointRadius: 5,
        pointHoverRadius: 7,
        borderWidth: 3,
        tension: 0,
        fill: false,
      }}],
    }},
    options: {{
      responsive: true,
      maintainAspectRatio: false,
      parsing: false,
      plugins: {{
        legend: {{ display: false }},
        tooltip: {{
          callbacks: {{
            title: (items) => `Время: ${{formatDuration(items[0].parsed.x)}}`,
            label: (item) => `Заказов: ${{item.parsed.y}}`,
          }},
        }},
      }},
      scales: {{
        x: {{
          type: 'linear',
          min: 0,
          max: {x_max},
          title: {{ display: true, text: 'Время выполнения' }},
          ticks: {{
            stepSize: {x_step},
            callback: (value) => formatDuration(value),
          }},
        }},
        y: {{
          beginAtZero: true,
          title: {{ display: true, text: 'Количество заказов' }},
          ticks: {{ precision: 0 }},
        }},
      }},
    }},
  }});

  function formatDuration(ms) {{
    if (ms >= 1000) {{
      const seconds = ms / 1000;
      return Number.isInteger(seconds) ? `${{seconds}}с` : `${{seconds.toFixed(1)}}с`;
    }}
    return `${{Math.round(ms)}}мс`;
  }}
}})();
</script>"""


def order_duration_chart_data(duration_rows):
    if not duration_rows:
        return {
            "bucket_ms": 0,
            "x_max_ms": 0,
            "x_step_ms": 0,
            "points": [{"x": 0, "y": 0}],
        }

    max_duration = max(row["duration_ms"] for row in duration_rows)
    bucket_ms = max(100, nice_step(max_duration, target_ticks=20))
    buckets = Counter()
    for row in duration_rows:
        duration_bucket = round(row["duration_ms"] / bucket_ms) * bucket_ms
        buckets[int(duration_bucket)] += 1

    x_max = max(max(buckets.keys()), bucket_ms)
    return {
        "bucket_ms": bucket_ms,
        "x_max_ms": x_max,
        "x_step_ms": nice_step(x_max),
        "points": [{"x": 0, "y": 0}] + [
            {"x": duration_ms, "y": count}
            for duration_ms, count in sorted(buckets.items())
        ],
    }


def render_table(headers, rows):
    head = "".join(f"<th>{escape(header)}</th>" for header in headers)
    body = []
    for row in rows:
        body.append("<tr>" + "".join(f"<td>{escape(str(cell))}</td>" for cell in row) + "</tr>")
    return f"<table><thead><tr>{head}</tr></thead><tbody>{''.join(body)}</tbody></table>"


def build_analytics(args, results, orders, histories, inventory):
    http_counts = Counter(str(result["http_status"]) for result in results)
    order_counts = Counter(order.get("order_status", "UNKNOWN") for order in orders)
    payment_counts = Counter(order.get("payment_status") or "NULL" for order in orders)
    request_scenario_counts = Counter(result["scenario"] for result in results)
    event_counts = Counter()
    scenario_status_counts = defaultdict(Counter)

    for order in orders:
        scenario_status_counts[order["scenario"]][order.get("order_status", "UNKNOWN")] += 1
    for events in histories.values():
        for event in events:
            event_counts[event.get("event_type", "UNKNOWN")] += 1

    duration_rows = order_duration_rows(orders, histories)
    created_count = sum(1 for result in results if result.get("order_id"))
    avg_latency = sum(result["elapsed_ms"] for result in results) / max(len(results), 1)
    avg_order_duration = sum(row["duration_ms"] for row in duration_rows) / max(len(duration_rows), 1)
    total_processing_time = total_processing_time_ms(histories)
    scenarios = [
        ("success-sku1", "SKU-001", "success", args.success_sku1),
        ("fail-payment-sku1", "SKU-001", "fail", args.fail_sku1),
        ("success-sku2", "SKU-002", "success", args.success_sku2),
        ("fail-payment-sku2", "SKU-002", "fail", args.fail_sku2),
    ]

    return {
        "generated_at": datetime.now().isoformat(timespec="seconds"),
        "api_url": args.api_url,
        "concurrency": args.concurrency,
        "wait_seconds": args.wait,
        "summary": {
            "total_requests": len(results),
            "created_orders": created_count,
            "avg_http_latency_ms": avg_latency,
            "avg_order_duration_ms": avg_order_duration,
            "total_processing_time_ms": total_processing_time,
            "total_processing_time_label": format_duration(total_processing_time),
        },
        "counts": {
            "http_statuses": dict(sorted(http_counts.items())),
            "final_order_statuses": dict(sorted(order_counts.items())),
            "payment_statuses": dict(sorted(payment_counts.items())),
            "events": dict(event_counts.most_common()),
            "requests_by_scenario": dict(sorted(request_scenario_counts.items())),
        },
        "latency_buckets": latency_buckets(results),
        "request_plan_actual": [
            {
                "scenario": scenario,
                "scenario_label": scenario_label(scenario),
                "sku": sku,
                "payment_scenario": payment,
                "planned_requests": planned,
                "actual_requests": request_scenario_counts[scenario],
            }
            for scenario, sku, payment, planned in scenarios
        ],
        "scenario_breakdown": [
            {
                "scenario": scenario,
                "scenario_label": scenario_label(scenario),
                "final_status": status,
                "final_status_label": russian_status(status),
                "count": count,
            }
            for scenario, statuses in sorted(scenario_status_counts.items())
            for status, count in sorted(statuses.items())
        ],
        "order_duration_chart": order_duration_chart_data(duration_rows),
        "order_durations": duration_rows,
        "inventory": inventory,
        "orders_sample": orders[:50],
    }


def build_report(args, results, orders, histories, inventory):
    http_counts = Counter(str(result["http_status"]) for result in results)
    order_counts = Counter(russian_status(order.get("order_status", "UNKNOWN")) for order in orders)
    payment_counts = Counter(russian_status(order.get("payment_status") or "NULL") for order in orders)
    scenario_counts = defaultdict(Counter)
    request_scenario_counts = Counter(result["scenario"] for result in results)
    event_counts = Counter()

    for order in orders:
        scenario_counts[order["scenario"]][order.get("order_status", "UNKNOWN")] += 1
    for events in histories.values():
        for event in events:
            event_counts[event.get("event_type", "UNKNOWN")] += 1

    scenario_rows = []
    for scenario, statuses in scenario_counts.items():
        for status, count in statuses.items():
            scenario_rows.append([scenario_label(scenario), russian_status(status), count])

    request_scenario_rows = [
        [scenario_label("success-sku1"), "SKU-001", "Успешная", args.success_sku1, request_scenario_counts["success-sku1"]],
        [scenario_label("fail-payment-sku1"), "SKU-001", "Ошибка", args.fail_sku1, request_scenario_counts["fail-payment-sku1"]],
        [scenario_label("success-sku2"), "SKU-002", "Успешная", args.success_sku2, request_scenario_counts["success-sku2"]],
        [scenario_label("fail-payment-sku2"), "SKU-002", "Ошибка", args.fail_sku2, request_scenario_counts["fail-payment-sku2"]],
    ]

    inventory_rows_html = [[row["sku"], row["available"], row["reserved"]] for row in inventory]
    order_rows = [[
        order.get("order_id"),
        scenario_label(order.get("scenario")),
        russian_status(order.get("order_status")),
        russian_status(order.get("payment_status") or "NULL"),
        russian_status(order.get("delivery_status") or "NULL"),
        order.get("total_amount"),
    ] for order in orders[:50]]

    duration_rows = order_duration_rows(orders, histories)
    duration_table_rows = [[
        row["order_id"],
        scenario_label(row.get("scenario")),
        russian_status(row.get("final_status")),
        row.get("final_event"),
        f"{row['duration_ms']:.0f}",
    ] for row in duration_rows[:50]]

    created_count = sum(1 for result in results if result.get("order_id"))
    avg_latency = sum(result["elapsed_ms"] for result in results) / max(len(results), 1)
    avg_order_duration = sum(row["duration_ms"] for row in duration_rows) / max(len(duration_rows), 1)
    total_processing_time = total_processing_time_ms(histories)
    generated_at = datetime.now().strftime("%Y-%m-%d %H:%M:%S")

    return f"""<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8" />
  <title>Отчёт нагрузочного тестирования</title>
  <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.7/dist/chart.umd.min.js"></script>
  <style>
    body {{ margin: 0; font-family: Inter, Arial, sans-serif; background: #f8fafc; color: #111827; }}
    main {{ width: min(1120px, calc(100% - 32px)); margin: 0 auto; padding: 36px 0; }}
    .hero, section {{ background: white; border: 1px solid #e5e7eb; border-radius: 22px; padding: 24px; margin-bottom: 18px; box-shadow: 0 18px 50px rgba(15,23,42,.06); }}
    h1 {{ margin: 0 0 8px; font-size: 42px; letter-spacing: -.04em; }}
    h2 {{ margin: 0 0 16px; }}
    h3 {{ margin: 20px 0 10px; }}
    .metrics {{ display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; }}
    .metric {{ background: #f1f5f9; border-radius: 16px; padding: 16px; }}
    .metric span {{ display: block; color: #64748b; font-size: 12px; font-weight: 800; text-transform: uppercase; }}
    .metric strong {{ display: block; margin-top: 8px; font-size: 28px; }}
    svg {{ width: 100%; height: auto; background: #f8fafc; border-radius: 16px; padding: 12px; }}
    .chart-canvas-wrap {{ height: 380px; background: #f8fafc; border-radius: 16px; padding: 16px; }}
    .chart-title {{ font-size: 18px; font-weight: 800; fill: #111827; }}
    .axis-label {{ font-size: 13px; font-weight: 700; fill: #475569; }}
    .axis-line {{ stroke: #334155; stroke-width: 2; }}
    .value-label {{ font-size: 13px; font-weight: 800; fill: #111827; }}
    table {{ width: 100%; border-collapse: collapse; font-size: 14px; }}
    th, td {{ border-bottom: 1px solid #e5e7eb; padding: 10px; text-align: left; }}
    th {{ color: #475569; font-size: 12px; text-transform: uppercase; }}
    @media (max-width: 760px) {{ .metrics {{ grid-template-columns: 1fr; }} }}
  </style>
</head>
<body>
<main>
  <div class="hero">
    <h1>Отчёт нагрузочного тестирования</h1>
    <p>Сформирован: {escape(generated_at)}. API: {escape(args.api_url)}</p>
    <div class="metrics">
      <div class="metric"><span>Всего запросов</span><strong>{len(results)}</strong></div>
      <div class="metric"><span>Создано заказов</span><strong>{created_count}</strong></div>
      <div class="metric"><span>Параллельность</span><strong>{args.concurrency}</strong></div>
      <div class="metric"><span>Средняя задержка HTTP</span><strong>{avg_latency:.1f} мс</strong></div>
      <div class="metric"><span>Среднее время заказа</span><strong>{avg_order_duration:.0f} мс</strong></div>
      <div class="metric"><span>Все заказы обработаны за</span><strong>{format_duration(total_processing_time)}</strong></div>
    </div>
  </div>

  <section>{bar_chart('HTTP-статусы', dict(sorted(http_counts.items())))}</section>
  <section>{bar_chart('Финальные статусы заказов', dict(sorted(order_counts.items())))}</section>
  <section>{bar_chart('Статусы оплаты', dict(sorted(payment_counts.items())))}</section>
  <section>{bar_chart('События в историях заказов', dict(event_counts.most_common()))}</section>
  <section>{latency_chart(results)}</section>
  <section>{order_duration_chart(duration_rows)}</section>

  <section>
    <h2>План и факт запросов</h2>
    {render_table(['Сценарий', 'SKU', 'Сценарий оплаты', 'Запланировано', 'Фактически'], request_scenario_rows)}
  </section>

  <section>
    <h2>Разбивка по сценариям</h2>
    {render_table(['Сценарий', 'Финальный статус', 'Количество'], scenario_rows)}
  </section>

  <section>
    <h2>Остатки после теста</h2>
    {render_table(['SKU', 'Доступно', 'Зарезервировано'], inventory_rows_html)}
  </section>

  <section>
    <h2>Пример созданных заказов</h2>
    {render_table(['ID заказа', 'Сценарий', 'Статус заказа', 'Статус оплаты', 'Статус доставки', 'Сумма'], order_rows)}
  </section>

  <section>
    <h2>Время выполнения заказов</h2>
    {render_table(['ID заказа', 'Сценарий', 'Финальный статус', 'Финальное событие', 'Длительность, мс'], duration_table_rows)}
  </section>
</main>
</body>
</html>"""


def main():
    args = parse_args()
    check_api(args.api_url)
    results = run_load(args)
    print(f"Created HTTP requests: {len(results)}")
    print(f"Waiting {args.wait}s for asynchronous Saga processing...")
    time.sleep(args.wait)
    orders, histories = collect_final_state(args.api_url, results)
    inventory = inventory_rows()
    report = build_report(args, results, orders, histories, inventory)
    analytics = build_analytics(args, results, orders, histories, inventory)
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(report, encoding="utf-8")
    json_output = Path(args.json_output) if args.json_output else output.with_suffix(".json")
    json_output.parent.mkdir(parents=True, exist_ok=True)
    json_output.write_text(json.dumps(analytics, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"Report written to: {output}")
    print(f"JSON analytics written to: {json_output}")


if __name__ == "__main__":
    main()
