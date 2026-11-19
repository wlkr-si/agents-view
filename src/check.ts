import { existsSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { overrides, pageEntries, parse } from "./document.js";
import { discover } from "./sources.js";

export function check(file: string, text: string) {
  const document = parse(text);
  const errors: string[] = [];
  const warnings: string[] = [];
  if (!document.hasVersion)
    errors.push("Missing first-line <!-- agents-view v0 --> tag.");
  if (document.tokens > 1000)
    warnings.push(
      `Core exceeds the 1,000-token budget (~${document.tokens} tokens).`,
    );

  const names = new Set(["Intro"]);
  for (const section of document.sections) {
    if (section.name === null) continue;
    if (names.has(section.name))
      errors.push(`Duplicate or reserved section name: ${section.name}`);
    names.add(section.name);
    if (section.type !== "pages" && section.name !== "Pages") continue;
    const entries = pageEntries(section.body);
    if (!entries) {
      errors.push(`Invalid page list in ${section.name}.`);
      continue;
    }
    for (const { href, note } of entries) {
      if (section.name === "Pages" && !/^read when\s+\S/i.test(note ?? ""))
        errors.push(`Missing read-when note: ${href}`);
      if (/^(?:[\w+.-]+:|\/)/.test(href)) {
        errors.push(`Use a relative Pages link: ${href}`);
        continue;
      }
      try {
        const path = decodeURIComponent(href.split(/[?#]/, 1)[0]);
        if (!existsSync(path ? resolve(dirname(file), path) : file))
          errors.push(`Broken Pages link: ${href}`);
      } catch {
        errors.push(`Invalid Pages link: ${href}`);
      }
    }
  }

  if (
    document.sections.filter((section) => section.name === "Overrides")
      .length === 1
  ) {
    const entries = overrides(document);
    const globals = entries.length
      ? discover(dirname(file)).flatMap((group) =>
          group.files.filter((file) => file.origin === "Global"),
        )
      : [];
    for (const entry of entries) {
      if (!entry.quote) errors.push(`Invalid override: ${entry.line}`);
      else if (!globals.some((file) => file.text.includes(entry.quote)))
        errors.push(`STALE override: ${entry.quote}`);
    }
  }
  return { tokens: document.tokens, errors, warnings };
}
