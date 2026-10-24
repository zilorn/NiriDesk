#!/usr/bin/env python3
"""Generate NiriDesk PNG icons without third-party design tools."""
from pathlib import Path
from PIL import Image, ImageDraw, ImageFilter

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "web" / "public" / "icons"
SCALE = 4


def color_at(t):
    top = (12, 24, 18)
    bottom = (5, 9, 7)
    return tuple(round(top[i] + (bottom[i] - top[i]) * t) for i in range(3))


def draw_mark(size, margin_ratio):
    canvas = size * SCALE
    image = Image.new("RGBA", (canvas, canvas), (0, 0, 0, 0))
    pixels = image.load()
    for y in range(canvas):
        base = color_at(y / max(1, canvas - 1))
        for x in range(canvas):
            glow = max(0.0, 1.0 - (((x - 0.5) ** 2 + (y - 0.36) ** 2) ** 0.5) * 1.65)
            pixels[x, y] = (
                min(255, round(base[0] + glow * 16)),
                min(255, round(base[1] + glow * 32)),
                min(255, round(base[2] + glow * 22)),
                255,
            )

    glow_layer = Image.new("RGBA", (canvas, canvas), (0, 0, 0, 0))
    glow_draw = ImageDraw.Draw(glow_layer)
    m = int(canvas * margin_ratio)
    inner = canvas - 2 * m
    stroke = max(8, int(inner * 0.086))
    pts = [
        (m + int(inner * 0.16), m + int(inner * 0.22)),
        (m + int(inner * 0.16), m + int(inner * 0.78)),
    ]
    mid = [
        (m + int(inner * 0.16), m + int(inner * 0.54)),
        (m + int(inner * 0.55), m + int(inner * 0.22)),
        (m + int(inner * 0.55), m + int(inner * 0.78)),
    ]
    right = [
        (m + int(inner * 0.84), m + int(inner * 0.22)),
        (m + int(inner * 0.84), m + int(inner * 0.78)),
    ]
    for line in (pts, mid, right):
        glow_draw.line(line, fill=(127, 255, 190, 170), width=stroke, joint="curve")
    glow_draw.ellipse(
        (m + int(inner * 0.84) - stroke // 2, m + int(inner * 0.86) - stroke // 2,
         m + int(inner * 0.84) + stroke // 2, m + int(inner * 0.86) + stroke // 2),
        fill=(127, 255, 190, 170),
    )
    glow_layer = glow_layer.filter(ImageFilter.GaussianBlur(stroke * 1.5))
    image.alpha_composite(glow_layer)

    draw = ImageDraw.Draw(image)
    for line in (pts, mid, right):
        draw.line(line, fill=(198, 255, 220, 255), width=stroke, joint="curve")
        draw.line(line, fill=(151, 242, 191, 255), width=max(2, stroke // 3), joint="curve")
    r = stroke // 2
    cx, cy = m + int(inner * 0.84), m + int(inner * 0.86)
    draw.ellipse((cx - r, cy - r, cx + r, cy + r), fill=(151, 242, 191, 255))

    return image.resize((size, size), Image.Resampling.LANCZOS)


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    for size, margin, name in [
        (512, 0.23, "icon-512.png"),
        (192, 0.23, "icon-192.png"),
        (180, 0.23, "icon-180.png"),
    ]:
        draw_mark(size, margin).save(OUT / name, optimize=True)
    draw_mark(512, 0.30).save(OUT / "icon-maskable-512.png", optimize=True)


if __name__ == "__main__":
    main()
