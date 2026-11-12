// Generated stand-in art for the mockup: covers and reader pages as SVG
// data URIs. Abstract shapes + halftone only — never imitates a real work.

const cache = new Map<string, string>();

function rng(seed: number) {
  let s = (seed * 2654435761) >>> 0;
  return () => {
    s = (s * 1664525 + 1013904223) >>> 0;
    return s / 0x100000000;
  };
}

const esc = (t: string) => t.replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]!);
const uri = (svg: string) => `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;

/** 2:3 cover. `label` is the volume marker ("1", "#3"); omit for series art. */
export function coverArt(seed: number, title: string, label?: string): string {
  const key = `c|${seed}|${title}|${label ?? ""}`;
  const hit = cache.get(key);
  if (hit) return hit;
  const r = rng(seed);
  const hue = Math.floor(r() * 360);
  const hue2 = (hue + 140 + Math.floor(r() * 80)) % 360;
  const bg = `oklch(${0.42 + r() * 0.35} ${0.06 + r() * 0.1} ${hue})`;
  const ink = `oklch(0.18 0.02 ${hue})`;
  const pop = `oklch(${0.72 + r() * 0.15} ${0.12 + r() * 0.08} ${hue2})`;
  const paper = `oklch(0.96 0.01 ${hue})`;
  const cx = 60 + r() * 180, cy = 80 + r() * 200, rad = 40 + r() * 90;
  const tilt = -30 + r() * 60;
  const vertical = /[\u3040-\u30ff\u4e00-\u9fff]/.test(title); // kana and kanji: set vertically
  const chars = [...title].slice(0, 10);
  const titleSvg = vertical
    ? chars
        .map((ch, i) => `<text x="262" y="${48 + i * 30}" font-size="26" text-anchor="middle">${esc(ch)}</text>`)
        .join("")
    : `<text x="22" y="56" font-size="28" font-weight="700">${esc(title.slice(0, 18))}</text>`;
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 300 450">
<defs><pattern id="h" width="8" height="8" patternUnits="userSpaceOnUse" patternTransform="rotate(45)"><circle cx="4" cy="4" r="1.6" fill="${ink}" opacity=".35"/></pattern></defs>
<rect width="300" height="450" fill="${bg}"/>
<rect width="300" height="450" fill="url(#h)"/>
<circle cx="${cx}" cy="${cy}" r="${rad}" fill="${pop}"/>
<g transform="rotate(${tilt} 150 300)"><rect x="-60" y="${250 + r() * 60}" width="420" height="${40 + r() * 50}" fill="${ink}" opacity=".85"/></g>
<path d="M0 ${380 + r() * 30} Q 150 ${330 + r() * 60} 300 ${370 + r() * 40} V450 H0Z" fill="${paper}" opacity=".9"/>
<g fill="${paper}" font-family="'Hiragino Mincho ProN','Yu Mincho',serif" font-weight="700" stroke="${ink}" stroke-width="5" paint-order="stroke">${titleSvg}</g>
${label ? `<g><rect x="18" y="396" width="${18 + label.length * 13}" height="34" rx="4" fill="${ink}"/><text x="${27 + (label.length * 13) / 2 - 4}" y="420" font-size="20" font-family="Inter,sans-serif" font-weight="700" fill="${paper}" text-anchor="middle">${esc(label)}</text></g>` : ""}
</svg>`;
  const out = uri(svg);
  cache.set(key, out);
  return out;
}

/** A manga page: panel grid with gutters, halftone fills and a page number. */
export function pageArt(seed: number, page: number, opts: { webtoon?: boolean; color?: boolean } = {}): string {
  const key = `p|${seed}|${page}|${opts.webtoon ? 1 : 0}`;
  const hit = cache.get(key);
  if (hit) return hit;
  const r = rng(seed * 131 + page);
  const W = opts.webtoon ? 800 : 1000;
  const H = opts.webtoon ? 1280 : 1500;
  const m = 60, g = 22;
  const hue = (seed * 37) % 360;
  const paper = opts.webtoon ? `oklch(0.97 0.01 ${hue})` : "#fbfaf6";
  const ink = "#1b1a19";
  // Split the live area into rows, each row into 1–3 panels.
  const rows = opts.webtoon ? 2 : 2 + Math.floor(r() * 3);
  let y = m;
  const rowH = (H - m * 2 - g * (rows - 1)) / rows;
  let panels = "";
  for (let i = 0; i < rows; i++) {
    const cols = 1 + Math.floor(r() * 3);
    let x = m;
    const colW = (W - m * 2 - g * (cols - 1)) / cols;
    for (let j = 0; j < cols; j++) {
      const fill = opts.webtoon || opts.color
        ? `oklch(${0.6 + r() * 0.3} ${0.05 + r() * 0.1} ${(hue + r() * 90) % 360})`
        : r() > 0.5 ? "url(#dots)" : paper;
      const bx = x + colW * (0.2 + r() * 0.5), by = y + rowH * (0.3 + r() * 0.4);
      panels += `<rect x="${x}" y="${y}" width="${colW}" height="${rowH}" fill="${fill}" stroke="${ink}" stroke-width="5"/>`;
      panels += `<circle cx="${bx}" cy="${by}" r="${Math.min(colW, rowH) * (0.12 + r() * 0.15)}" fill="${ink}" opacity=".8"/>`;
      if (r() > 0.45)
        panels += `<ellipse cx="${x + colW * 0.7}" cy="${y + rowH * 0.25}" rx="${colW * 0.18}" ry="${rowH * 0.14}" fill="#fff" stroke="${ink}" stroke-width="4"/>`;
      x += colW + g;
    }
    y += rowH + g;
  }
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${W} ${H}" width="${W}" height="${H}">
<defs><pattern id="dots" width="12" height="12" patternUnits="userSpaceOnUse" patternTransform="rotate(30)"><rect width="12" height="12" fill="${paper}"/><circle cx="6" cy="6" r="2.4" fill="${ink}" opacity=".5"/></pattern></defs>
<rect width="${W}" height="${H}" fill="${paper}"/>${panels}
<text x="${W / 2}" y="${H - 22}" font-size="22" font-family="Inter,sans-serif" fill="#8a877f" text-anchor="middle">${page}</text>
</svg>`;
  const out = uri(svg);
  cache.set(key, out);
  return out;
}
