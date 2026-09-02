#!/usr/bin/env python3
"""The two bitmaps the installer's dialogs are drawn on.

    packaging/dialog-bitmaps.py <card.png> <mark.png> <out-dir>

WixUIDialogBmp is not a panel beside the text - it is the whole 493x312
background of the welcome and finished pages, and WiX draws its title and body
on top of it. So the artwork lives in a band down the left and the rest is left
white for the text to sit on, which is what every stock WiX bitmap does. A
full-bleed card there put dark artwork behind dark text and neither could be
read; this was found by screenshotting the wizard rather than by reading the
source.

WixUIBannerBmp is the same story the other way round: the page title is drawn
along its left, so a dark bar swallowed it. It is light, with the mark at the
right edge.

Pillow rather than ImageMagick, which is what this did first. Ubuntu 22.04 -
which is what lab-2204 is, and the glibc floor says it stays that way - carries
ImageMagick 6, whose command is `convert`; `magick` is 7 only. So the obvious
one-liner worked where it was written and would have failed where it runs, and
the difference is not visible until a release is being cut.
"""

import sys

from PIL import Image

# The band is the artwork; WiX writes over everything to the right of it.
BAND_W, DIALOG_W, DIALOG_H = 164, 493, 312
BANNER_W, BANNER_H = 493, 58
GROUND = (11, 10, 18)  # #0B0A12, the brand's own near-black
ACCENT = (255, 122, 69)  # #FF7A45
WHITE = (255, 255, 255)

# Regions of the card at its committed 1200x630. A redrawn card wants these
# checked - which is the price of pointing at the one copy rather than keeping
# a second, cropped one here.
MASSIF = (750, 290, 1130, 590)
WORDMARK = (92, 348, 637, 453)


def fit(img, width):
    """Scale to a width, keeping the shape."""
    return img.resize((width, max(1, round(img.height * width / img.width))), Image.LANCZOS)


def main():
    if len(sys.argv) != 4:
        sys.exit(__doc__)
    card = Image.open(sys.argv[1]).convert("RGBA")
    mark = Image.open(sys.argv[2]).convert("RGBA")
    out = sys.argv[3]

    band = Image.new("RGBA", (BAND_W, DIALOG_H), GROUND + (255,))

    # The terrain, half-strength: it is texture under the mark rather than
    # something to read, and at full strength it competes with the wordmark.
    massif = fit(card.crop(MASSIF), 150)
    faded = massif.copy()
    faded.putalpha(massif.getchannel("A").point(lambda a: a // 2))
    band.alpha_composite(faded, (26, 178))

    band.alpha_composite(fit(mark, 78), (43, 54))
    band.alpha_composite(fit(card.crop(WORDMARK), 124), (20, 150))

    dialog = Image.new("RGB", (DIALOG_W, DIALOG_H), WHITE)
    dialog.paste(band.convert("RGB"), (0, 0))
    dialog.paste(Image.new("RGB", (2, DIALOG_H), ACCENT), (BAND_W, 0))
    dialog.save(f"{out}/dialog.bmp", "BMP")

    banner = Image.new("RGBA", (BANNER_W, BANNER_H), WHITE + (255,))
    banner.alpha_composite(fit(mark, 34), (445, 12))
    banner = banner.convert("RGB")
    banner.paste(Image.new("RGB", (BANNER_W, 2), ACCENT), (0, BANNER_H - 2))
    banner.save(f"{out}/banner.bmp", "BMP")

    print(f"windows-msi: dialog.bmp {DIALOG_W}x{DIALOG_H}, "
          f"banner.bmp {BANNER_W}x{BANNER_H}")


if __name__ == "__main__":
    main()
