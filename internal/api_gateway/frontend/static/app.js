const products = [...document.querySelectorAll('.product')];
const form = document.querySelector('#orderForm');
const createButton = document.querySelector('#createButton');
const formMessage = document.querySelector('#formMessage');
const statusBadge = document.querySelector('#statusBadge');
const paymentStatus = document.querySelector('#paymentStatus');
const deliveryStatus = document.querySelector('#deliveryStatus');
const totalAmount = document.querySelector('#totalAmount');
const historyList = document.querySelector('#history');
const ordersList = document.querySelector('#ordersList');
const refreshButton = document.querySelector('#refreshButton');
const refreshOrdersButton = document.querySelector('#refreshOrdersButton');
const quantityInput = document.querySelector('#quantity');
const deliveryAddressInput = document.querySelector('#deliveryAddress');
const selectedName = document.querySelector('#selectedName');
const selectedSku = document.querySelector('#selectedSku');
const selectedPrice = document.querySelector('#selectedPrice');
const summaryTotal = document.querySelector('#summaryTotal');
const notificationToast = document.querySelector('#notificationToast');
const notificationTitle = document.querySelector('#notificationTitle');
const notificationText = document.querySelector('#notificationText');

let selectedProduct = getSelectedProduct();
let orderId = localStorage.getItem('currentOrderId') || '';
let timer = null;
let toastTimer = null;
const notifiedOrders = new Set(JSON.parse(localStorage.getItem('notifiedOrders') || '[]'));

syncSelectedProduct();

if (orderId) {
  startPolling();
}
loadOrders();

products.forEach((card) => {
  card.addEventListener('click', () => {
    selectedProduct = productFromCard(card);
    products.forEach((item) => item.classList.toggle('selected', item === card));
    syncSelectedProduct();
  });
});

quantityInput.addEventListener('input', syncSelectedProduct);

form.addEventListener('submit', async (event) => {
  event.preventDefault();
  formMessage.textContent = '';
  createButton.disabled = true;

  const quantity = Math.max(1, Number(quantityInput.value || 1));
  const deliveryAddress = deliveryAddressInput.value.trim();
  if (!deliveryAddress) {
    formMessage.textContent = 'Укажите адрес доставки';
    createButton.disabled = false;
    return;
  }

  const payload = {
    customer_id: `customer-ui-${Date.now()}`,
    items: [{
      sku: selectedProduct.sku,
      quantity,
      price: selectedProduct.price,
    }],
    delivery_address: deliveryAddress,
    payment_scenario: document.querySelector('#paymentScenario').value,
  };

  try {
    const response = await fetch('/orders', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || 'Не удалось создать заказ');

    orderId = data.order_id;
    localStorage.setItem('currentOrderId', orderId);
    renderStatus({ order_status: data.status, total_amount: quantity * selectedProduct.price });
    historyList.innerHTML = '';
    startPolling();
    setTimeout(loadOrders, 1200);
  } catch (error) {
    formMessage.textContent = error.message;
  } finally {
    createButton.disabled = false;
  }
});

refreshButton.addEventListener('click', () => {
  if (orderId) refresh();
});

refreshOrdersButton.addEventListener('click', loadOrders);

function getSelectedProduct() {
  return productFromCard(document.querySelector('.product.selected') || products[0]);
}

function productFromCard(card) {
  return {
    sku: card.dataset.sku,
    name: card.dataset.name,
    price: Number(card.dataset.price),
  };
}

function syncSelectedProduct() {
  const quantity = Math.max(1, Number(quantityInput.value || 1));
  selectedName.textContent = selectedProduct.name;
  selectedSku.textContent = selectedProduct.sku;
  selectedPrice.textContent = money(selectedProduct.price);
  summaryTotal.textContent = money(quantity * selectedProduct.price);
}

function startPolling() {
  if (timer) clearInterval(timer);
  refresh();
  timer = setInterval(refresh, 2000);
}

async function refresh() {
  await Promise.all([loadOrder(), loadHistory(), loadOrders()]);
}

async function loadOrders() {
  const response = await fetch('/orders?limit=30');
  if (!response.ok) return;
  const data = await response.json();
  renderOrders(data.orders || []);
}

async function loadOrder() {
  const response = await fetch(`/orders/${orderId}`);
  if (response.status === 404) return;
  if (!response.ok) throw new Error('Не удалось получить заказ');
  renderStatus(await response.json());
}

async function loadHistory() {
  const response = await fetch(`/orders/${orderId}/history`);
  if (!response.ok) return;
  const data = await response.json();
  renderHistory(data.events || []);
}

function renderStatus(order) {
  const status = order.order_status || order.status || 'CREATED';
  statusBadge.textContent = status;
  statusBadge.className = `status ${statusClass(status)}`;
  paymentStatus.textContent = order.payment_status || '—';
  deliveryStatus.textContent = order.delivery_status || '—';
  totalAmount.textContent = order.total_amount ? money(order.total_amount) : '—';
}

function statusClass(status) {
  if (status === 'COMPLETED') return 'success';
  if (status === 'FAILED' || status === 'CANCELLED') return 'failure';
  if (status === 'WAITING') return 'waiting';
  return 'progress';
}

function renderHistory(events) {
  historyList.innerHTML = '';
  for (const event of events) {
    const item = document.createElement('li');
    const occurredAt = event.occurred_at ? new Date(event.occurred_at).toLocaleTimeString() : '';
    item.innerHTML = `
      <div>
        <div class="event-name">${escapeHTML(event.event_type)}</div>
        <div class="event-source">${escapeHTML(event.service_name || '')}</div>
      </div>
      <time>${occurredAt}</time>
    `;
    historyList.appendChild(item);
  }
  maybeShowOrderNotification(events);
}

function maybeShowOrderNotification(events) {
  if (!orderId || notifiedOrders.has(orderId)) return;
  const notification = events.find((event) => event.event_type === 'NotificationSent');
  if (!notification) return;

  notifiedOrders.add(orderId);
  localStorage.setItem('notifiedOrders', JSON.stringify([...notifiedOrders].slice(-100)));

  const triggeredBy = notification.payload?.triggered_by || '';
  showToast(notificationTitleFor(triggeredBy), `Заказ ${shortId(orderId)} обработан. Событие: ${triggeredBy || 'NotificationSent'}`);
}

function showToast(title, text) {
  if (toastTimer) clearTimeout(toastTimer);
  notificationTitle.textContent = title;
  notificationText.textContent = text;
  notificationToast.hidden = false;
  requestAnimationFrame(() => notificationToast.classList.add('show'));
  toastTimer = setTimeout(() => {
    notificationToast.classList.remove('show');
    setTimeout(() => {
      notificationToast.hidden = true;
    }, 220);
  }, 5200);
}

function notificationTitleFor(eventType) {
  if (eventType === 'OrderCompleted') return 'Заказ успешно завершён';
  if (eventType === 'OrderCancelled') return 'Заказ отменён';
  if (eventType === 'OrderFailed') return 'Заказ завершился ошибкой';
  return 'Уведомление по заказу отправлено';
}

function shortId(value) {
  return value ? value.slice(0, 8) : '';
}

function renderOrders(orders) {
  ordersList.innerHTML = '';
  if (orders.length === 0) {
    ordersList.innerHTML = '<p class="muted">Заказов пока нет.</p>';
    return;
  }

  for (const order of orders) {
    const card = document.createElement('button');
    card.type = 'button';
    card.className = `order-card ${order.order_id === orderId ? 'active' : ''}`;
    card.innerHTML = `
      <strong>${escapeHTML(order.order_status)}</strong>
      <code>${escapeHTML(order.order_id)}</code>
      <span>${money(order.total_amount)} · ${new Date(order.created_at).toLocaleString()}</span>
    `;
    card.addEventListener('click', () => {
      orderId = order.order_id;
      localStorage.setItem('currentOrderId', orderId);
      renderStatus(order);
      startPolling();
      window.location.hash = 'tracking';
    });
    ordersList.appendChild(card);
  }
}

function money(value) {
  return `${Number(value).toLocaleString('ru-RU')} ₽`;
}

function escapeHTML(value) {
  return String(value).replace(/[&<>'"]/g, (char) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    "'": '&#39;',
    '"': '&quot;',
  }[char]));
}
