"use strict";

// The token arrives in the query string, because a browser navigation cannot
// set a header. It is moved into memory and stripped from the URL immediately
// so it does not sit in the address bar or leak through history.
const TOKEN = new URLSearchParams(location.search).get("token") || "";
history.replaceState(null, "", location.pathname);

let STATE = null;

async function api(path, body) {
  const res = await fetch(path, {
    method: body ? "POST" : "GET",
    headers: {
      "X-Loadout-Token": TOKEN,
      ...(body ? { "Content-Type": "application/json" } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

const el = (tag, props = {}, ...kids) => {
  const n = Object.assign(document.createElement(tag), props);
  for (const k of kids.flat()) {
    if (k != null) n.append(k.nodeType ? k : document.createTextNode(k));
  }
  return n;
};

function toast(msg, ms = 2600) {
  const t = document.getElementById("toast");
  t.textContent = msg;
  t.hidden = false;
  clearTimeout(toast._t);
  toast._t = setTimeout(() => (t.hidden = true), ms);
}

// --- tabs ------------------------------------------------------------------

document.getElementById("tabs").addEventListener("click", (e) => {
  const btn = e.target.closest("button[data-tab]");
  if (!btn) return;
  for (const b of document.querySelectorAll("#tabs button"))
    b.classList.toggle("active", b === btn);
  for (const s of document.querySelectorAll(".tab"))
    s.classList.toggle("active", s.id === btn.dataset.tab);
});

// --- rendering -------------------------------------------------------------

function renderMeta() {
  document.getElementById("meta").textContent =
    `${STATE.platform} · ${STATE.config_path}` +
    (STATE.stale ? " · config newer than generated output" : "");
  document.getElementById("apply").disabled = false;
}

function renderOverview() {
  const root = document.getElementById("overview");
  root.replaceChildren();

  if (STATE.config_error) {
    root.append(
      el(
        "div",
        { className: "card" },
        el("h3", {}, "Configuration problem"),
        el("pre", {}, STATE.config_error),
      ),
    );
    return;
  }

  const active = STATE.aliases.filter((a) => a.enabled && a.applies);
  const unavailable = active.filter((a) => !a.available);
  const notReady = STATE.functions.filter((f) => !f.ready);

  const stat = (label, value, cls) =>
    el(
      "div",
      { className: "row" },
      el("span", { className: "nm" }, label),
      el("span", { className: cls || "" }, String(value)),
    );

  root.append(
    el(
      "div",
      { className: "card" },
      el("h3", {}, "Summary"),
      stat("Aliases active", active.length),
      stat(
        "Missing tools",
        unavailable.length,
        unavailable.length ? "status-warn" : "status-ok",
      ),
      stat(
        "Commands not ready",
        notReady.length,
        notReady.length ? "status-warn" : "status-ok",
      ),
      stat("Packs enabled", STATE.packs.filter((p) => p.enabled).length),
      stat("Keychain", STATE.keyring),
      stat(
        "Generated output",
        STATE.stale ? "out of date — apply to refresh" : "current",
        STATE.stale ? "status-warn" : "status-ok",
      ),
    ),
  );

  if (unavailable.length) {
    const byTool = new Map();
    for (const a of unavailable)
      for (const m of a.missing || [])
        byTool.set(m, [...(byTool.get(m) || []), a.name]);
    root.append(
      el(
        "div",
        { className: "card" },
        el("h3", {}, "Tools not installed"),
        ...[...byTool.entries()]
          .sort()
          .map(([tool, names]) =>
            el(
              "div",
              { className: "row" },
              el("span", { className: "dot warn" }),
              el("span", { className: "nm" }, tool),
              el("span", { className: "cmd" }, names.join(" ")),
            ),
          ),
      ),
    );
  }

  if (notReady.length) {
    root.append(
      el(
        "div",
        { className: "card" },
        el("h3", {}, "Commands needing setup"),
        ...notReady.map((f) =>
          el(
            "div",
            { className: "row" },
            el("span", { className: "dot warn" }),
            el("span", { className: "nm" }, f.name),
            el("span", { className: "cmd" }, (f.unmet || []).join("; ")),
          ),
        ),
      ),
    );
  }
}

function aliasDot(a) {
  if (!a.enabled) return "off";
  return a.available ? "ok" : "warn";
}

function renderAliases() {
  const list = document.getElementById("alias-list");
  const q = document.getElementById("alias-filter").value.toLowerCase();
  const onlyBad = document.getElementById("only-problems").checked;

  const rows = STATE.aliases
    .filter((a) => a.applies)
    .filter((a) => !onlyBad || (a.enabled && !a.available))
    .filter(
      (a) =>
        !q ||
        a.name.toLowerCase().includes(q) ||
        a.command.toLowerCase().includes(q) ||
        a.category.toLowerCase().includes(q) ||
        (a.pack || "").toLowerCase().includes(q),
    );

  document.getElementById("alias-count").textContent =
    `${rows.length} of ${STATE.aliases.filter((a) => a.applies).length}`;

  list.replaceChildren(
    ...rows.map((a) => {
      const toggle = el("input", { type: "checkbox", checked: a.enabled });
      toggle.addEventListener("change", async () => {
        try {
          await api("/api/toggle", {
            kind: "alias",
            name: a.name,
            enabled: toggle.checked,
          });
          a.enabled = toggle.checked;
          toast(`${a.name} ${toggle.checked ? "enabled" : "disabled"}`);
          await refresh();
        } catch (err) {
          toggle.checked = a.enabled;
          toast(`failed: ${err.message}`);
        }
      });

      return el(
        "div",
        { className: "row" },
        el("span", { className: "dot " + aliasDot(a) }),
        el("span", { className: "nm" }, a.name),
        el("span", { className: "cmd" }, a.command),
        a.pack ? el("span", { className: "tag" }, a.pack) : null,
        !a.available && a.missing?.length
          ? el("span", { className: "tag status-warn" }, "needs " + a.missing.join(" "))
          : null,
        el("label", { className: "switch" }, toggle),
      );
    }),
  );
}

function renderCommands() {
  const root = document.getElementById("commands");
  root.replaceChildren(
    ...STATE.functions.map((f) =>
      el(
        "div",
        { className: "card" },
        el(
          "h3",
          {},
          el("span", { className: "dot " + (f.ready ? "ok" : "warn") }),
          " " + f.name,
        ),
        el("div", { className: "muted" }, f.summary || ""),
        f.usage ? el("pre", {}, f.usage.trim()) : null,
        f.config?.length
          ? el("div", { className: "muted" }, "Configured by: " + f.config.join(", "))
          : null,
        f.unmet?.length
          ? el(
              "div",
              { className: "status-warn" },
              "Needs: " + f.unmet.join("; "),
            )
          : null,
        f.notes ? el("div", { className: "muted" }, f.notes.trim()) : null,
      ),
    ),
  );
}

function renderPacks() {
  const root = document.getElementById("packs");
  root.replaceChildren(
    el(
      "div",
      { className: "card" },
      el("h3", {}, "Optional bundles"),
      el(
        "div",
        { className: "muted" },
        "Packs are off by default. Enabling one adds its aliases to your shell.",
      ),
      ...STATE.packs.map((p) => {
        const cb = el("input", { type: "checkbox", checked: p.enabled });
        cb.addEventListener("change", async () => {
          try {
            await api("/api/toggle", {
              kind: "pack",
              name: p.name,
              enabled: cb.checked,
            });
            toast(`${p.name} ${cb.checked ? "enabled" : "disabled"}`);
            await refresh();
          } catch (err) {
            cb.checked = p.enabled;
            toast(`failed: ${err.message}`);
          }
        });
        return el(
          "div",
          { className: "row" },
          el("span", { className: "dot " + (p.enabled ? "ok" : "off") }),
          el("span", { className: "nm" }, p.name),
          el("span", { className: "cmd" }, `${p.count} aliases`),
          el("label", { className: "switch" }, cb),
        );
      }),
    ),
  );
}

function renderCredentials() {
  const root = document.getElementById("credentials");
  root.replaceChildren();

  if (!STATE.credentials.length) {
    root.append(
      el(
        "div",
        { className: "card" },
        el("h3", {}, "No credentials declared"),
        el(
          "div",
          { className: "muted" },
          "Add them under 'credentials:' in your config.",
        ),
      ),
    );
    return;
  }

  root.append(
    el(
      "div",
      { className: "card" },
      el("h3", {}, "How this works"),
      el(
        "div",
        { className: "muted" },
        `Values live in ${STATE.keyring} and are never sent to this page — only whether a value exists. ` +
          `After switching, run: eval "$(loadout env)"`,
      ),
    ),
  );

  for (const c of STATE.credentials) {
    root.append(
      el(
        "div",
        { className: "card" },
        el("h3", {}, c.name),
        ...c.entries.map((e) => {
          const isActive = e.name === c.active;
          const use = el("button", { disabled: isActive }, isActive ? "active" : "use");
          use.addEventListener("click", async () => {
            try {
              await api("/api/cred/use", { name: c.name, entry: e.name });
              toast(`${c.name} → ${e.name}`);
              await refresh();
            } catch (err) {
              toast(`failed: ${err.message}`);
            }
          });

          const setBtn = el("button", {}, e.has_value ? "replace value" : "set value");
          setBtn.addEventListener("click", () => promptForValue(c.name, e.name));

          return el(
            "div",
            { className: "row" },
            el("span", { className: "dot " + (e.has_value ? "ok" : "warn") }),
            el("span", { className: "nm" }, e.name),
            el(
              "span",
              { className: "cmd" },
              e.has_value ? "value stored" : "no value stored",
            ),
            use,
            setBtn,
          );
        }),
      ),
    );
  }
}

function promptForValue(name, entry) {
  const input = el("input", {
    type: "password",
    placeholder: "value",
    autocomplete: "off",
  });
  const save = el("button", { className: "primary" }, "Store in keychain");
  save.addEventListener("click", async () => {
    if (!input.value) return toast("value cannot be empty");
    try {
      await api("/api/cred/set", { name, entry, value: input.value });
      input.value = "";
      closeModal();
      toast(`stored ${name}/${entry}`);
      await refresh();
    } catch (err) {
      toast(`failed: ${err.message}`);
    }
  });
  openModal(
    `Set ${name} / ${entry}`,
    el(
      "div",
      {},
      el(
        "p",
        { className: "muted" },
        `Stored in ${STATE.keyring}. It is not written to any file loadout controls.`,
      ),
      input,
    ),
    [save],
  );
  input.focus();
}

// --- diff & apply ----------------------------------------------------------

// A minimal line diff. Good enough to show what changes in a generated file,
// where edits are almost always whole added or removed lines.
function diffLines(a, b) {
  const A = a ? a.split("\n") : [];
  const B = b ? b.split("\n") : [];
  const setA = new Set(A);
  const setB = new Set(B);
  const out = [];
  for (const line of B) if (!setA.has(line)) out.push(["+", line]);
  for (const line of A) if (!setB.has(line)) out.push(["-", line]);
  return out;
}

async function showDiff() {
  let d;
  try {
    d = await api("/api/diff");
  } catch (err) {
    return toast(`failed: ${err.message}`);
  }

  const changes = diffLines(d.current, d.next);
  const body = el("div", {});

  if (!d.changed) {
    body.append(
      el(
        "p",
        { className: "muted" },
        "No changes. The generated file already matches your configuration.",
      ),
    );
  } else {
    body.append(
      el(
        "p",
        { className: "muted" },
        d.no_output
          ? `Nothing generated yet. This will create ${d.path}.`
          : `${changes.length} line(s) change in ${d.path}.`,
      ),
    );
    const pre = el("pre", {});
    for (const [sign, line] of changes.slice(0, 400)) {
      pre.append(
        el(
          "div",
          { className: sign === "+" ? "diff-add" : "diff-del" },
          `${sign} ${line}`,
        ),
      );
    }
    if (changes.length > 400) {
      pre.append(el("div", { className: "muted" }, `… ${changes.length - 400} more`));
    }
    body.append(pre);
  }

  const confirm = el("button", { className: "primary" }, "Write the file");
  confirm.disabled = !d.changed;
  confirm.addEventListener("click", async () => {
    try {
      const r = await api("/api/apply", {});
      closeModal();
      toast(`wrote ${r.bytes} bytes — reload your shell to pick it up`, 5000);
      await refresh();
    } catch (err) {
      toast(`failed: ${err.message}`);
    }
  });

  openModal("Review changes", body, [confirm]);
}

// --- modal -----------------------------------------------------------------

function openModal(title, body, actions = []) {
  document.getElementById("modal-title").textContent = title;
  document.getElementById("modal-body").replaceChildren(body);
  const cancel = el("button", {}, "Cancel");
  cancel.addEventListener("click", closeModal);
  document.getElementById("modal-foot").replaceChildren(cancel, ...actions);
  document.getElementById("modal").hidden = false;
}

function closeModal() {
  document.getElementById("modal").hidden = true;
}

document.getElementById("modal-close").addEventListener("click", closeModal);
document.getElementById("modal").addEventListener("click", (e) => {
  if (e.target.id === "modal") closeModal();
});
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape") closeModal();
});

// --- boot ------------------------------------------------------------------

document.getElementById("apply").addEventListener("click", showDiff);
document.getElementById("alias-filter").addEventListener("input", renderAliases);
document.getElementById("only-problems").addEventListener("change", renderAliases);

async function refresh() {
  STATE = await api("/api/state");
  renderMeta();
  renderOverview();
  if (!STATE.config_error) {
    renderAliases();
    renderCommands();
    renderPacks();
    renderCredentials();
  }
}

refresh().catch((err) => {
  document.getElementById("overview").append(
    el("div", { className: "card" }, el("h3", {}, "Could not load"), el("pre", {}, err.message)),
  );
});
