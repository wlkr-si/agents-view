const trigger = document.querySelector("#theme");
const stylesheet = document.querySelector('link[href="/theme.css"]');
const picker = document.createElement("dialog");
picker.id = "theme-picker";
picker.setAttribute("aria-labelledby", "theme-title");
picker.innerHTML = `
  <header><h2 id="theme-title">Theme</h2><button type="button" aria-label="Close">×</button></header>
  <p role="status" hidden></p>
  <div id="theme-options"></div>
`;
document.body.append(picker);

const status = picker.querySelector("p");
const options = picker.querySelector("#theme-options");
const labels = {
  system: "System",
  "norwegian-wood-sunrise": "Norwegian Wood Sunrise",
  "flexoki-light": "Flexoki Light",
  omarchy: "Omarchy",
};
let revision = 0;

function refresh() {
  stylesheet.href = `/theme.css?v=${++revision}`;
}

function showProblem(message = "") {
  status.hidden = !message;
  status.textContent = message;
}

async function request(url, init) {
  const response = await fetch(url, init);
  const data = await response.json();
  if (!response.ok) throw new Error(data.error);
  return data;
}

async function choose(url, body, contentType) {
  try {
    await request(url, {
      method: "POST",
      headers: { "Content-Type": contentType },
      body,
    });
    refresh();
    picker.close();
  } catch (error) {
    if (!picker.open) picker.showModal();
    showProblem(error.message);
  }
}

trigger.setAttribute("aria-label", "Theme");
trigger.setAttribute("aria-haspopup", "dialog");
picker.querySelector("button").addEventListener("click", () => picker.close());
trigger.addEventListener("click", async () => {
  options.replaceChildren();
  showProblem();
  picker.showModal();
  try {
    const { active, names, problem } = await request("/theme");
    showProblem(problem);
    for (const name of names) {
      const button = document.createElement("button");
      button.type = "button";
      button.textContent = Object.hasOwn(labels, name) ? labels[name] : name;
      if (name === active) button.setAttribute("aria-current", "true");
      button.addEventListener("click", () =>
        choose("/theme", JSON.stringify({ name }), "application/json"),
      );
      options.append(button);
    }
  } catch (error) {
    showProblem(error.message);
  }
});

new EventSource("/events").addEventListener("theme", refresh);

window.addEventListener("dragover", (event) => {
  if (event.dataTransfer.types.includes("Files")) event.preventDefault();
});
window.addEventListener("drop", async (event) => {
  const files = [...event.dataTransfer.files];
  if (!files.length) return;
  event.preventDefault();
  const file = files.find((item) => /\.toml$/i.test(item.name));
  if (!file || file.size > 16 * 1024) {
    if (!picker.open) picker.showModal();
    showProblem("Drop a colors.toml file up to 16 KiB.");
    return;
  }
  await choose(
    `/theme?${new URLSearchParams({ file: file.name })}`,
    file,
    "application/toml",
  );
});
