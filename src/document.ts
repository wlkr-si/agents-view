import { randomUUID } from "node:crypto";
import {
  existsSync,
  readFileSync,
  realpathSync,
  renameSync,
  statSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { dirname, resolve } from "node:path";
import MarkdownIt from "markdown-it";

const version = "<!-- agents-view v0 -->";
export const markdown = new MarkdownIt({ html: true });
markdown.renderer.rules.html_block = () => "";
markdown.renderer.rules.html_inline = () => "";
export const tokens = (value: string) => Math.ceil(value.length / 4);

const sectionTypes = {
  diagram: (body: string) =>
    markdown
      .parse(body, {})
      .some(
        (token) => token.type === "fence" && token.info.trim() === "mermaid",
      ),
  pages: (body: string) => pageEntries(body) !== null,
  rules: (body: string) =>
    markdown.parse(body, {}).some((token) => token.type === "bullet_list_open"),
  text: () => true,
};
type SectionType = keyof typeof sectionTypes;

export function pageEntries(body: string) {
  const lines = body.split(/\r?\n/).filter((line) => line.trim());
  if (!lines.length) return null;
  const pages = [];
  for (const line of lines) {
    const match =
      /^ {0,3}[-*+]\s+\[([^\]]+)\]\((<[^>]+>|(?:[^()\s]|\([^()]*\))+)\)(?:\s+—\s+(.+))?$/.exec(
        line,
      );
    if (!match) return null;
    pages.push({
      name: match[1],
      href: match[2].replace(/^<|>$/g, ""),
      note: match[3],
    });
  }
  return pages;
}

function section(
  name: string | null,
  heading: string,
  raw: string,
  start: number,
  end: number,
) {
  const leading = /^(?:[ \t]*\r?\n)*/.exec(raw)?.[0].length ?? 0;
  let trailing = raw.length;
  if (leading < trailing) {
    const ending = /(?:\r?\n[ \t]*)+$/.exec(raw.slice(leading));
    trailing -= ending?.[0].length ?? 0;
  }
  const body = raw.slice(leading, trailing);
  const type =
    (Object.keys(sectionTypes) as SectionType[]).find((type) =>
      sectionTypes[type](body),
    ) ?? "text";
  return {
    name,
    type,
    body,
    heading,
    start,
    end,
    bodyStart: start + leading,
    bodyEnd: start + trailing,
    tokens: tokens(heading + raw),
  };
}

export function parse(text: string) {
  const lines = text.split(/(?<=\n)/);
  const offsets = [0];
  for (const line of lines)
    offsets.push(offsets[offsets.length - 1] + line.length);
  const hasVersion = lines[0]?.replace(/\r?\n$/, "") === version;
  const headings = markdown.parse(text, {}).flatMap((token) => {
    if (
      token.type !== "heading_open" ||
      token.tag !== "h2" ||
      token.level !== 0 ||
      !token.map
    )
      return [];
    const [line, next] = token.map;
    const match = /^ {0,3}##[ \t]+(.+?)\s*$/.exec(lines[line]);
    if (!match) return [];
    return [
      {
        name: match[1].replace(/\s+#+$/, ""),
        start: offsets[line],
        bodyStart: offsets[next],
      },
    ];
  });
  const intro = hasVersion ? offsets[1] : 0;
  const end = headings[0]?.start ?? text.length;
  const sections = [section(null, "", text.slice(intro, end), intro, end)];
  for (const [index, heading] of headings.entries()) {
    const end = headings[index + 1]?.start ?? text.length;
    sections.push(
      section(
        heading.name,
        text.slice(heading.start, heading.bodyStart),
        text.slice(heading.bodyStart, end),
        heading.bodyStart,
        end,
      ),
    );
  }
  return { hasVersion, sections, tokens: tokens(text) };
}

export function findSection(document: ReturnType<typeof parse>, name: string) {
  const matches = document.sections.filter(
    (section) => (section.name ?? "Intro") === name,
  );
  if (matches.length > 1) throw new Error(`Ambiguous section: ${name}`);
  return matches[0];
}

export function update(
  file: string,
  name: string,
  replacement: string,
  expected?: string | null,
  append = false,
) {
  if (!name.trim() || name !== name.trim() || /[\r\n]|\s+#+$/.test(name))
    throw new Error("Use a single section heading.");
  if (parse(replacement).sections.length > 1)
    throw new Error("A section body cannot include another level-two heading.");
  const target = realpathSync(file);
  const lock = `${target}.agents-view.lock`;
  writeFileSync(lock, "", { flag: "wx", mode: 0o600 });
  try {
    const text = readFileSync(target, "utf8");
    const document = parse(text);
    const current = findSection(document, name);
    if (expected !== undefined && expected !== (current?.body ?? null))
      throw new Error(
        "The section changed on disk. Edit it again using the latest version.",
      );
    const eol = text.includes("\r\n") ? "\r\n" : "\n";
    const body = replacement
      .replace(/^(?:[ \t]*\r?\n)+|(?:\r?\n[ \t]*)+$/g, "")
      .replace(/\r?\n/g, eol);
    const nextBody = append && current?.body ? current.body + eol + body : body;
    let next: string;
    if (current?.body) {
      next =
        text.slice(0, current.bodyStart) +
        nextBody +
        text.slice(current.bodyEnd);
    } else if (current && nextBody) {
      const before = text.slice(0, current.start);
      const after = text.slice(current.start);
      const prefix =
        before && !before.endsWith("\n") ? eol + eol : before ? eol : "";
      next =
        before +
        prefix +
        nextBody +
        eol +
        (after || (current.end < text.length ? eol : ""));
    } else if (!current) {
      const gap =
        text.endsWith(eol + eol) || !text
          ? ""
          : text.endsWith(eol)
            ? eol
            : eol + eol;
      next = `${text}${gap}## ${name}${eol}${body ? eol + body + eol : ""}`;
    } else {
      return;
    }
    if (next === text) return;
    const after = parse(next);
    const names = document.sections.map((section) => section.name);
    if (!current) names.push(name);
    if (
      after.hasVersion !== document.hasVersion ||
      after.sections.length !== names.length ||
      after.sections.some((section, index) => section.name !== names[index])
    ) {
      throw new Error(
        "The edit changes the document structure. Check headings, comments, and code fences.",
      );
    }
    const temporary = resolve(
      dirname(target),
      `.agents-view-${randomUUID()}.tmp`,
    );
    try {
      writeFileSync(temporary, next, {
        flag: "wx",
        mode: statSync(target).mode & 0o777,
      });
      renameSync(temporary, target);
    } finally {
      if (existsSync(temporary)) unlinkSync(temporary);
    }
  } finally {
    unlinkSync(lock);
  }
}

export function overrides(document: ReturnType<typeof parse>) {
  const body = findSection(document, "Overrides")?.body ?? "";
  return body
    .split(/\r?\n/)
    .filter((line) => line.trim())
    .map((line) => {
      const match =
        /^- Global: "((?:[^"\\]|\\.)+)"\s+→ here:\s+(.+)$/.exec(line) ??
        /^- Global: "(.+?)"\s+→ here:\s+(.+)$/.exec(line);
      if (!match) return { line, quote: "", replacement: "" };
      let quote = match[1];
      try {
        quote = JSON.parse(`"${quote}"`);
      } catch {
        /* Accept literal quotation marks in handwritten lines. */
      }
      return { line, quote, replacement: match[2] };
    });
}
