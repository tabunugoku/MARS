// Fails when Japanese text is left outside a defineMessages `ja:` block in
// web/src (see docs/i18n.md). The files are parsed with the TypeScript
// compiler (the tsconfig.app.json program), so apostrophes in JSX text, URLs
// and comments are handled as the compiler sees them. The invented mock data,
// the mock client and the i18n module itself are skipped.
import { API } from "typescript/unstable/sync";
import { SyntaxKind } from "typescript/unstable/ast";
import { join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const webDir = fileURLToPath(new URL("../", import.meta.url));
const root = resolve(webDir, "src");
const SKIP = ["mock" + sep, join("api", "mock.ts"), "i18n" + sep];
// Language names are shown in their own language whatever the UI language.
const ALLOWED = ["日本語"];
const JP = /[぀-ヿ㐀-鿿！-｠]/;

// Nodes whose text is user-visible: string literals, template pieces, JSX text.
const TEXT_KINDS = new Set([
  SyntaxKind.StringLiteral,
  SyntaxKind.NoSubstitutionTemplateLiteral,
  SyntaxKind.TemplateHead,
  SyntaxKind.TemplateMiddle,
  SyntaxKind.TemplateTail,
  SyntaxKind.JsxText,
]);

const nameText = (n) => n?.text ?? n?.escapedText;

function isDefineMessagesCall(node) {
  return (
    node.kind === SyntaxKind.CallExpression &&
    node.expression.kind === SyntaxKind.Identifier &&
    nameText(node.expression) === "defineMessages"
  );
}

// Returns the source lines that hold Japanese in one file, as
// [1-based line, source text] pairs.
function japaneseLines(sf) {
  const literals = [];
  const jaRanges = []; // [pos, end] of each `ja: { ... }` value

  const walk = (node) => {
    if (TEXT_KINDS.has(node.kind)) literals.push(node);
    if (isDefineMessagesCall(node)) {
      const messages = node.arguments[0];
      if (messages?.kind === SyntaxKind.ObjectLiteralExpression) {
        for (const prop of messages.properties) {
          if (
            prop.kind === SyntaxKind.PropertyAssignment &&
            nameText(prop.name) === "ja" &&
            prop.initializer.kind === SyntaxKind.ObjectLiteralExpression
          ) {
            jaRanges.push([prop.initializer.pos, prop.initializer.end]);
          }
        }
      }
    }
    node.forEachChild(walk);
  };
  sf.forEachChild(walk);

  const srcLines = sf.text.split("\n");
  const found = new Map(); // 0-based line -> source text
  for (const lit of literals) {
    if (jaRanges.some(([a, b]) => lit.pos >= a && lit.end <= b)) continue;
    const start = lit.getStart(sf);
    const firstLine = sf.getLineAndCharacterOfPosition(start).line;
    const pieces = sf.text.slice(start, lit.end).split("\n");
    pieces.forEach((piece, k) => {
      let chunk = piece;
      for (const a of ALLOWED) chunk = chunk.split(a).join("");
      if (JP.test(chunk)) found.set(firstLine + k, srcLines[firstLine + k]);
    });
  }
  return [...found.entries()].sort((x, y) => x[0] - y[0]).map(([line, text]) => [line + 1, text]);
}

let bad = 0;
const report = [];
const api = new API({ cwd: webDir });
try {
  const snapshot = api.updateSnapshot({ openProjects: [join(webDir, "tsconfig.app.json")] });
  const project = snapshot.getProjects()[0];
  for (const name of project.program.getSourceFileNames()) {
    const p = resolve(name);
    if (!p.startsWith(root + sep) || !/\.(ts|tsx)$/.test(p) || p.endsWith(".d.ts")) continue;
    const rel = relative(root, p);
    if (SKIP.some((s) => rel.startsWith(s) || rel === s)) continue;
    const sf = project.program.getSourceFile(p);
    if (!sf) continue;
    for (const [line, text] of japaneseLines(sf)) {
      bad++;
      report.push(`src/${rel.split(sep).join("/")}:${line}: ${text.trim().slice(0, 100)}`);
    }
  }
  snapshot.dispose();
} finally {
  api.close();
}

for (const line of report) console.log(line);
if (bad) {
  console.log(`\n${bad} line(s) with Japanese outside a defineMessages ja: block. See docs/i18n.md.`);
  process.exit(1);
}
console.log("i18n:check OK");
