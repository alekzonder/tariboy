#!/usr/bin/env node
// Annotate non-token custom properties in the staged Tailwind output with
// `/* @kind other */`.
//
// Why: the design-system check scans every custom property reachable from
// styles.css. Tariboy ships no static stylesheet, so ALL of its CSS - theme
// tokens and compiled Tailwind utility output alike - lands in one file
// (_ds_bundle.css). The scan therefore sees, alongside the real theme tokens
// under :root/.dark, several hundred declarations that are not design tokens
// at all:
//
//   --tw-*            Tailwind v4 internals (`*,:before,:after,::backdrop`,
//                     utility classes, and 73 `@property` rules)
//   --color-*, --font-*, --size-*   flexlayout-react's light.css theme, all
//                     declared on `.flexlayout__layout` under generic names
//                     that collide with real token names
//   --splitter-*      terminalWorkspace.css
//   --card-spacing    a Tailwind arbitrary-property utility
//
// The rule applied here is structural, not a name allowlist: a custom
// property declared inside a component or utility selector is not a design
// token. Only declarations whose innermost scope is a theme scope (:root,
// :host, html, .dark, or a comma list of those) stay unannotated.
//
// Nothing is deleted - every declaration still ships and still renders;
// `--tw-*` in particular is load-bearing for the compiled utilities. CSS
// comments are inert, so this is a pure metadata pass.
//
// Idempotent: re-running over annotated CSS is a no-op.

import { readFileSync, writeFileSync } from 'node:fs';

const MARKER = '/* @kind other */';
const THEME_PART = /^(:root|:host|html|\.dark)$/;

// A scope is a theme scope only if EVERY comma-separated part is one.
// `:root,:host` qualifies; `.dark\:scale-0:is(.dark *)` does not.
const isThemeScope = (sel) => {
  const s = sel.replace(/\/\*[\s\S]*?\*\//g, '').trim();
  if (!s || s.startsWith('@')) return false;
  return s.split(',').every((p) => THEME_PART.test(p.trim()));
};

const isCustomProp = (decl) => /^\s*--[\w-]+\s*:/.test(decl.replace(/\/\*[\s\S]*?\*\//g, ''));

function annotate(css) {
  // Idempotence by reconstruction: drop every marker this script has ever
  // written, then re-derive them. Checking "is it already annotated?" during
  // the walk is positionally fragile - a block's first declaration sees no
  // preceding marker in its own text and would be marked twice.
  css = css.split(MARKER).join('');
  let out = '';
  let cur = ''; // text since the last {, } or ; at paren depth 0
  const stack = []; // innermost-last selector preludes
  let paren = 0;
  let counts = { decls: 0, atProperty: 0, skipped: 0 };

  const scope = () => (stack.length ? stack[stack.length - 1] : '');

  // Flush a declaration, appending the marker when it is a custom property
  // in a non-theme scope and isn't already annotated.
  const flushDecl = (terminator) => {
    let text = cur;
    cur = '';
    if (!isCustomProp(text)) return (out += text + terminator);
    if (isThemeScope(scope())) {
      counts.skipped++;
      return (out += text + terminator);
    }
    if (text.includes('@kind')) return (out += text + terminator);
    counts.decls++;
    // Always terminate with `;` so the marker never fuses with the value -
    // a block's last declaration may arrive with `}` as its terminator.
    out += `${text};${MARKER}${terminator === ';' ? '' : terminator}`;
  };

  for (let i = 0; i < css.length; i++) {
    const c = css[i];

    // CSS escapes come first: Tailwind's generated class names escape the
    // punctuation they embed, so selectors carry literal `\'`, `\(`, `\)` and
    // `\/` (e.g. `.\[\&_svg\:not\(\[class\*\=\'size-\'\]\)\]\:size-3`, or
    // `.top-1\/2`). Read as delimiters those would open a phantom string,
    // unbalance the paren count, or start a phantom comment.
    if (c === '\\' && i + 1 < css.length) {
      cur += css.slice(i, i + 2);
      i++;
      continue;
    }

    // Comments and strings pass through verbatim.
    if (c === '/' && css[i + 1] === '*') {
      const end = css.indexOf('*/', i + 2);
      const stop = end === -1 ? css.length : end + 2;
      cur += css.slice(i, stop);
      i = stop - 1;
      continue;
    }
    if (c === '"' || c === "'") {
      let j = i + 1;
      while (j < css.length && css[j] !== c) j += css[j] === '\\' ? 2 : 1;
      cur += css.slice(i, j + 1);
      i = j;
      continue;
    }

    if (c === '(') paren++;
    else if (c === ')') paren = Math.max(0, paren - 1);

    // `;` inside url(data:...;base64,...) is not a declaration terminator.
    if (paren > 0) {
      cur += c;
      continue;
    }

    if (c === '{') {
      const prelude = cur.trim();
      out += cur + '{';
      cur = '';
      stack.push(prelude);
      // `@property --tw-foo { ... }` declares the name in the prelude, not as
      // a declaration - mark the block itself.
      const m = prelude.match(/^@property\s+(--[\w-]+)/);
      if (m && !THEME_NAMES.has(m[1])) {
        out += MARKER;
        counts.atProperty++;
      }
      continue;
    }

    if (c === '}') {
      flushDecl('');
      out += '}';
      stack.pop();
      continue;
    }

    if (c === ';') {
      flushDecl(';');
      continue;
    }

    cur += c;
  }
  out += cur;
  return { css: out, counts };
}

// Pass 1: collect the names declared in theme scopes, so an `@property` rule
// backing a real token is never demoted.
const THEME_NAMES = new Set();
function collectThemeNames(css) {
  const re = /([^{}]*)\{([^{}]*)\}/g;
  let m;
  while ((m = re.exec(css))) {
    const sel = m[1].split(/[{}]/).pop();
    if (!isThemeScope(sel)) continue;
    for (const d of m[2].matchAll(/(--[\w-]+)\s*:/g)) THEME_NAMES.add(d[1]);
  }
}

// Annotate a stylesheet in place. Safe to call on a file that has already
// been annotated, and on one with no custom properties at all.
export function annotateFile(file) {
  const src = readFileSync(file, 'utf8');
  if (!/--[\w-]+\s*:/.test(src)) return null;
  THEME_NAMES.clear();
  collectThemeNames(src);
  const { css, counts } = annotate(src);
  writeFileSync(file, css);
  console.error(
    `  @kind other: ${counts.decls} declaration(s) + ${counts.atProperty} @property rule(s) marked; ` +
      `${counts.skipped} theme-scope token(s) left as tokens (${THEME_NAMES.size} distinct)`,
  );
  return counts;
}

// CLI: annotate-tokens.mjs <styles.css>
if (process.argv[1] && process.argv[1].endsWith('annotate-tokens.mjs')) {
  const file = process.argv[2];
  if (!file) {
    console.error('usage: annotate-tokens.mjs <styles.css>');
    process.exit(1);
  }
  annotateFile(file);
}
