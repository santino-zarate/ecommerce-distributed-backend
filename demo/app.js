const demoUserId = "11111111-1111-1111-1111-111111111111";
const productWithStock = "22222222-2222-2222-2222-222222222222";
const productWithoutStock = "33333333-3333-3333-3333-333333333333";
const defaultBackendURL = "https://ecommerce-api-u14x.onrender.com";

const backendInput = document.getElementById("backend-url");
const payloadView = document.getElementById("payload-view");
const result = document.getElementById("result");
const backendLink = document.getElementById("backend-link");
const metricsLink = document.getElementById("metrics-link");
const btnSuccess = document.getElementById("btn-success");
const btnFail = document.getElementById("btn-fail");

function readOrderId(order) {
  return order?.id || order?.ID || "";
}

function readOrderStatus(order) {
  return order?.status || order?.Status || "";
}

function normalizeBaseURL(url) {
  return (url || defaultBackendURL).trim().replace(/\/+$/, "");
}

function getBackendURL() {
  const base = normalizeBaseURL(backendInput.value);
  localStorage.setItem("demo_backend_url", base);
  backendLink.href = `${base}/health`;
  if (metricsLink) {
    metricsLink.href = `${base}/metrics`;
  }
  return base;
}

function showPayload(productId) {
  const payload = {
    userId: demoUserId,
    productId,
    quantity: 1,
  };
  payloadView.textContent = JSON.stringify(payload, null, 2);
  return payload;
}

function statusClass(status) {
  return `status-${String(status || "idle").toLowerCase()}`;
}

function statusBadge(status) {
  return `<span class="status-pill ${statusClass(status)}">${status}</span>`;
}

function resultRow(label, value) {
  return `
    <div class="result-row">
      <span class="result-label">${label}</span>
      <span class="result-value">${value}</span>
    </div>
  `;
}

function setLoading(message) {
  result.innerHTML = `
    <p>${statusBadge("PENDING")}</p>
    <p>${message}</p>
  `;
}

function setError(message) {
  result.innerHTML = `
    <p>${statusBadge("ERROR")}</p>
    <p><strong>Error:</strong> ${message}</p>
  `;
}

function setResult({ orderId, initialStatus, finalStatus, checks, tookMs }) {
  result.innerHTML = `
    <p>${statusBadge(finalStatus)}</p>
    ${resultRow("order_id", orderId)}
    ${resultRow("initial_status", statusBadge(initialStatus))}
    ${resultRow("final_status", statusBadge(finalStatus))}
    ${resultRow("polls", checks)}
    ${resultRow("took_ms", tookMs)}
  `;
}

async function pollOrderStatus(baseURL, orderId, maxChecks = 12, delayMs = 1000) {
  for (let i = 1; i <= maxChecks; i++) {
    const res = await fetch(`${baseURL}/orders/${orderId}`);
    if (!res.ok) {
      throw new Error(`GET /orders/:id devolvió ${res.status}`);
    }

    const order = await res.json();
    const status = readOrderStatus(order);
    if (!status) {
      throw new Error("GET /orders/:id devolvió payload sin status");
    }
    if (status !== "PENDING") {
      return { status, checks: i };
    }

    await new Promise((resolve) => setTimeout(resolve, delayMs));
  }

  return { status: "PENDING", checks: maxChecks };
}

async function runScenario(productId) {
  const baseURL = getBackendURL();
  const payload = showPayload(productId);
  const startedAt = Date.now();

  setLoading("Creando orden...");
  const createRes = await fetch(`${baseURL}/orders`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });

  if (!createRes.ok) {
    const body = await createRes.text();
    throw new Error(`POST /orders devolvió ${createRes.status}. ${body}`);
  }

  const created = await createRes.json();
  const orderId = readOrderId(created);
  const initialStatus = readOrderStatus(created) || "PENDING";
  if (!orderId) {
    throw new Error(`POST /orders devolvió payload sin id: ${JSON.stringify(created)}`);
  }

  setLoading(`Orden ${orderId} creada en ${initialStatus}. Esperando resolución de saga...`);
  const polled = await pollOrderStatus(baseURL, orderId);

  setResult({
    orderId,
    initialStatus,
    finalStatus: polled.status,
    checks: polled.checks,
    tookMs: Date.now() - startedAt,
  });
}

async function handleRun(productId) {
  btnSuccess.disabled = true;
  btnFail.disabled = true;
  try {
    await runScenario(productId);
  } catch (err) {
    setError(err.message || "falló la ejecución");
  } finally {
    btnSuccess.disabled = false;
    btnFail.disabled = false;
  }
}

btnSuccess.addEventListener("click", () => handleRun(productWithStock));
btnFail.addEventListener("click", () => handleRun(productWithoutStock));
backendInput.addEventListener("change", getBackendURL);
backendInput.addEventListener("input", getBackendURL);

backendInput.value = normalizeBaseURL(localStorage.getItem("demo_backend_url") || defaultBackendURL);
getBackendURL();
showPayload(productWithStock);
