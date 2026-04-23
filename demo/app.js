const demoUserId = "11111111-1111-1111-1111-111111111111";
const productWithStock = "22222222-2222-2222-2222-222222222222";
const productWithoutStock = "33333333-3333-3333-3333-333333333333";

const payloadView = document.getElementById("payload-view");
const result = document.getElementById("result");

function showPayload(productId) {
  const payload = {
    userId: demoUserId,
    productId,
    quantity: 1,
  };
  payloadView.textContent = JSON.stringify(payload, null, 2);
}

function showResult(kind) {
  const success = kind === "success";
  const orderId = `demo-order-${Date.now()}`;
  const finalStatus = success ? "CREATED" : "FAILED";
  const reason = success ? "stock reserved" : "insufficient stock";

  result.innerHTML = `
    <p><strong>order_id:</strong> ${orderId}</p>
    <p><strong>initial_status:</strong> PENDING</p>
    <p><strong>final_status:</strong> ${finalStatus}</p>
    <p><strong>reason:</strong> ${reason}</p>
    <p class="muted">Simulación local (sin backend) — PR #14 conecta este flujo a endpoints reales.</p>
  `;
}

document.getElementById("btn-success").addEventListener("click", () => {
  showPayload(productWithStock);
  showResult("success");
});

document.getElementById("btn-fail").addEventListener("click", () => {
  showPayload(productWithoutStock);
  showResult("fail");
});

showPayload(productWithStock);
