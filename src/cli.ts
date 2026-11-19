#!/usr/bin/env node
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { check } from "./check.js";
import { findSection, parse, tokens, update } from "./document.js";
import { serve } from "./server.js";
import { discover } from "./sources.js";

const usage = `agents-view sections
agents-view read [section]
agents-view write <section> < body.md
agents-view check
agents-view sources [--agent claude|codex]
agents-view view [--no-open]`;

async function main([command, ...args]: string[]) {
  const cwd = process.cwd();
  const file = resolve(cwd, "AGENTS.md");
  const read = () => readFileSync(file, "utf8");
  switch (command) {
    case undefined:
    case "--help":
    case "-h":
      console.log(usage);
      return;
    case "sections":
      if (args.length) break;
      for (const section of parse(read()).sections)
        console.log(
          `${section.name ?? "Intro"}\t${section.type}\t~${section.tokens} tokens`,
        );
      return;
    case "read": {
      if (args.length > 1) break;
      const text = read();
      if (!args.length) process.stdout.write(text);
      else {
        const section = findSection(parse(text), args[0]);
        if (!section) throw new Error(`Unknown section: ${args[0]}`);
        process.stdout.write(
          text.slice(section.start - section.heading.length, section.end),
        );
      }
      return;
    }
    case "write":
      if (args.length !== 1) break;
      update(file, args[0], readFileSync(0, "utf8"));
      return;
    case "check": {
      if (args.length) break;
      const result = check(file, read());
      for (const warning of result.warnings)
        console.warn(`Warning: ${warning}`);
      for (const error of result.errors) console.error(`Error: ${error}`);
      if (result.errors.length) process.exitCode = 1;
      else
        console.log(
          `OK · ~${result.tokens} / 1,000 tokens · Pages and Overrides valid`,
        );
      return;
    }
    case "sources":
      if (args.length && (args.length !== 2 || args[0] !== "--agent")) break;
      for (const group of discover(cwd, args[1])) {
        console.log(group.agent);
        for (const file of group.files)
          console.log(`${file.path}\t~${tokens(file.text)} tokens`);
      }
      return;
    case "view":
      if (args.length && (args.length !== 1 || args[0] !== "--no-open")) break;
      read();
      await serve(cwd, file, !args.length);
      return;
  }
  throw new Error(usage);
}

main(process.argv.slice(2)).catch((error) => {
  console.error(error instanceof Error ? error.message : error);
  process.exitCode = 1;
});
