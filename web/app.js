const $ = (selector) => document.querySelector(selector);
const view = $("#view");
const alert = $("#alert");
let tab = "project";
let agent = "claude";
let project;
let revision = 0;
let editing = 0;
let pending = false;
let jobStatus = "";
const expanded = new Set();
const diagrams = new Map();
const preferDark = matchMedia("(prefers-color-scheme: dark)");

function element(tag, options = {}, ...children) {
  const node = Object.assign(document.createElement(tag), options);
  node.append(...children);
  return node;
}

function button(text, handler, className = "") {
  const node = element("button", {
    textContent: text,
    type: "button",
    className,
  });
  node.addEventListener("click", handler);
  return node;
}

function status(message = "") {
  alert.textContent = message;
  alert.hidden = !message;
}

async function request(path, data) {
  const response = await fetch(
    path,
    data === undefined
      ? undefined
      : {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(data),
        },
  );
  const result = await response.json();
  if (!response.ok) throw new Error(result.error || "Request failed.");
  return result;
}

function updateTheme(theme) {
  document.documentElement.dataset.theme = theme;
  $("#theme").textContent = theme === "dark" ? "Light mode" : "Dark mode";
  for (const send of diagrams.values()) send();
}

updateTheme(
  localStorage.getItem("theme") ?? (preferDark.matches ? "dark" : "light"),
);
preferDark.addEventListener("change", () => {
  if (!localStorage.getItem("theme"))
    updateTheme(preferDark.matches ? "dark" : "light");
});
$("#theme").addEventListener("click", () => {
  const next =
    document.documentElement.dataset.theme === "dark" ? "light" : "dark";
  localStorage.setItem("theme", next);
  updateTheme(next);
});

function editForm({
  value = "",
  label,
  action,
  save,
  cancel,
  single = false,
  finish = () => {},
}) {
  const input = element(single ? "input" : "textarea", {
    value,
    className: action === "Save" ? "editor" : "",
  });
  input.setAttribute("aria-label", label);
  if (action !== "Save") input.placeholder = label;
  const submit = element("button", {
    type: "submit",
    textContent: action,
    className: "primary",
  });
  const actions = element("div", { className: "actions" }, submit);
  if (cancel) actions.append(button("Cancel", cancel));
  const form = element("form", {}, input, actions);
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    submit.disabled = true;
    status();
    try {
      await save(input.value);
      finish();
    } catch (error) {
      status(error.message);
    } finally {
      submit.disabled = false;
    }
  });
  return { form, input };
}

function diagram(graph, title) {
  const frame = element("iframe", { src: "/diagram", title });
  frame.setAttribute("sandbox", "allow-scripts");
  function send() {
    frame.contentWindow?.postMessage(
      {
        kind: "diagram",
        graph,
        version: project.mermaidVersion,
        dark: document.documentElement.dataset.theme === "dark",
      },
      "*",
    );
  }
  frame.addEventListener("load", send);
  diagrams.set(frame, send);
  return frame;
}

window.addEventListener("message", ({ source, origin, data }) => {
  if (
    origin !== "null" ||
    data?.kind !== "diagram-size" ||
    !Number.isFinite(data.height)
  )
    return;
  for (const frame of diagrams.keys()) {
    if (frame.contentWindow === source) frame.dataset.ready = "true";
    frame.style.height = `${Math.min(2400, Math.max(24, Math.ceil(data.height)))}px`;
  }
});

function closeEditor() {
  editing--;
  if (pending && editing === 0) refresh();
}

function renderProject() {
  const budget = element(
    "div",
    { className: "toolbar meta" },
    "AGENTS.md",
    element(
      "span",
      { className: project.tokens > 1000 ? "stale" : "" },
      `~${project.tokens} / 1,000 tokens`,
    ),
  );
  view.append(budget);
  for (const section of project.sections) {
    const head = element(
      "div",
      { className: "card-head" },
      element("h2", {}, section.name),
    );
    const content = element("div", { className: "content" });
    content.innerHTML = section.html;
    if (section.name === "Intro") {
      const title = content.firstElementChild;
      if (title?.tagName === "H1" && title.textContent === project.project)
        title.remove();
    }
    if (section.type === "pages") {
      for (const link of content.querySelectorAll("a[href]")) {
        const href = link.getAttribute("href");
        if (!/^(?:[\w+.-]+:|\/|#)/.test(href)) link.href = `/page/${href}`;
      }
    }
    const card = element("section", { className: "card" }, head, content);
    if (section.type === "diagram") {
      for (const code of content.querySelectorAll(
        "pre > code.language-mermaid",
      ))
        code.parentElement.replaceWith(diagram(code.textContent, section.name));
      const job = element("span", {
        className: "meta job",
        textContent: jobStatus,
      });
      const { form, input } = editForm({
        label: "Describe a diagram change",
        action: "Update diagram",
        save: (change) => request("/diagram", { name: section.name, change }),
        finish: () => {
          input.value = "";
          if (input.dataset.draft) {
            delete input.dataset.draft;
            closeEditor();
          }
        },
      });
      input.addEventListener("input", () => {
        if (input.value && !input.dataset.draft) {
          input.dataset.draft = "yes";
          editing++;
        }
        if (!input.value && input.dataset.draft) {
          delete input.dataset.draft;
          closeEditor();
        }
      });
      form.querySelector(".actions").append(job);
      card.append(form);
    } else {
      const edit = button("Edit", () => {
        editing++;
        edit.hidden = true;
        content.hidden = true;
        function close() {
          form.remove();
          edit.hidden = false;
          content.hidden = false;
          closeEditor();
        }
        const { form, input } = editForm({
          value: section.body,
          label: `Edit ${section.name}`,
          action: "Save",
          save: (body) =>
            request("/section", {
              name: section.name,
              body,
              expected: section.body,
            }),
          cancel: close,
          finish: () => {
            close();
            refresh();
          },
        });
        card.append(form);
        input.focus();
      });
      head.append(edit);
      edit.setAttribute("aria-label", `Edit ${section.name}`);
    }
    view.append(card);
  }
}

function renderSource(file, entries) {
  const key = `${agent}:${file.path}`;
  const displayPath =
    file.origin === "Project" || file.origin === "Nested"
      ? file.path.replace(`${project.directory}/`, "")
      : file.path;
  const details = element(
    "details",
    { className: "card", open: expanded.has(key) },
    element(
      "summary",
      {},
      displayPath,
      element("span", { className: "meta" }, `~${file.tokens} tokens`),
    ),
  );
  if (file.via)
    details.append(
      element("span", { className: "meta via" }, `Imported by ${file.via}`),
    );
  details.addEventListener("toggle", () => {
    if (details.open) expanded.add(key);
    else expanded.delete(key);
  });
  let rendered = false;
  function renderLines() {
    if (rendered || !details.open) return;
    rendered = true;
    const lines = element("div", { className: "source-lines" });
    for (const [index, line] of file.text.split(/\r?\n/).entries()) {
      const content = element("span", { className: "line-content" }, line);
      const row = element(
        "div",
        { className: "source-line" },
        element("span", {
          className: "line-number",
          textContent: String(index + 1),
        }),
        content,
      );
      if (file.origin === "Global" && line.trim()) {
        const matching = entries.filter(
          (entry) => entry.quote && line.includes(entry.quote),
        );
        if (matching.length) {
          const quote = matching[0].quote;
          const start = line.indexOf(quote);
          content.replaceChildren(
            line.slice(0, start),
            element("s", {}, quote),
            line.slice(start + quote.length),
          );
          for (const entry of matching)
            content.append(
              element(
                "span",
                { className: "override-text" },
                `→ ${entry.replacement}`,
              ),
            );
        }
        const change = button(
          "Change",
          () => {
            editing++;
            change.hidden = true;
            function close() {
              form.remove();
              change.hidden = false;
              closeEditor();
            }
            const { form, input } = editForm({
              label: "Replacement for this project",
              action: "Save override",
              single: true,
              save: (replacement) =>
                request("/override", { quote: line, replacement }),
              cancel: close,
              finish: () => {
                close();
                refresh();
              },
            });
            row.after(form);
            input.focus();
          },
          "line-action",
        );
        change.setAttribute(
          "aria-label",
          `Change line ${index + 1} for this project`,
        );
        change.title = "Change for this project";
        row.append(change);
      }
      lines.append(row);
    }
    details.append(lines);
  }
  details.addEventListener("toggle", renderLines);
  renderLines();
  return details;
}

function renderSources(sources) {
  const files = sources.groups.find((group) => group.agent === agent).files;
  const select = element(
    "select",
    {},
    element("option", { value: "claude" }, "Claude Code"),
    element("option", { value: "codex" }, "Codex"),
  );
  select.value = agent;
  select.setAttribute("aria-label", "Agent");
  select.addEventListener("change", () => {
    agent = select.value;
    editing = 0;
    render("sources", sources);
  });
  view.append(
    element(
      "div",
      { className: "toolbar" },
      select,
      element(
        "span",
        { className: "meta" },
        `${files.length} ${files.length === 1 ? "file" : "files"}`,
      ),
    ),
  );
  for (const entry of sources.overrides.filter((entry) => entry.stale)) {
    view.append(
      element(
        "div",
        { className: "card stale" },
        "STALE ",
        element("s", {}, entry.quote || entry.line),
        element("span", { className: "override-text" }, entry.replacement),
      ),
    );
  }
  let origin;
  for (const file of files) {
    if (origin !== file.origin) {
      origin = file.origin;
      const title = origin === "Nested" ? "Nested · on demand" : origin;
      view.append(element("h2", { className: "source-heading" }, title));
    }
    view.append(renderSource(file, sources.overrides));
  }
}

function render(name, sources) {
  diagrams.clear();
  view.replaceChildren();
  if (name === "project") renderProject();
  else renderSources(sources);
}

async function refresh() {
  if (editing) {
    pending = true;
    return;
  }
  pending = false;
  const current = ++revision;
  try {
    const [document, sources] = await Promise.all([
      request("/project"),
      tab === "sources" ? request("/sources") : null,
    ]);
    if (current !== revision) return;
    if (editing) {
      pending = true;
      return;
    }
    project = document;
    $("#project-name").textContent = document.project;
    window.document.title = `${document.project} · agents-view`;
    render(tab, sources);
  } catch (error) {
    if (current === revision) status(error.message);
  }
}

for (const name of ["project", "sources"]) {
  $(`#tab-${name}`).addEventListener("click", () => {
    if (tab === name) return;
    tab = name;
    editing = 0;
    status();
    for (const item of document.querySelectorAll(".tab")) {
      const active = item.id === `tab-${name}`;
      item.classList.toggle("active", active);
      if (active) item.setAttribute("aria-current", "page");
      else item.removeAttribute("aria-current");
    }
    refresh();
  });
}

const events = new EventSource("/events");
events.addEventListener("change", refresh);
events.addEventListener("job", (event) => {
  jobStatus = JSON.parse(event.data);
  for (const label of document.querySelectorAll(".job"))
    label.textContent = jobStatus;
});
refresh();
