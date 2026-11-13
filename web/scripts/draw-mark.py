"""Draw the MARS mark (a red planet on a night tile, a dotted orbit, two moons;
the near moon is the blue "you are here") as favicon.svg and the PWA PNGs.
Geometry matches web/src/components/brand/app-mark.tsx.
Run from web/: python scripts/draw-mark.py public  (needs Pillow)."""
import math
import sys
from PIL import Image, ImageDraw

OUT = sys.argv[1]
TILE, PLANET, DUST, HERE = "#1f1210", "#e8542f", "#f3c9bd", "#5db2f7"
RX, RY, ROT, N, DOT = 13.0, 5.0, -20.0, 40, 0.5
PR = 7.0
NEAR, FAR = 40.0, 200.0  # moon angles on the orbit (deg)


def on_orbit(deg):
    t = math.radians(deg)
    x, y = RX * math.cos(t), RY * math.sin(t)
    r = math.radians(ROT)
    return 16 + x * math.cos(r) - y * math.sin(r), 16 + x * math.sin(r) + y * math.cos(r), math.sin(t) > 0


moons = [on_orbit(NEAR)[:2], on_orbit(FAR)[:2]]
dots = []  # (x, y, front)
for i in range(N):
    x, y, front = on_orbit(360 * i / N)
    if not front and math.hypot(x - 16, y - 16) < PR + 0.9:
        continue
    if any(math.hypot(x - mx, y - my) < 2.6 for mx, my in moons):
        continue
    dots.append((x, y, front))


def layers():
    """Shapes back to front: (kind, x, y, r, colour, opacity)."""
    out = [("dot", x, y, DOT, DUST, 0.5) for x, y, f in dots if not f]
    out.append(("disc", 16, 16, PR, PLANET, 1))
    out += [("dot", x, y, DOT, DUST, 0.85) for x, y, f in dots if f]
    fx, fy = moons[1]
    out.append(("disc", fx, fy, 1.2, DUST, 1))
    nx, ny = moons[0]
    out.append(("disc", nx, ny, 2.1, HERE, 1))
    return out


# favicon.svg
parts = [f'<rect width="32" height="32" rx="8" fill="{TILE}"/>']
for _, x, y, r, c, o in layers():
    op = "" if o == 1 else f' fill-opacity="{o}"'
    parts.append(f'<circle cx="{x:.2f}" cy="{y:.2f}" r="{r}" fill="{c}"{op}/>')
open(OUT + "/favicon.svg", "w", encoding="utf-8", newline="\n").write(
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">' + "".join(parts) + "</svg>\n")


def hexrgb(h, a=1.0):
    h = h.lstrip("#")
    return tuple(int(h[i:i + 2], 16) for i in (0, 2, 4)) + (round(255 * a),)


def png(size, name, bleed, scale):
    S = 8
    W = size * S
    im = Image.new("RGBA", (W, W), (0, 0, 0, 0))
    d = ImageDraw.Draw(im)
    if bleed:
        d.rectangle([0, 0, W, W], fill=hexrgb(TILE))
    else:
        d.rounded_rectangle([0, 0, W - 1, W - 1], radius=W * 8 / 32, fill=hexrgb(TILE))
    u = W / 32 * scale
    off = (W - 32 * u) / 2
    for _, x, y, r, c, o in layers():
        layer = Image.new("RGBA", (W, W), (0, 0, 0, 0))
        ImageDraw.Draw(layer).ellipse([off + (x - r) * u, off + (y - r) * u, off + (x + r) * u, off + (y + r) * u], fill=hexrgb(c, o))
        im = Image.alpha_composite(im, layer)
    im.resize((size, size), Image.LANCZOS).save(OUT + "/" + name, optimize=True)


png(192, "icon-192.png", False, 1)
png(512, "icon-512.png", False, 1)
png(512, "icon-maskable-512.png", True, 0.72)  # inside the 80% safe zone
png(180, "apple-touch-icon.png", True, 0.86)
print("dots", len(dots), "moons", [(round(x, 2), round(y, 2)) for x, y in moons])
