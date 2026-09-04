// Diglett web UI. This talks to the same public JSON API documented in the
// "Using the API" section, so anything done here can be done from curl too.
(function () {
  "use strict";

  const $ = (sel) => document.querySelector(sel);
  const el = (tag, attrs = {}, children = []) => {
    const node = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs)) {
      if (k === "class") node.className = v;
      else if (k === "text") node.textContent = v;
      else node.setAttribute(k, v);
    }
    for (const c of [].concat(children)) {
      if (c) node.appendChild(typeof c === "string" ? document.createTextNode(c) : c);
    }
    return node;
  };

  const form = $("#lookup-form");
  const typeSel = $("#type");
  const resolverList = $("#resolver-list");
  const statusEl = $("#status");
  const resultsEl = $("#results");
  const customInput = $("#custom-resolver");

  let defaults = [];
  const customResolvers = []; // {id, name, address}

  // Populate resolvers, types and defaults from the API.
  async function init() {
    try {
      const res = await fetch("api/resolvers");
      const data = await res.json();
      defaults = data.defaults || [];
      populateTypes(data.types || []);
      populateResolvers(data.resolvers || []);
    } catch (e) {
      statusEl.textContent = "Failed to load resolvers: " + e.message;
    }
    fetch("api/version").then(r => r.json()).then(v => {
      $("#version").textContent = "Diglett " + (v.version || "") + " · ";
    }).catch(() => {});
    applyStateFromURL();
  }

  function populateTypes(types) {
    // "AUTO" is already present as the first option.
    for (const t of types) {
      typeSel.appendChild(el("option", { value: t, text: t }));
    }
  }

  function populateResolvers(resolvers) {
    resolverList.innerHTML = "";
    const groups = {};
    const order = [];
    for (const r of resolvers) {
      const g = r.group || "Other";
      if (!groups[g]) { groups[g] = []; order.push(g); }
      groups[g].push(r);
    }
    for (const g of order) {
      resolverList.appendChild(el("div", { class: "resolver-group-label", text: g }));
      for (const r of groups[g]) resolverList.appendChild(resolverRow(r));
    }
    selectDefaults();
  }

  function resolverRow(r) {
    const id = "res-" + r.id;
    const cb = el("input", { type: "checkbox", id, value: r.id });
    cb.dataset.resolver = r.id;
    const label = el("label", { class: "resolver", for: id }, [
      cb,
      el("span", { text: r.name }),
      el("code", { text: r.address }),
    ]);
    return label;
  }

  function checkedResolvers() {
    return Array.from(resolverList.querySelectorAll("input[type=checkbox]:checked"))
      .map((c) => c.value);
  }

  function setChecked(ids) {
    const set = new Set(ids);
    resolverList.querySelectorAll("input[type=checkbox]").forEach((c) => {
      c.checked = set.has(c.value);
    });
  }

  function selectDefaults() { setChecked(defaults); }

  // Resolver quick-action buttons.
  $("#select-all").addEventListener("click", () =>
    resolverList.querySelectorAll("input[type=checkbox]").forEach((c) => (c.checked = true)));
  $("#select-none").addEventListener("click", () =>
    resolverList.querySelectorAll("input[type=checkbox]").forEach((c) => (c.checked = false)));
  $("#select-defaults").addEventListener("click", selectDefaults);

  $("#add-custom").addEventListener("click", addCustom);
  customInput.addEventListener("keydown", (e) => {
    if (e.key === "Enter") { e.preventDefault(); addCustom(); }
  });

  function addCustom() {
    const addr = customInput.value.trim();
    if (!addr) return;
    if (customResolvers.some((c) => c.id === addr)) { customInput.value = ""; return; }
    const isURL = /^https?:\/\//i.test(addr);
    const r = { id: addr, name: isURL ? "Custom (DoH)" : "Custom", address: addr, group: "Custom" };
    customResolvers.push(r);
    // Ensure a Custom group label exists, then append.
    let label = Array.from(resolverList.querySelectorAll(".resolver-group-label"))
      .find((n) => n.textContent === "Custom");
    if (!label) {
      label = el("div", { class: "resolver-group-label", text: "Custom" });
      resolverList.appendChild(label);
    }
    const row = resolverRow(r);
    row.querySelector("input").checked = true;
    resolverList.appendChild(row);
    customInput.value = "";
  }

  // Build a request object from the current form state.
  function buildRequest() {
    const hostnames = $("#hostnames").value
      .split(/[\s,]+/)
      .map((s) => s.trim())
      .filter(Boolean);
    return {
      hostnames,
      type: typeSel.value,
      resolvers: checkedResolvers(),
      reverse: $("#reverse").checked,
      trace: $("#trace").checked,
      dnssec: $("#dnssec").checked,
    };
  }

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const req = buildRequest();
    if (req.hostnames.length === 0) {
      statusEl.textContent = "Enter at least one hostname.";
      return;
    }
    if (req.resolvers.length === 0) {
      statusEl.textContent = "Select at least one resolver.";
      return;
    }
    updateURL(req);
    const btn = $("#submit");
    btn.disabled = true;
    statusEl.textContent = "Querying…";
    resultsEl.innerHTML = "";
    const started = performance.now();
    try {
      const res = await fetch("api/lookup", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(req),
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || ("HTTP " + res.status));
      render(data);
      const ms = Math.round(performance.now() - started);
      statusEl.textContent = `${data.queries.length} result(s) in ${ms} ms`;
    } catch (err) {
      statusEl.textContent = "Error: " + err.message;
    } finally {
      btn.disabled = false;
    }
  });

  // Render results grouped by hostname.
  function render(data) {
    resultsEl.innerHTML = "";
    const byHost = {};
    const order = [];
    for (const q of data.queries) {
      if (!byHost[q.hostname]) { byHost[q.hostname] = []; order.push(q.hostname); }
      byHost[q.hostname].push(q);
    }
    for (const host of order) {
      const group = el("div", { class: "result-group" });
      group.appendChild(el("h2", { text: host }));
      for (const q of byHost[host]) group.appendChild(renderQuery(q));
      resultsEl.appendChild(group);
    }
  }

  function renderQuery(q) {
    const card = el("div", { class: "result" });
    const statusText = q.error ? "ERROR" : (q.status || "");
    const head = el("div", { class: "result-head" }, [
      el("span", { class: "type-badge", text: q.type }),
      el("span", { class: "resolver-name", text: q.resolver.name || q.resolver.id }),
      statusText ? el("span", { class: "status-badge status-" + statusText, text: statusText }) : null,
      el("span", { class: "meta", text: `${q.server} · ${(q.protocol || "udp").toUpperCase()} · ${q.query_time_ms} ms` }),
    ]);
    card.appendChild(head);

    if (q.error) {
      card.appendChild(el("div", { class: "error-note", text: q.error }));
      return card;
    }
    if (q.trace && q.trace.length) {
      card.appendChild(renderTrace(q.trace));
      return card;
    }
    if (q.answers && q.answers.length) {
      card.appendChild(recordsTable(q.answers));
    } else {
      card.appendChild(el("div", { class: "empty-note", text: "No records of this type." }));
    }
    if (q.authority && q.authority.length) {
      const details = el("details", {});
      details.appendChild(el("summary", { text: `Authority section (${q.authority.length})` }));
      details.appendChild(recordsTable(q.authority));
      card.appendChild(details);
    }
    return card;
  }

  function recordsTable(records) {
    const wrap = el("div", { class: "records-wrap" });
    const table = el("table", { class: "records" });
    table.appendChild(el("tr", {}, [
      el("th", { text: "Name" }), el("th", { text: "TTL" }),
      el("th", { text: "Type" }), el("th", { text: "Data" }),
    ]));
    for (const r of records) {
      table.appendChild(el("tr", {}, [
        el("td", { text: r.name }),
        el("td", { text: String(r.ttl) }),
        el("td", { text: r.type }),
        el("td", { class: "data", text: r.data }),
      ]));
    }
    wrap.appendChild(table);
    return wrap;
  }

  function renderTrace(steps) {
    const box = el("div", { class: "trace" });
    for (const s of steps) {
      const step = el("div", { class: "trace-step" });
      step.appendChild(el("span", { class: "zone", text: (s.zone || ".") + " " }));
      step.appendChild(el("span", { class: "srv", text: `→ ${s.server} (${s.query_time_ms} ms)` }));
      if (s.records && s.records.length) {
        const ul = el("ul");
        for (const r of s.records) ul.appendChild(el("li", { text: `${r.name} ${r.type} ${r.data}` }));
        step.appendChild(ul);
      }
      box.appendChild(step);
    }
    return box;
  }

  // Shareable-link handling: encode key fields into the URL query string.
  function updateURL(req) {
    const p = new URLSearchParams();
    if (req.hostnames.length) p.set("name", req.hostnames.join(","));
    if (req.type) p.set("type", req.type);
    if (req.resolvers.length) p.set("resolvers", req.resolvers.join(","));
    if (req.reverse) p.set("reverse", "1");
    if (req.trace) p.set("trace", "1");
    if (req.dnssec) p.set("dnssec", "1");
    history.replaceState(null, "", "?" + p.toString());
  }

  function applyStateFromURL() {
    const p = new URLSearchParams(location.search);
    const names = p.get("name");
    if (names) $("#hostnames").value = names.split(",").join("\n");
    if (p.get("type")) typeSel.value = p.get("type");
    if (p.get("reverse")) $("#reverse").checked = true;
    if (p.get("trace")) $("#trace").checked = true;
    if (p.get("dnssec")) $("#dnssec").checked = true;
    const resolvers = p.get("resolvers");
    if (resolvers) {
      const ids = resolvers.split(",").filter(Boolean);
      // Register any that aren't in the known list as custom entries.
      for (const id of ids) {
        if (!resolverList.querySelector(`input[value="${CSS.escape(id)}"]`)) {
          customInput.value = id;
          addCustom();
        }
      }
      setChecked(ids);
    }
    if (names) form.requestSubmit();
  }

  $("#share").addEventListener("click", async () => {
    updateURL(buildRequest());
    try {
      await navigator.clipboard.writeText(location.href);
      statusEl.textContent = "Link copied to clipboard.";
    } catch {
      statusEl.textContent = location.href;
    }
  });

  init();
})();
