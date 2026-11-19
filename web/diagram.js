const host = new URL(location.href).origin;
let revision = 0;

window.addEventListener("message", async ({ source, origin, data }) => {
  if (source !== parent || origin !== host || data?.kind !== "diagram") return;
  const current = ++revision;
  try {
    const { default: mermaid } = await import(
      `https://cdn.jsdelivr.net/npm/mermaid@${data.version}/dist/mermaid.esm.min.mjs`
    );
    mermaid.initialize({
      startOnLoad: false,
      securityLevel: "strict",
      suppressErrorRendering: true,
      theme: data.dark ? "dark" : "neutral",
      htmlLabels: false,
      flowchart: {
        useMaxWidth: true,
        wrappingWidth: 155,
        rankSpacing: 28,
        nodeSpacing: 24,
      },
    });
    const result = await mermaid.render(`graph-${current}`, data.graph);
    if (current !== revision) return;
    document.body.innerHTML = result.svg;
  } catch (error) {
    if (current !== revision) return;
    const fallback = document.createElement("pre");
    fallback.textContent = `Diagram unavailable: ${error.message}\n\n${data.graph}`;
    document.body.replaceChildren(fallback);
  }
  const observer = new ResizeObserver(() => {
    if (current !== revision) {
      observer.disconnect();
      return;
    }
    parent.postMessage(
      { kind: "diagram-size", height: document.body.scrollHeight },
      host,
    );
  });
  observer.observe(document.body);
});
