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
    for (const r of resolvers) chipFor(r);
    selectDefaults();
  }

  // Returns the chip container for a group, creating the group on first use.
  function groupFor(name) {
    const existing = Array.from(resolverList.querySelectorAll(".resolver-group"))
      .find((g) => g.dataset.group === name);
    if (existing) return existing.querySelector(".chips");
    const toggle = el("button", { type: "button", class: "group-toggle", text: "All" });
    const group = el("div", { class: "resolver-group" }, [
      el("div", { class: "resolver-group-head" }, [
        el("span", { class: "resolver-group-label", text: name }),
        toggle,
      ]),
      el("div", { class: "chips" }),
    ]);
    group.dataset.group = name;
    toggle.addEventListener("click", () => {
      const boxes = Array.from(group.querySelectorAll("input[type=checkbox]"));
      const allOn = boxes.every((c) => c.checked);
      boxes.forEach((c) => (c.checked = !allOn));
      updateResolverCount();
    });
    resolverList.appendChild(group);
    return group.querySelector(".chips");
  }

  // Short, human-friendly address: the host for DoH URLs, as-is otherwise.
  function shortAddress(addr) {
    try {
      if (/^https?:\/\//.test(addr)) return new URL(addr).host;
    } catch { /* fall through */ }
    return addr;
  }

  const protoLabels = { https: "DoH", tls: "DoT", tcp: "TCP" };

  // Each resolver is a toggleable chip backed by a real checkbox, so keyboard
  // and screen-reader behaviour comes for free.
  function chipFor(r, removable) {
    const id = "res-" + r.id;
    const cb = el("input", { type: "checkbox", id, value: r.id, class: "sr-only" });
    const proto = protoLabels[r.protocol];
    const chip = el("label", { class: "chip", for: id, title: r.address }, [
      cb,
      el("span", { class: "chip-check", "aria-hidden": "true" }),
      el("span", { class: "chip-body" }, [
        el("span", { class: "chip-name" }, [
          r.name,
          proto ? el("span", { class: "proto-tag", text: proto }) : null,
        ]),
        r.name !== r.address ? el("span", { class: "chip-addr", text: shortAddress(r.address) }) : null,
      ]),
    ]);
    const chips = groupFor(r.group || "Other");
    if (!removable) {
      chips.appendChild(chip);
      return cb;
    }
    const remove = el("button", {
      type: "button", class: "chip-remove", title: "Remove " + r.address,
      "aria-label": "Remove " + r.address, text: "×",
    });
    const wrap = el("span", { class: "chip-wrap" }, [chip, remove]);
    remove.addEventListener("click", () => {
      const i = customResolvers.findIndex((c) => c.id === r.id);
      if (i >= 0) customResolvers.splice(i, 1);
      const group = wrap.closest(".resolver-group");
      wrap.remove();
      if (!group.querySelector(".chip")) group.remove();
      updateResolverCount();
    });
    chips.appendChild(wrap);
    return cb;
  }

  function allBoxes() {
    return resolverList.querySelectorAll("input[type=checkbox]");
  }

  function checkedResolvers() {
    return Array.from(allBoxes()).filter((c) => c.checked).map((c) => c.value);
  }

  function setChecked(ids) {
    const set = new Set(ids);
    allBoxes().forEach((c) => { c.checked = set.has(c.value); });
    updateResolverCount();
  }

  function updateResolverCount() {
    const n = checkedResolvers().length;
    $("#resolver-count").textContent = n + " selected";
    $("#resolver-count").classList.toggle("none", n === 0);
    resolverList.querySelectorAll(".resolver-group").forEach((g) => {
      const boxes = Array.from(g.querySelectorAll("input[type=checkbox]"));
      const action = boxes.length && boxes.every((c) => c.checked) ? "None" : "All";
      const toggle = g.querySelector(".group-toggle");
      toggle.textContent = action;
      toggle.setAttribute("aria-label",
        (action === "All" ? "Select all " : "Clear all ") + g.dataset.group + " resolvers");
    });
  }

  function selectDefaults() { setChecked(defaults); }

  // Resolver quick-action buttons.
  resolverList.addEventListener("change", updateResolverCount);
  $("#select-all").addEventListener("click", () => setChecked(Array.from(allBoxes()).map((c) => c.value)));
  $("#select-none").addEventListener("click", () => setChecked([]));
  $("#select-defaults").addEventListener("click", selectDefaults);

  $("#add-custom").addEventListener("click", addCustom);
  customInput.addEventListener("keydown", (e) => {
    if (e.key === "Enter") { e.preventDefault(); addCustom(); }
  });

  function addCustom() {
    const addr = customInput.value.trim();
    if (!addr) return;
    customInput.value = "";
    if (customResolvers.some((c) => c.id === addr)) return;
    // Ad-hoc resolvers are plain DNS servers only; DoH/DoT endpoints must be
    // configured server-side (the API rejects arbitrary URLs to prevent SSRF).
    const r = { id: addr, name: addr, address: addr, group: "Custom" };
    customResolvers.push(r);
    chipFor(r, true).checked = true;
    updateResolverCount();
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

  // Accessible tab strip. items: [{label, badge?, badgeClass?, panel}].
  // With a single item and hideSingle set, just the panel is returned.
  let tabSeq = 0;
  function makeTabs(items, { className = "", hideSingle = false } = {}) {
    if (hideSingle && items.length === 1) return items[0].panel;
    const box = el("div", { class: "tabs " + className });
    const list = el("div", { class: "tablist", role: "tablist" });
    box.appendChild(list);
    const tabs = [];
    const panels = [];
    items.forEach((item, i) => {
      const n = ++tabSeq;
      const tab = el("button", {
        type: "button", role: "tab", class: "tab", id: "tab-" + n,
        "aria-controls": "panel-" + n,
      }, [
        el("span", { class: "tab-label", text: item.label }),
        item.badge != null ? el("span", { class: "tab-badge " + (item.badgeClass || ""), text: String(item.badge) }) : null,
      ]);
      if (item.title) tab.title = item.title;
      const panel = el("div", { role: "tabpanel", class: "tabpanel", id: "panel-" + n, "aria-labelledby": "tab-" + n });
      panel.appendChild(item.panel);
      tab.addEventListener("click", () => select(i));
      tab.addEventListener("keydown", (e) => {
        const keys = { ArrowRight: i + 1, ArrowLeft: i - 1, Home: 0, End: items.length - 1 };
        if (!(e.key in keys)) return;
        e.preventDefault();
        const next = (keys[e.key] + items.length) % items.length;
        select(next);
        tabs[next].focus();
      });
      tabs.push(tab);
      panels.push(panel);
      list.appendChild(tab);
      box.appendChild(panel);
    });
    function select(idx) {
      tabs.forEach((t, j) => {
        t.setAttribute("aria-selected", String(j === idx));
        t.tabIndex = j === idx ? 0 : -1;
        panels[j].hidden = j !== idx;
      });
    }
    select(0);
    return box;
  }

  // Groups items by key, preserving first-seen order.
  function groupBy(items, keyFn) {
    const map = new Map();
    for (const it of items) {
      const k = keyFn(it);
      if (!map.has(k)) map.set(k, []);
      map.get(k).push(it);
    }
    return map;
  }

  // Render results as one tab per hostname, each with a side-by-side
  // comparison across resolvers plus a detail tab per resolver.
  function render(data) {
    resultsEl.innerHTML = "";
    if (!data.queries.length) {
      resultsEl.appendChild(el("div", { class: "card empty-note", text: "No results." }));
      return;
    }
    const byHost = groupBy(data.queries, (q) => q.hostname);
    const items = [];
    for (const [host, queries] of byHost) {
      const view = renderHost(queries);
      const problems = queries.filter((q) => q.error || (q.status && q.status !== "NOERROR")).length;
      items.push({
        label: host,
        badge: problems ? "!" : (view.differs ? "≠" : null),
        badgeClass: problems ? "bad" : "warn",
        title: problems ? `${problems} error response(s) (e.g. NXDOMAIN, SERVFAIL, timeout)`
          : (view.differs ? "Resolvers disagree on some answers" : ""),
        panel: view.node,
      });
    }
    resultsEl.appendChild(makeTabs(items, { className: "host-tabs card" }));
  }

  function renderHost(queries) {
    const byResolver = groupBy(queries, (q) => q.resolver.id);
    const items = [];
    let differs = false;
    const hasTrace = queries.some((q) => q.trace && q.trace.length);
    if (byResolver.size > 1 && !hasTrace) {
      const cmp = compareTable(queries, byResolver);
      differs = cmp.differs;
      items.push({ label: "Compare", panel: cmp.node });
    }
    for (const [, qs] of byResolver) {
      const r = qs[0].resolver;
      const bad = qs.filter((q) => q.error || (q.status && q.status !== "NOERROR")).length;
      const box = el("div", { class: "resolver-detail" });
      // The tab already names the resolver unless it is the only one.
      for (const q of qs) box.appendChild(renderQuery(q, byResolver.size === 1));
      items.push({
        label: r.name || r.id,
        badge: bad || null,
        badgeClass: "bad",
        title: r.address,
        panel: box,
      });
    }
    return { node: makeTabs(items, { className: "resolver-tabs", hideSingle: true }), differs };
  }

  // A record-type x resolver matrix, flagging rows where resolvers disagree.
  function compareTable(queries, byResolver) {
    const types = Array.from(groupBy(queries, (q) => q.type).keys());
    const resolvers = Array.from(byResolver.values()).map((qs) => qs[0].resolver);
    const lookup = new Map(queries.map((q) => [q.type + "\u0000" + q.resolver.id, q]));
    const avg = (qs) => Math.round(qs.reduce((s, q) => s + (q.query_time_ms || 0), 0) / qs.length);

    const table = el("table", { class: "compare" });
    table.appendChild(el("tr", {}, [
      el("th", { text: "Type" }),
      ...resolvers.map((r) => el("th", { title: r.address }, [
        el("span", { class: "col-name", text: r.name || r.id }),
        el("span", { class: "col-meta", text: `${shortAddress(r.address)} · ${avg(byResolver.get(r.id))} ms` }),
      ])),
    ]));

    let differs = false;
    for (const t of types) {
      const sigs = new Set();
      const cells = resolvers.map((r) => {
        const q = lookup.get(t + "\u0000" + r.id);
        if (!q) { sigs.add("none"); return el("td", { class: "muted", text: "—" }); }
        if (q.error) { sigs.add("error"); return el("td", { class: "bad", title: q.error, text: "Error" }); }
        const answers = (q.answers || []).map((a) => a.data).sort();
        sigs.add(q.status + "|" + answers.join("\n"));
        if (!answers.length) return el("td", { class: "muted", text: q.status && q.status !== "NOERROR" ? q.status : "No records" });
        return el("td", {}, answers.map((d) => el("div", { class: "data", text: d })));
      });
      const rowDiffers = sigs.size > 1;
      differs = differs || rowDiffers;
      table.appendChild(el("tr", { class: rowDiffers ? "differs" : "" }, [
        el("th", { scope: "row" }, [
          el("span", { class: "type-badge", text: t }),
          rowDiffers ? el("span", { class: "differs-tag", text: "differs" }) : null,
        ]),
        ...cells,
      ]));
    }
    return { node: el("div", { class: "records-wrap" }, [table]), differs };
  }

  function renderQuery(q, showResolver) {
    const card = el("div", { class: "result" });
    const statusText = q.error ? "ERROR" : (q.status || "");
    const head = el("div", { class: "result-head" }, [
      el("span", { class: "type-badge", text: q.type }),
      showResolver ? el("span", { class: "resolver-name", text: q.resolver.name || q.resolver.id }) : null,
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
