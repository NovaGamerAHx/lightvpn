#!/usr/bin/env python3
"""Generate the icon set for LightVPN.

Produces (all checked into the repo so builds are reproducible and so the
//go:embed directives always find a file):

  build/appicon.png                 1024x1024 master icon (Wails derives icon.ico from it)
  build/windows/icon.ico            window/taskbar icon, 16..256 px
  build/windows/tray-idle.ico       tray icon, not connected
  build/windows/tray-connected.ico  tray icon, connected (green dot)

Usage:  python3 scripts/make_icons.py
"""
import os
from PIL import Image, ImageDraw, ImageFilter

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

TOP = (91, 140, 255)      # #5B8CFF
BOTTOM = (33, 212, 253)   # #21D4FD
BG_TOP = (23, 26, 34)
BG_BOT = (13, 14, 18)


def lerp(a, b, t):
    return tuple(int(round(a[i] + (b[i] - a[i]) * t)) for i in range(len(a)))


def vertical_gradient(size, c1, c2):
    img = Image.new("RGBA", (size, size))
    d = ImageDraw.Draw(img)
    for y in range(size):
        d.line([(0, y), (size, y)], fill=lerp(c1, c2, y / max(1, size - 1)) + (255,))
    return img


def rounded_mask(size, radius):
    m = Image.new("L", (size, size), 0)
    ImageDraw.Draw(m).rounded_rectangle([0, 0, size - 1, size - 1], radius=radius, fill=255)
    return m


def shield_points(size):
    s = size
    pts = [
        (0.500 * s, 0.135 * s),
        (0.790 * s, 0.250 * s),
        (0.790 * s, 0.545 * s),
        (0.500 * s, 0.865 * s),
        (0.210 * s, 0.545 * s),
        (0.210 * s, 0.250 * s),
    ]
    return pts


def draw_master(size=1024):
    ss = size * 2  # supersample for clean edges
    img = Image.new("RGBA", (ss, ss), (0, 0, 0, 0))
    bg = vertical_gradient(ss, BG_TOP, BG_BOT)
    img = Image.composite(bg, img, rounded_mask(ss, int(ss * 0.225)))
    d = ImageDraw.Draw(img)

    # soft halo behind the shield
    halo = Image.new("RGBA", (ss, ss), (0, 0, 0, 0))
    hd = ImageDraw.Draw(halo)
    hd.polygon(shield_points(ss * 1.06), fill=TOP + (60,), outline=None)
    halo = halo.filter(ImageFilter.GaussianBlur(ss // 90))
    img = Image.alpha_composite(img, halo)
    d = ImageDraw.Draw(img)

    # gradient-filled shield
    grad = vertical_gradient(ss, TOP, BOTTOM)
    smask = Image.new("L", (ss, ss), 0)
    ImageDraw.Draw(smask).polygon(shield_points(ss), fill=255)
    shield = Image.composite(grad, Image.new("RGBA", (ss, ss), (0, 0, 0, 0)), smask)
    img = Image.alpha_composite(img, shield)
    d = ImageDraw.Draw(img)

    # thin light rim on the shield
    d.polygon(shield_points(ss), outline=(255, 255, 255, 90))

    # keyhole: circle + stem, punched in the background colour
    cx, cy, r = ss * 0.5, ss * 0.425, ss * 0.082
    colour = lerp(BG_TOP, BG_BOT, 0.42) + (255,)
    d.ellipse([cx - r, cy - r, cx + r, cy + r], fill=colour)
    d.polygon(
        [
            (cx - r * 0.42, cy + r * 0.35),
            (cx + r * 0.42, cy + r * 0.35),
            (cx + r * 0.72, cy + r * 3.05),
            (cx - r * 0.72, cy + r * 3.05),
        ],
        fill=colour,
    )

    # two small motion ticks: the "traffic" going through the tunnel
    for i, (x0, y0, w) in enumerate([(0.085, 0.46, 0.055), (0.085, 0.56, 0.035)]):
        d.rounded_rectangle([ss * x0, ss * y0, ss * (x0 + w), ss * (y0 + 0.028)], radius=ss * 0.012,
                            fill=(255, 255, 255, 70 if i else 110))

    return img.resize((size, size), Image.LANCZOS)


def draw_tray(size=32, connected=False):
    ss = size * 8
    img = Image.new("RGBA", (ss, ss), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    line = (235, 240, 250, 255) if connected else (150, 156, 170, 255)
    pts = shield_points(ss)
    d.polygon(pts, outline=line)
    # thick outline: draw the polygon repeatedly with tiny offsets (cheap AA)
    for off in range(1, max(2, ss // 22)):
        d.polygon([(x + off, y) for x, y in pts], outline=line)
        d.polygon([(x - off, y) for x, y in pts], outline=line)
        d.polygon([(x, y + off) for x, y in pts], outline=line)
        d.polygon([(x, y - off) for x, y in pts], outline=line)
    key_r = ss * 0.085
    cx, cy = ss * 0.5, ss * 0.44
    d.ellipse([cx - key_r, cy - key_r, cx + key_r, cy + key_r], outline=line, width=max(2, ss // 20))
    if connected:
        r = ss * 0.14
        x, y = ss * 0.79, ss * 0.79
        d.ellipse([x - r, y - r, x + r, y + r], fill=(46, 220, 130, 255))
    return img.resize((size, size), Image.LANCZOS)


def save_ico(im, path, sizes):
    frames = []
    for s in sizes:
        f = im if im.size == (s, s) else im.resize((s, s), Image.LANCZOS)
        frames.append(f)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    frames[-1].save(path, format="ICO", sizes=[(s, s) for s in sizes])
    print(f"  wrote {os.path.relpath(path, ROOT)}  ({os.path.getsize(path)} bytes, sizes {sizes})")


def main():
    os.makedirs(os.path.join(ROOT, "build", "windows"), exist_ok=True)
    print("generating icons…")

    master = draw_master(1024)
    p = os.path.join(ROOT, "build", "appicon.png")
    master.save(p, format="PNG", optimize=True)
    print(f"  wrote build/appicon.png ({os.path.getsize(p)} bytes)")

    save_ico(master, os.path.join(ROOT, "build", "windows", "icon.ico"), [16, 24, 32, 48, 64, 128, 256])
    for name, connected in (("tray-idle.ico", False), ("tray-connected.ico", True)):
        t = draw_tray(64, connected=connected)
        save_ico(t, os.path.join(ROOT, "build", "windows", name), [16, 24, 32, 48])


if __name__ == "__main__":
    main()
