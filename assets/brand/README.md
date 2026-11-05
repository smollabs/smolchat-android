# SmolWorks brand artwork

Master files (not embedded in the app). Everything the app ships is derived
from these:

| Master | Derived file | Used for |
|---|---|---|
| `logo-1254.png` | `assets/icons/icon.png` (512 px) | Android launcher icon (`gogio -icon`) |
| `logo-314.png` | `assets/images/logo.png` (copy) | Drawer, assistant avatar, error screen |
| `smollabs_mascot_clean.png` | `assets/images/mascot.png` (720 px wide) | Splash and welcome screen |

`smollabs_mascot.png` is the original export. The `_clean` version removes a
stray arc in its top-left corner and is the one to use. `logo-157.png` and
`logo-627.png` are spare sizes.

To regenerate the derived files on macOS:

```bash
sips -Z 512 assets/brand/logo-1254.png --out assets/icons/icon.png
cp assets/brand/logo-314.png assets/images/logo.png
sips -Z 720 assets/brand/smollabs_mascot_clean.png --out assets/images/mascot.png
```
