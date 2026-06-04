// Curated Prism (refractor) setup. react-diff-view requires refractor@3.x.
// We register only the languages we care about to keep the bundle small.
import refractor from "refractor/core";
import markup from "refractor/lang/markup";
import clike from "refractor/lang/clike";
import javascript from "refractor/lang/javascript";
import json from "refractor/lang/json";
import css from "refractor/lang/css";
import bash from "refractor/lang/bash";
import yaml from "refractor/lang/yaml";
import python from "refractor/lang/python";
import rust from "refractor/lang/rust";
import go from "refractor/lang/go";
import markdown from "refractor/lang/markdown";
import jsx from "refractor/lang/jsx";
import typescript from "refractor/lang/typescript";
import tsx from "refractor/lang/tsx";

// Register in dependency order (base grammars first).
[markup, clike, javascript, json, css, bash, yaml, python, rust, go, markdown, jsx, typescript, tsx].forEach(
  (lang) => refractor.register(lang as any),
);

export default refractor;

const EXT_TO_LANG: Record<string, string> = {
  ts: "typescript",
  tsx: "tsx",
  js: "javascript",
  jsx: "jsx",
  mjs: "javascript",
  cjs: "javascript",
  py: "python",
  rs: "rust",
  go: "go",
  json: "json",
  css: "css",
  scss: "css",
  html: "markup",
  xml: "markup",
  svg: "markup",
  sh: "bash",
  bash: "bash",
  zsh: "bash",
  yml: "yaml",
  yaml: "yaml",
  md: "markdown",
  markdown: "markdown",
};

/** Prism language id for a file path, or undefined if we don't highlight it. */
export function languageForPath(path: string | undefined | null): string | undefined {
  if (!path) return undefined;
  const ext = path.split(".").pop()?.toLowerCase();
  return ext ? EXT_TO_LANG[ext] : undefined;
}
