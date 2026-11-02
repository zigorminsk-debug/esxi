"""Regenerate PNG/ICO from the original application artwork (Pillow required).

After running this script, regenerate the Windows-only COFF resource with:
    go run github.com/akavel/rsrc@v0.10.2 -arch amd64 \
      -ico assets/raid-assistant.ico -o cmd/raid-assistant/raid_icon_windows_amd64.syso
Run both commands from the raid-assistant directory.
"""
from pathlib import Path
from PIL import Image, ImageDraw

HERE = Path(__file__).resolve().parent
image = Image.open(HERE / "raid-assistant-icon-source.png").convert("RGBA")
image = image.crop((190, 190, 835, 835)).resize((512, 512), Image.Resampling.LANCZOS)
mask = Image.new("L", (2048, 2048), 0)
ImageDraw.Draw(mask).rounded_rectangle((0, 0, 2047, 2047), radius=320, fill=255)
image.putalpha(mask.resize((512, 512), Image.Resampling.LANCZOS))
image.save(HERE / "raid-assistant-icon.png", optimize=True)
image.save(HERE / "raid-assistant.ico", format="ICO", sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)])
