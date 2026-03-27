const DAY_OPTIONS = [
  { value: 0, label: "Dom" },
  { value: 1, label: "Lun" },
  { value: 2, label: "Mar" },
  { value: 3, label: "Mie" },
  { value: 4, label: "Jue" },
  { value: 5, label: "Vie" },
  { value: 6, label: "Sab" },
];

const state = {
  config: null,
  status: null,
  history: [],
};

const configForm = document.getElementById("config-form");
const activeDaysContainer = document.getElementById("active-days");
const timeWindowsContainer = document.getElementById("time-windows");
const saveFeedback = document.getElementById("save-feedback");
const runNowButton = document.getElementById("run-now");
const addWindowButton = document.getElementById("add-window");
const windowTemplate = document.getElementById("window-template");

document.addEventListener("DOMContentLoaded", async () => {
  renderDayCheckboxes(activeDaysContainer, []);
  bindEvents();
  await refreshAll();
  window.setInterval(refreshStatus, 15000);
  window.setInterval(refreshHistory, 30000);
});

function bindEvents() {
  configForm.addEventListener("submit", saveConfig);
  runNowButton.addEventListener("click", runValidationNow);
  addWindowButton.addEventListener("click", () => appendTimeWindow());
}

async function refreshAll() {
  await Promise.all([refreshConfig(), refreshStatus(), refreshHistory()]);
}

async function refreshConfig() {
  const response = await fetch("/api/config");
  state.config = await response.json();
  fillConfigForm(state.config);
}

async function refreshStatus() {
  const response = await fetch("/api/validate/status");
  state.status = await response.json();
  renderStatus();
}

async function refreshHistory() {
  const response = await fetch("/api/validate/history");
  state.history = await response.json();
  renderHistory();
}

function fillConfigForm(config) {
  document.getElementById("enabled").checked = Boolean(config.enabled);
  document.getElementById("timezone").value = config.timezone || "";
  document.getElementById("run-interval").value = config.run_interval_minutes || 20;
  document.getElementById("lookahead").value = config.lookahead_minutes || 120;
  renderDayCheckboxes(activeDaysContainer, config.active_days || []);

  timeWindowsContainer.innerHTML = "";
  (config.time_windows || []).forEach((windowConfig) => appendTimeWindow(windowConfig));
}

function renderDayCheckboxes(container, selectedDays, namePrefix = "active-day") {
  container.innerHTML = "";
  DAY_OPTIONS.forEach((day) => {
    const wrapper = document.createElement("label");
    wrapper.className = "day-pill";
    wrapper.innerHTML = `
      <input type="checkbox" name="${namePrefix}" value="${day.value}" ${selectedDays.includes(day.value) ? "checked" : ""} />
      <span>${day.label}</span>
    `;
    container.appendChild(wrapper);
  });
}

function appendTimeWindow(windowConfig = null) {
  const fragment = windowTemplate.content.cloneNode(true);
  const card = fragment.querySelector(".window-card");
  const id = windowConfig?.id || crypto.randomUUID();
  card.dataset.id = id;

  const labelInput = card.querySelector('[data-field="label"]');
  const startInput = card.querySelector('[data-field="start"]');
  const endInput = card.querySelector('[data-field="end"]');
  const daysContainer = card.querySelector('[data-field="days"]');
  const removeButton = card.querySelector('[data-action="remove-window"]');

  labelInput.value = windowConfig?.label || "Horario operativo";
  startInput.value = windowConfig?.start || "00:00";
  endInput.value = windowConfig?.end || "23:59";
  renderDayCheckboxes(daysContainer, windowConfig?.days || [], `window-day-${id}`);

  removeButton.addEventListener("click", () => {
    card.remove();
  });

  timeWindowsContainer.appendChild(fragment);
}

async function saveConfig(event) {
  event.preventDefault();
  saveFeedback.textContent = "Guardando...";

  const payload = {
    enabled: document.getElementById("enabled").checked,
    timezone: document.getElementById("timezone").value.trim(),
    run_interval_minutes: Number(document.getElementById("run-interval").value),
    lookahead_minutes: Number(document.getElementById("lookahead").value),
    active_days: getCheckedValues(activeDaysContainer),
    time_windows: collectTimeWindows(),
  };

  const response = await fetch("/api/config", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  const body = await response.json();

  if (!response.ok) {
    saveFeedback.textContent = body.error || "No se pudo guardar.";
    return;
  }

  state.config = body;
  fillConfigForm(body);
  saveFeedback.textContent = "Configuracion guardada.";
  await refreshStatus();
}

function collectTimeWindows() {
  return Array.from(timeWindowsContainer.querySelectorAll(".window-card")).map((card) => ({
    id: card.dataset.id,
    label: card.querySelector('[data-field="label"]').value.trim(),
    start: card.querySelector('[data-field="start"]').value,
    end: card.querySelector('[data-field="end"]').value,
    days: getCheckedValues(card.querySelector('[data-field="days"]')),
  }));
}

function getCheckedValues(container) {
  return Array.from(container.querySelectorAll('input[type="checkbox"]:checked')).map((input) =>
    Number(input.value),
  );
}

async function runValidationNow() {
  runNowButton.disabled = true;
  runNowButton.textContent = "Ejecutando...";

  try {
    const response = await fetch("/api/validate/run", { method: "POST" });
    const body = await response.json();
    if (!response.ok && body.error) {
      throw new Error(body.error);
    }
    state.status = { ...(state.status || {}), last_run: body };
    await refreshStatus();
    await refreshHistory();
  } catch (error) {
    alert(error.message || "No se pudo ejecutar la validacion.");
  } finally {
    runNowButton.disabled = false;
    runNowButton.textContent = "Ejecutar ahora";
  }
}

function renderStatus() {
  const status = state.status;
  if (!status) return;

  const lastRun = status.last_run;
  document.getElementById("server-time").textContent = formatDateTime(status.server_time);
  document.getElementById("next-run").textContent = status.next_run_at ? formatDateTime(status.next_run_at) : "Sin proxima ejecucion";
  document.getElementById("last-status").textContent = lastRun ? readableStatus(lastRun.status) : "Sin ejecuciones";
  document.getElementById("last-events-count").textContent = lastRun ? String(lastRun.events_found) : "0";
  document.getElementById("status-chip").textContent = status.running ? "Validando..." : "En espera";

  const details = document.getElementById("last-run-details");
  if (!lastRun) {
    details.textContent = "Aun no hay ejecuciones.";
  } else {
    details.innerHTML = `
      <strong>${readableStatus(lastRun.status)}</strong>
      <div>Inicio: ${formatDateTime(lastRun.started_at)}</div>
      <div>Fin: ${formatDateTime(lastRun.ended_at)}</div>
      <div>Trigger: ${lastRun.trigger}</div>
      <div>Scope: ${lastRun.scope_used || "-"}</div>
      <div>Resolucion de identidad: ${lastRun.identity_resolution_status || "-"}</div>
      <div>Error: ${lastRun.error || "-"}</div>
    `;
  }

  renderAppointments(lastRun?.events || []);
}

function renderAppointments(appointments) {
  const body = document.getElementById("appointments-body");
  body.innerHTML = "";

  if (!appointments.length) {
    body.innerHTML = `
      <tr>
        <td colspan="5" class="muted centered">Sin citas en la ultima ejecucion.</td>
      </tr>
    `;
    return;
  }

  appointments.forEach((appointment) => {
    const row = document.createElement("tr");
    row.innerHTML = `
      <td>${escapeHTML(appointment.event_name || "-")}</td>
      <td>${formatDateTime(appointment.start_time)}</td>
      <td>${escapeHTML(appointment.invitee_name || "-")}</td>
      <td>${escapeHTML(appointment.invitee_email || "-")}</td>
      <td>${escapeHTML(appointment.invitee_phone || "-")}</td>
    `;
    body.appendChild(row);
  });
}

function renderHistory() {
  const container = document.getElementById("history-list");
  container.innerHTML = "";

  if (!state.history.length) {
    container.textContent = "Sin historial.";
    return;
  }

  state.history.forEach((item) => {
    const article = document.createElement("article");
    article.className = "history-item";
    article.innerHTML = `
      <strong>${formatDateTime(item.started_at)} · ${readableStatus(item.status)}</strong>
      <div>Citas: ${item.events_found}</div>
      <div>Trigger: ${item.trigger}</div>
      <div>Identidad: ${item.identity_resolution_status || "-"}</div>
      <div>${escapeHTML(item.error || "Sin errores")}</div>
    `;
    container.appendChild(article);
  });
}

function readableStatus(status) {
  switch (status) {
    case "success":
      return "Correcto";
    case "partial":
      return "Parcial";
    case "failed":
      return "Fallido";
    case "running":
      return "En curso";
    default:
      return status || "-";
  }
}

function formatDateTime(value) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return new Intl.DateTimeFormat("es-MX", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}
