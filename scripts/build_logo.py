#!/usr/bin/env python3
"""Build the Soul App icon from the transparent logo artwork.

Loads the transparent source PNG, trims the transparent margin, re-centers the
emblem on a square canvas with even padding and writes the embedded icon.
Run from the repository root:

    python3 scripts/build_logo.py

The source defaults to assets/soul-logo.png and the output is written to
internal/ui/assets/soul.png (embedded by internal/ui/resources.go).
"""

from __future__ import annotations

import argparse
import os

from PIL import Image

ICON_SIZE = 512
# Fraction of the canvas kept as padding on each side (8% => emblem fills 84%).
PADDING_RATIO = 0.08

DEFAULT_SOURCE = os.path.join("assets", "soul-logo.png")
DEFAULT_OUTPUT = os.path.join("internal", "ui", "assets", "soul.png")


def build(source_path: str, output_path: str) -> None:
    img = Image.open(source_path).convert("RGBA")

    bbox = img.getchannel("A").getbbox()
    if bbox is None:
        raise SystemExit(f"source {source_path} has no visible pixels")
    emblem = img.crop(bbox)

    max_side = int(ICON_SIZE * (1 - 2 * PADDING_RATIO))
    scale = min(max_side / emblem.width, max_side / emblem.height, 1.0)
    new_size = (max(1, round(emblem.width * scale)), max(1, round(emblem.height * scale)))
    emblem = emblem.resize(new_size, Image.LANCZOS)

    canvas = Image.new("RGBA", (ICON_SIZE, ICON_SIZE), (0, 0, 0, 0))
    canvas.paste(emblem, ((ICON_SIZE - new_size[0]) // 2, (ICON_SIZE - new_size[1]) // 2))

    os.makedirs(os.path.dirname(output_path), exist_ok=True)
    canvas.save(output_path, format="PNG")
    print(f"wrote {output_path} ({ICON_SIZE}x{ICON_SIZE}) from {source_path}")


def main() -> None:
    parser = argparse.ArgumentParser(description="Build the Soul App icon.")
    parser.add_argument("--source", default=DEFAULT_SOURCE)
    parser.add_argument("--output", default=DEFAULT_OUTPUT)
    args = parser.parse_args()
    build(args.source, args.output)


if __name__ == "__main__":
    main()
