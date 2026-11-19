export function escape(text) {
  return String(text).replace(/[&<>"']/g, (char) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  })[char]);
}

const span = (className, text) => `<span class="${className}">${escape(text)}</span>`;
const inline = /\x60+[^\x60]+\x60+|\[[^\]]+\]\((?:[^()]|\([^()]*\))*\)/g;

function inlineHTML(text) {
  let html = "", end = 0;
  for (const match of text.matchAll(inline)) {
    html += escape(text.slice(end, match.index));
    html += span(match[0][0] === "[" ? "link" : "code-token", match[0]);
    end = match.index + match[0].length;
  }
  return html + escape(text.slice(end));
}

export function syntax(lines, stale = new Set()) {
  let fence;
  return lines.map((text, index) => {
    const number = index + 1;
    const marker = /^ {0,3}(`{3,}|~{3,})(.*)$/.exec(text);
    if (fence) {
      if (marker && marker[1][0] === fence[0] && marker[1].length >= fence.length && !marker[2].trim()) {
        fence = null;
      }
      return { html: span("code-token", text) };
    }
    if (marker && (marker[1][0] !== "`" || !marker[2].includes("`"))) {
      fence = marker[1];
      return { html: span("code-token", text), diagram: marker[2].trim() === "mermaid" };
    }
    const setext = /^ {0,3}(?:=+|-+)\s*$/;
    if (/^ {0,3}#{1,6}(?:\s|$)/.test(text) || setext.test(text) ||
        (text.trim() && setext.test(lines[index + 1] || "") && !/^\s*(?:[-+*]\s|@)/.test(text))) {
      return { html: span("heading", text) };
    }
    if (/^\s*(?:<!--|@\S)/.test(text)) return { html: span("comment", text) };

    const override = /^(- Global: )("(?:[^"\\]|\\.)*"|".+?")(\s+→ here:\s+)(.*)$/.exec(text);
    if (override) {
      return { html: span("global", override[1]) + span(stale.has(number) ? "quote stale" : "quote", override[2]) +
        span("arrow", override[3]) + inlineHTML(override[4]) };
    }
    const bullet = /^\s*([-+*]|\d+[.)])(?=\s)/.exec(text)?.[0] || "";
    const quote = /^\s*>/.test(text);
    return {
      html: quote ? span("quote", text) : span("marker", bullet) + inlineHTML(text.slice(bullet.length)),
      rule: Boolean(text.trim()) && !/^\s*(?:[-+*]\s+)?@\S/.test(text),
    };
  });
}
