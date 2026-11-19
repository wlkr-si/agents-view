const host = location.origin;
const diagrams = document.querySelector("#diagrams");
const mermaid = window.mermaid;
const secure = ["secure", "securityLevel", "startOnLoad", "suppressErrorRendering",
  "maxTextSize", "maxEdges", "theme", "themeVariables", "htmlLabels"];
let pending, rendering = false, revision = 0;

new ResizeObserver(() => {
  parent.postMessage({ height: diagrams.getBoundingClientRect().height }, host);
}).observe(diagrams);

window.addEventListener("message", ({ source, origin, data }) => {
  if (source !== parent || origin !== host || !Array.isArray(data?.texts)) return;
  pending = data;
  if (!rendering) void render();
});

async function render() {
  rendering = true;
  while (pending) {
    const { texts, colors: [background, fill, foreground, accent, orange], dark } = pending;
    pending = undefined;
    document.documentElement.style.colorScheme = dark ? "dark" : "light";
    document.documentElement.style.backgroundColor = background;
    const drawings = document.createDocumentFragment();
    for (const diagramText of texts) {
      if (pending) break;
      const drawing = document.createElement("div");
      drawings.append(drawing);
      try {
        mermaid.initialize({
          secure, securityLevel: "strict", startOnLoad: false,
          suppressErrorRendering: true, htmlLabels: false, theme: "base",
          themeVariables: {
            darkMode: dark, background, textColor: foreground, lineColor: foreground,
            primaryColor: fill, primaryTextColor: foreground, primaryBorderColor: accent,
            secondaryColor: fill, tertiaryColor: fill, edgeLabelBackground: background,
          },
        });
        const scratch = document.createElement("div");
        scratch.style.cssText = "position:absolute;inset:0 0 auto;visibility:hidden";
        document.body.append(scratch);
        try {
          const { svg } = await mermaid.render(`diagram-${++revision}`, diagramText, scratch);
          drawing.innerHTML = svg;
          for (const link of drawing.querySelectorAll("a")) {
            link.removeAttribute("href");
            link.removeAttribute("xlink:href");
          }
        } finally {
          scratch.remove();
        }
      } catch (error) {
        drawing.className = "error";
        drawing.style.color = orange;
        const detail = (error?.message || String(error)).split(/[\r\n\u2028\u2029]/, 1)[0].replace(/:$/, "");
        drawing.textContent = `Diagram error — ${detail.slice(0, 200)}`;
        drawing.title = drawing.textContent;
      }
    }
    if (!pending) diagrams.replaceChildren(drawings);
  }
  rendering = false;
}
