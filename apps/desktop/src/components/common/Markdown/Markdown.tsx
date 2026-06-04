import { useState, type ReactNode, Fragment } from 'react';
import { jsx, jsxs } from 'react/jsx-runtime';
import { open as shellOpen } from '@tauri-apps/plugin-shell';
import ReactMarkdown, { type Components } from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { toJsxRuntime } from 'hast-util-to-jsx-runtime';
import refractor from 'refractor/core';

type JsxRuntimeTree = Parameters<typeof toJsxRuntime>[0];
import { Copy, Check } from 'lucide-react';
import MermaidDiagram from '../../agent/PlanCard/MermaidDiagram';

// Register a curated set of common languages on refractor's core (keeps the
// bundle small vs. the full `refractor` which loads every Prism grammar).
import js from 'refractor/lang/javascript.js';
import ts from 'refractor/lang/typescript.js';
import jsx_ from 'refractor/lang/jsx.js';
import tsx from 'refractor/lang/tsx.js';
import python from 'refractor/lang/python.js';
import rust from 'refractor/lang/rust.js';
import bash from 'refractor/lang/bash.js';
import json from 'refractor/lang/json.js';
import css from 'refractor/lang/css.js';
import markup from 'refractor/lang/markup.js';
import go from 'refractor/lang/go.js';
import java from 'refractor/lang/java.js';
import c from 'refractor/lang/c.js';
import cpp from 'refractor/lang/cpp.js';
import sql from 'refractor/lang/sql.js';
import yaml from 'refractor/lang/yaml.js';
import toml from 'refractor/lang/toml.js';
import markdownLang from 'refractor/lang/markdown.js';
import diff from 'refractor/lang/diff.js';

for (const lang of [
  js, ts, jsx_, tsx, python, rust, bash, json, css, markup, go, java, c, cpp, sql, yaml, toml,
  markdownLang, diff,
]) {
  refractor.register(lang);
}

// Common aliases → registered grammar names.
const LANG_ALIAS: Record<string, string> = {
  js: 'javascript',
  ts: 'typescript',
  py: 'python',
  rs: 'rust',
  sh: 'bash',
  shell: 'bash',
  zsh: 'bash',
  yml: 'yaml',
  html: 'markup',
  xml: 'markup',
  'c++': 'cpp',
  md: 'markdown',
};

function highlight(code: string, lang?: string): ReactNode {
  const name = lang ? (LANG_ALIAS[lang] ?? lang) : undefined;
  if (name && refractor.registered(name)) {
    const tree = refractor.highlight(code, name) as unknown as JsxRuntimeTree;
    return toJsxRuntime(tree, { Fragment, jsx, jsxs });
  }
  return code;
}

function CodeBlock({ code, lang }: { code: string; lang?: string }) {
  const [copied, setCopied] = useState(false);
  const onCopy = () => {
    navigator.clipboard.writeText(code).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  };
  return (
    <>
      <button type="button" className="code-copy-btn" onClick={onCopy} aria-label="Copy code">
        {copied ? <Check size={13} /> : <Copy size={13} />}
      </button>
      <code className={lang ? `language-${lang}` : undefined}>{highlight(code, lang)}</code>
    </>
  );
}

export interface MarkdownProps {
  children: string;
  className?: string;
  enableMermaid?: boolean;
}

/**
 * Shared markdown renderer: GFM, refractor syntax highlighting + copy button,
 * mermaid code blocks, and Tauri-shell links. Replaces the per-surface
 * ad-hoc ReactMarkdown usages.
 */
export default function Markdown({ children, className = 'message-html', enableMermaid = true }: MarkdownProps) {
  const components: Components = {
    a: ({ href, children }) => (
      <a
        href={href}
        className="text-blue-600 dark:text-blue-400 underline hover:text-blue-800 dark:hover:text-blue-300"
        onClick={(e) => {
          e.preventDefault();
          if (href) shellOpen(href).catch(() => window.open(href, '_blank'));
        }}
      >
        {children}
      </a>
    ),
    code: ({ className: cls, children: codeChildren, ...props }) => {
      const text = Array.isArray(codeChildren)
        ? codeChildren.map((c) => (typeof c === 'string' ? c : String(c ?? ''))).join('')
        : String(codeChildren ?? '');
      const lang = /language-(\w[\w+-]*)/.exec(cls || '')?.[1];
      if (lang === 'mermaid' && enableMermaid) {
        return <MermaidDiagram chart={text.replace(/\n$/, '')} />;
      }
      const isBlock = !!lang || text.includes('\n');
      if (!isBlock) {
        return <code className={cls} {...props}>{codeChildren}</code>;
      }
      return <CodeBlock code={text.replace(/\n$/, '')} lang={lang} />;
    },
  };

  return (
    <div className={className}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {children}
      </ReactMarkdown>
    </div>
  );
}
