import { escape as esc, syntax } from "./syntax.js";

const $ = (id) => document.getElementById(id);
const text = $("text");
const budget = $("budget");
const jump = $("jump");
const query = $("query");
const selected = "aria-current";
const icons = { text: "¶", rules: "≡", pages: "↗", diagram: "◇", overrides: "≡" };
const split = (value) => value.replaceAll("\r\n", "\n").split("\n");
const short = (path) => path.split("/").at(-1);
const tilde = (path) => base?.home && path.startsWith(`${base.home}/`) ? `~${path.slice(base.home.length)}` : path;
const sourceKey = (file) => `${file.origin}:${file.path}`;
const button = (action, value, label, attrs = "") =>
  `<button type="button" data-${action}="${esc(value)}" ${attrs}>${label}</button>`;
const table = (headers, rows) => `<table><thead><tr>${headers.map((label) => `<th>${label}</th>`).join("")}</tr></thead>
  <tbody>${rows.map((cells) => `<tr>${cells.map((cell) => `<td>${cell}</td>`).join("")}</tr>`).join("")}</tbody></table>`;

let base, outside, missing = false, busy = false, refreshID = 0;
let preview = { sections: [], overrides: [] }, lines = [""], paintedText, paintedPreview;
let groups = [], readers = {}, agent = "claude", globals = [];
const tabs = new Map();
let active = "", inlineOverride;
let caret = { line: 1, col: 1 }, section, overrides = [], problems = [];
let panel = "preview", panelOpen = true, panelHTML = "", job = "", previewTimer;

const dirty = () => Boolean(base && base.text !== text.value);
const blocked = () => Boolean(outside || missing);

async function request(path, data, signal) {
  const response = await fetch(path, { signal, ...data && {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(data),
  } });
  const result = await response.json();
  if (!response.ok) throw Object.assign(new Error(result.error), { status: response.status });
  return result;
}

function message(value = "") {
  $("message").textContent = value;
  $("message").hidden = !value;
}

function showConflict() {
  $("conflict").hidden = !base || !blocked();
  for (const control of $("conflict").querySelectorAll("button")) control.disabled = busy || !outside;
}

function take(project, draft = project.text) {
  const { selectionStart, selectionEnd, selectionDirection, scrollTop, scrollLeft } = text;
  base = project;
  outside = null;
  missing = false;
  preview = project;
  text.disabled = false;
  if (text.value !== draft) {
    text.value = draft;
    text.setSelectionRange(selectionStart, selectionEnd, selectionDirection);
    text.scrollTop = scrollTop;
    text.scrollLeft = scrollLeft;
  }
  if (draft !== project.text) schedulePreview();
}

function setSources(data) {
  groups = data.groups;
  readers = data.readers;
  if (!$("agent").options.length) {
    $("agent").innerHTML = groups.map(({ agent }) => `<option value="${esc(agent)}">${esc(agent)}</option>`).join("");
  }
  for (const option of $("agent").options) {
    option.textContent = option.value + (readers[option.value] ? "" : " !");
  }
  const files = new Map(groups.flatMap(({ files }) => files.map((file) => [sourceKey(file), file])));
  paintedPreview = null;
  globals = [...files.values()].filter((file) => file.origin === "Global").map((file) => ({
    file, lines: split(file.text),
  }));
  for (const [key, file] of tabs) {
    if (files.has(key)) tabs.set(key, files.get(key));
    else tabs.set(key, { ...file, unavailable: true });
  }
}

async function refresh() {
  if (busy) return;
  const id = ++refreshID;
  const [project, sources] = await Promise.allSettled([request("/project"), request("/sources")]);
  if (id !== refreshID || busy) return;
  if (project.status === "fulfilled") {
    const next = project.value;
    missing = false;
    if (!dirty()) take(next);
    else if (next.revision !== base.revision) outside = next;
    else outside = null;
  } else if (project.reason.status === 409 && base) {
    outside = null;
    missing = true;
  } else message(project.reason.message);
  if (sources.status === "fulfilled") setSources(sources.value);
  else message(sources.reason.message);
  update();
}

let pendingPreview;
function schedulePreview() {
  clearTimeout(previewTimer);
  pendingPreview?.abort();
  if (!dirty()) {
    preview = base;
    return;
  }
  const value = text.value;
  previewTimer = setTimeout(async () => {
    const controller = new AbortController();
    pendingPreview = controller;
    try {
      const result = await request("/preview", { text: value }, controller.signal);
      if (text.value === value && dirty()) {
        preview = result;
        update();
      }
    } catch (error) {
      if (error.name !== "AbortError") message(error.message);
    }
  }, 100);
}

async function write(path, data, revision) {
  if (busy) return false;
  busy = true;
  refreshID++;
  const before = text.value;
  showConflict();
  drawStatus();
  try {
    const project = await request(path, { ...data, revision });
    take(project, text.value === before ? project.text : text.value);
    message();
    return true;
  } catch (error) {
    if (error.status !== 409 || !dirty()) message(error.message);
    return false;
  } finally {
    busy = false;
    update();
    void refresh();
  }
}

function save(keep = false) {
  if (!dirty() || blocked() && (!keep || !outside)) return;
  return write("/project", { text: text.value }, keep ? outside.revision : base.revision);
}

let tabsHTML = "";
function drawTabs() {
  const tab = (key, name, title) => `<div class="tab" title="${esc(title)}">${button("tab", key, esc(name),
    `${selected}="${key === active}"`)}${key ? button("close", key, "×", `aria-label="Close ${esc(name)}"`) : ""}</div>`;
  const html = tab("", "AGENTS.md" + (dirty() ? " ●" : ""), base?.file || "AGENTS.md") +
    [...tabs].map(([key, file]) => tab(key, short(file.path), file.path)).join("");
  if (tabsHTML !== html) {
    tabsHTML = html;
    $("tabs").innerHTML = html;
    $("tabs").querySelector('[aria-current="true"]')?.scrollIntoView({ block: "nearest", inline: "nearest" });
  }
}

function drawOutline() {
  const stale = overrides.filter((entry) => !entry.hit).length;
  $("outline").innerHTML = preview.sections.map((item) => button("line", item.line,
    `<span class="icon">${icons[item.type]}</span><span class="name">${esc(item.name)}</span>` +
    `<span class="detail">${item.name === "Overrides" ? overrides.length + (stale ? ` · <span class="warn">${stale}!</span>` : "") : item.tokens}</span>`,
    `class="row" ${selected}="${!active && item === section}" title="${esc(item.type)} · ~${item.tokens} tok"`,
  )).join("");
}

function drawSources() {
  const files = groups.find((group) => group.agent === agent)?.files || [];
  const originGroups = Map.groupBy(files, (file) => file.origin);
  const directory = base?.file.slice(0, base.file.lastIndexOf("/") + 1);
  const nav = $("sources");
  nav.replaceChildren();
  for (const [origin, entries] of originGroups) {
    nav.insertAdjacentHTML("beforeend", `<div class="origin">${esc(origin)}${origin === "Nested" ? " · on demand" : ""}</div>`);
    const depth = new Map();
    for (const file of entries) {
      const key = sourceKey(file);
      const level = file.via ? (depth.get(file.via) ?? 0) + 1 : 0;
      depth.set(file.path, level);
      const project = file.path === base?.file;
      const name = directory && file.path.startsWith(directory) ? file.path.slice(directory.length) : short(file.path);
      nav.insertAdjacentHTML("beforeend", button("source", key,
        `<span class="icon">${level ? "└" : "▪"}</span><span class="name">${esc(name)}</span><span class="detail">${file.tokens}</span>`,
        `class="row" ${selected}="${active === key || !active && project}" title="${esc(file.path)}"`,
      ));
      if (level) nav.lastElementChild.style.paddingInlineStart = `${14 + level * 12}px`;
    }
  }
}

function findGlobal(quote) {
  if (!quote) return null;
  let partial = null;
  for (const { file, lines } of globals) {
    for (let index = 0; index < lines.length; index++) {
      if (lines[index] === quote) return { file, line: index + 1 };
      if (!partial && lines[index].includes(quote)) partial = { file, line: index + 1 };
    }
  }
  return partial;
}

function drawBuffer() {
  const stale = new Set(overrides.filter((entry) => !entry.hit).map((entry) => entry.line));
  const lenses = new Map(preview.sections.slice(1).map((item) => [item.line,
    button("line", item.line, `${item.type} · ~${item.tokens} tok`),
  ]));
  for (const [index, entry] of overrides.entries()) {
    const hit = entry.hit;
    lenses.set(entry.line, hit
      ? button("hit", index, `✓ ${esc(short(hit.file.path))}:${hit.line}`, `class="add" title="${esc(hit.file.path)}:${hit.line}"`)
      : '<span class="warn">STALE</span>');
  }
  const tokens = syntax(lines, stale);
  $("highlight").innerHTML = tokens.map(({ html }) => `<div>${html}</div>`).join("");
  $("numbers").innerHTML = lines.map((_, index) => `<div>${index + 1}</div>`).join("");
  $("lens").innerHTML = tokens.map(({ diagram }, index) => `<div>${lenses.get(index + 1) ||
    (diagram ? button("diagram", index + 1, "preview ↓") : "")}</div>`).join("");
  paintedText = text.value;
  paintedPreview = preview;
  markLine($("numbers"), 0);
  markLine($("highlight"), 0);
  syncScroll();
}

function syncScroll() {
  const y = -text.scrollTop;
  $("numbers").style.transform = $("lens").style.transform = `translateY(${y}px)`;
  $("highlight").style.transform = `translate(${-text.scrollLeft}px, ${y}px)`;
}

function markLine(element, previous) {
  element.children[previous - 1]?.classList.remove("current");
  element.children[caret.line - 1]?.classList.add("current");
}

function track() {
  const before = text.value.slice(0, text.selectionStart);
  const next = { line: split(before).length, col: before.length - before.lastIndexOf("\n") };
  if (caret.line !== next.line) {
    const previous = caret.line;
    caret = next;
    markLine($("numbers"), previous);
    markLine($("highlight"), previous);
  } else caret = next;
  const current = preview.sections.findLast((item) => item.line <= caret.line);
  if (current !== section) {
    section = current;
    drawOutline();
    drawPanel();
  }
  drawStatus();
}

function goTo(line) {
  activate("");
  const index = Math.min(Math.max(1, Number(line)), lines.length) - 1;
  const offset = lines.slice(0, index).reduce((total, value) => total + value.length + 1, 0);
  text.focus({ preventScroll: true });
  text.setSelectionRange(offset, offset);
  const top = index * 20 + 8;
  if (top < text.scrollTop || top + 20 > text.scrollTop + text.clientHeight) {
    text.scrollTop = top - text.clientHeight / 3;
  }
  text.scrollLeft = 0;
  syncScroll();
  track();
}

function drawStatus() {
  const tokens = Math.ceil(text.value.length / 4);
  const stale = overrides.filter((entry) => !entry.hit).length;
  $("position").textContent = active ? "" : `Ln ${caret.line}, Col ${caret.col}`;
  $("section-status").textContent = !active && section ? `${section.name} · ${section.type} · ~${section.tokens} tok` : "";
  $("override-status").innerHTML = `${overrides.length} Overrides${stale ? ` · <span class="warn">${stale} STALE</span>` : ""}`;
  budget.firstElementChild.value = tokens;
  budget.lastElementChild.textContent = `~${tokens.toLocaleString("en-US")} / 1,000`;
  budget.classList.toggle("warn", tokens > 1000);
  $("save").textContent = busy ? "saving…" : dirty() ? "● modified" : "saved";
  $("save").disabled = !dirty() || busy || blocked();
}

function makeProblems() {
  return [
    ...overrides.filter((entry) => !entry.hit).map((entry) => [entry.line, "STALE", entry.quote]),
    ...preview.sections.flatMap((item) => (item.pages || []).filter((page) => !page.exists)
      .map((page) => [page.line, "Missing page", page.path])),
    ...(text.value.length > 4000 ? [[1, "Over budget", `~${Math.ceil(text.value.length / 4)} / 1,000`]] : []),
    ...Object.entries(readers).filter(([, read]) => !read).map(([id]) => [id, "AGENTS.md unread", id]),
  ];
}

function drawPanel() {
  $("panel").hidden = !panelOpen;
  document.querySelector('.tab-strip [data-action="panel"]').setAttribute("aria-expanded", panelOpen);
  for (const control of document.querySelectorAll("[data-panel]")) {
    control.setAttribute("aria-pressed", control.dataset.panel === panel);
  }
  $("problem-count").textContent = problems.length || "";
  $("preview").hidden = panel !== "preview";
  $("problems").hidden = panel !== "problems";
  if (!panelOpen) return;
  $("panel-section").textContent = panel === "preview" && section ? section.name : "";
  if (panel === "problems") {
    $("problems").innerHTML = problems.length ? table(["Problem", ""], problems.map(([target, label, detail]) => [
      button(typeof target === "number" ? "line" : "agent", target, esc(label), 'class="warn"'),
      esc(detail),
    ])) : "No Problems";
    return;
  }
  let html = "";
  if (section?.name === "Overrides") {
    html = table(["Line", "Global text", "Replacement", "Found in"], overrides.map((entry, index) => [
      button("line", entry.line, entry.line), esc(entry.quote),
      `<span class="add">${esc(entry.replacement)}</span>`,
      entry.hit ? button("hit", index, `${esc(short(entry.hit.file.path))}:${entry.hit.line}`,
        `title="${esc(entry.hit.file.path)}"`) : '<span class="warn">STALE</span>',
    ]));
  } else if (section?.type === "pages") {
    html = table(["Name", "Path", "Read when", ""], section.pages.map((page) => [
      esc(page.name), button("line", page.line, esc(page.path), 'class="link"'), esc(page.note),
      page.exists ? '<span class="add">✓ exists</span>' : '<span class="warn">✗ missing</span>',
    ]));
  } else if (section) {
    html = section.html || "";
    if (section.type === "diagram") {
      html += `<form data-form="diagram" data-section="${esc(section.name)}">
        <input name="change" aria-label="Describe a diagram change" placeholder="describe a change → local agent command" required>
        <button>Request</button>
      </form><output id="job" role="status"></output>`;
    }
  }
  if (html !== panelHTML) {
    panelHTML = html;
    $("preview").innerHTML = html;
    $("preview").scrollTop = 0;
  }
  if ($("job")) $("job").textContent = job;
}

let sourceHTML = "";
function drawSource() {
  const file = tabs.get(active);
  if (!file) return;
  const view = $("source");
  const rows = split(file.text);
  const global = file.origin === "Global" && !file.unavailable;
  const replacementForm = (line) => `<form data-form="override" data-line="${line}">
    <label for="replacement" class="arrow">→ here:</label>
    <input id="replacement" name="replacement" spellcheck="false" aria-label="Replacement" required>
    <button>Save</button>
  </form>`;
  const heading = [tilde(file.path), file.origin, file.onDemand && "on demand", `~${file.tokens} tok`,
    file.unavailable && "Unavailable"].filter(Boolean).join(" · ");
  const html = `<div class="source-heading" title="${esc(heading)}">${esc(heading)}</div>` +
    syntax(rows).map(({ html, rule }, index) => {
      const line = index + 1;
      const hits = global ? (base?.overrides || []).filter((entry) => entry.quote && rows[index].includes(entry.quote)) : [];
      return `<div class="source-line" id="source-line-${line}"><span class="number">${line}</span><span class="line">${hits.length
        ? `<s>${html}</s>${hits.map((entry) => `<span class="add"><br>→ ${esc(entry.replacement)}</span>`).join("")}`
        : html}</span>${global && rule && !hits.length ? button("override", line, "Override") : ""}</div>` +
        (inlineOverride?.key === active && inlineOverride.line === line ? replacementForm(line) : "");
    }).join("");
  if (html === sourceHTML) return;
  const field = $("replacement");
  const draft = field?.value;
  const focused = document.activeElement === field;
  sourceHTML = html;
  view.innerHTML = html;
  if ($("replacement") && draft !== undefined) {
    $("replacement").value = draft;
    if (focused) $("replacement").focus({ preventScroll: true });
  }
}

function update() {
  lines = split(text.value);
  overrides = (preview.overrides || []).map((entry) => ({ ...entry, hit: findGlobal(entry.quote) }));
  problems = makeProblems();
  if (text.value !== paintedText || preview !== paintedPreview) drawBuffer();
  track();
  drawTabs();
  drawOutline();
  drawSources();
  drawSource();
  drawPanel();
  showConflict();
}

function activate(key) {
  if (active !== key) inlineOverride = null;
  active = key;
  $("buffer").hidden = Boolean(key);
  $("source").hidden = !key;
  if (!key) syncScroll();
  else $("source").scrollTop = 0;
  update();
}

function openFile(file, line) {
  if (!file) return;
  if (file.path === base?.file) {
    goTo(line || 1);
    return;
  }
  const key = sourceKey(file);
  tabs.set(key, file);
  activate(key);
  if (line) $("source-line-" + line)?.scrollIntoView({ block: "center" });
}

function openSource(key) {
  const group = groups.find((entry) => entry.agent === agent);
  openFile(group?.files.find((file) => sourceKey(file) === key));
}

let matches = [], choice = 0;
function matchScore(label, needle) {
  let score = 0, end = 0;
  for (const char of needle) {
    const index = label.indexOf(char, end);
    if (index < 0) return Infinity;
    score += index - end;
    end = index + 1;
  }
  return score + label.length / 1000;
}

function drawMatches() {
  const needle = query.value.toLowerCase().trim();
  const entries = [
    ...preview.sections.map((item) => ({ label: item.name, detail: item.type, action: () => goTo(item.line) })),
    ...[...new Map([...(groups.find((item) => item.agent === agent)?.files || []),
      ...groups.flatMap((item) => item.files)].map((file) => [sourceKey(file), file])).values()]
      .map((file) => ({ label: short(file.path), detail: file.origin, search: file.path, action: () => openFile(file) })),
  ];
  matches = entries.map((entry) => ({ ...entry, score: Math.min(
    matchScore(entry.label.toLowerCase(), needle), 1 + matchScore((entry.search || "").toLowerCase(), needle),
  ) })).filter((entry) => Number.isFinite(entry.score))
    .sort((a, b) => needle ? a.score - b.score : 0).slice(0, 10);
  $("matches").innerHTML = matches.map((entry, index) => button("match", index,
    `<span class="name">${esc(entry.label)}</span><span class="detail">${esc(entry.detail)}</span>`,
    `class="row" aria-selected="${choice === index}" title="${esc(entry.search || entry.label)}"`,
  )).join("");
}

function choose(index) {
  if (!matches[index]) return;
  jump.close();
  matches[index].action();
}

function setAgent(id) {
  agent = id;
  $("agent").value = id;
  drawSources();
}

function showPanel(view = panel) {
  panelOpen = true;
  panel = view;
  drawPanel();
}

$("agent").addEventListener("change", (event) => setAgent(event.target.value));
text.addEventListener("scroll", syncScroll);
text.addEventListener("selectionchange", track);
text.addEventListener("input", () => {
  message();
  schedulePreview();
  if (!dirty() && outside) take(outside);
  update();
});
query.addEventListener("input", () => { choice = 0; drawMatches(); });
jump.addEventListener("click", (event) => {
  if (event.target === jump) jump.close();
});

const actions = {
  save: () => save(),
  reload: () => {
    if (!outside) return;
    take(outside);
    message();
    update();
  },
  keep: () => save(true),
  panel: () => { panelOpen = !panelOpen; drawPanel(); },
  jump: () => {
    if (jump.open) return jump.close();
    query.value = "";
    choice = 0;
    drawMatches();
    jump.showModal();
  },
};
document.addEventListener("click", (event) => {
  const control = event.target.closest("button");
  if (!control) return;
  const data = control.dataset;
  if (data.action) actions[data.action]();
  else if (data.line) goTo(data.line);
  else if (data.source) openSource(data.source);
  else if (data.tab !== undefined) activate(data.tab);
  else if (data.close) {
    tabs.delete(data.close);
    activate(active === data.close ? [...tabs.keys()].at(-1) || "" : active);
  } else if (data.panel) showPanel(data.panel);
  else if (data.hit !== undefined) {
    const hit = overrides[Number(data.hit)]?.hit;
    if (hit) openFile(hit.file, hit.line);
  } else if (data.diagram) {
    goTo(data.diagram);
    showPanel("preview");
  } else if (data.override) {
    const file = tabs.get(active);
    const line = Number(data.override);
    inlineOverride = { key: active, line, quote: split(file.text)[line - 1].replace(/^\s*(?:[-*+]|\d+[.)])\s+/, "") };
    drawSource();
    $("replacement")?.focus();
  } else if (data.match) choose(Number(data.match));
  else if (data.agent) {
    setAgent(data.agent);
    $("agent").focus();
  }
});

document.addEventListener("submit", async (event) => {
  const form = event.target;
  if (!form.dataset.form) return;
  event.preventDefault();
  if (form.dataset.form === "override") {
    if (!inlineOverride || blocked() || busy) return;
    if (dirty()) return message("Save AGENTS.md before adding an Override.");
    const replacement = new FormData(form).get("replacement");
    if (await write("/override", { quote: inlineOverride.quote, replacement }, base.revision)) {
      inlineOverride = null;
      drawSource();
    }
  } else {
    const change = new FormData(form).get("change");
    if (dirty() || blocked() || busy) return message("Save AGENTS.md before requesting a diagram change.");
    try {
      await request("/diagram", { name: form.dataset.section, change });
      form.reset();
      message();
    } catch (error) {
      message(error.message);
    }
  }
});

document.addEventListener("keydown", (event) => {
  const key = event.key.toLowerCase();
  if ((event.metaKey || event.ctrlKey) && actions[{ s: "save", j: "panel", p: "jump" }[key]]) {
    event.preventDefault();
    actions[{ s: "save", j: "panel", p: "jump" }[key]]();
  } else if (jump.open && ["arrowdown", "arrowup", "enter"].includes(key)) {
    event.preventDefault();
    if (key === "enter") choose(choice);
    else if (matches.length) {
      choice = (choice + (key === "arrowdown" ? 1 : matches.length - 1)) % matches.length;
      drawMatches();
      $("matches").children[choice]?.scrollIntoView({ block: "nearest" });
    }
  } else if (key === "escape" && jump.open) {
    event.preventDefault();
    jump.close();
  } else if (key === "escape" && inlineOverride) {
    const line = inlineOverride.line;
    inlineOverride = null;
    drawSource();
    $("source-line-" + line)?.querySelector("button")?.focus();
  }
});

const events = new EventSource("/events");
events.addEventListener("open", refresh);
events.addEventListener("change", refresh);
events.addEventListener("job", (event) => {
  job = JSON.parse(event.data);
  if ($("job")) $("job").textContent = job;
});
void refresh();
