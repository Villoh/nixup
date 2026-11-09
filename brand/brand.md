# nixup brand guide

A short reference for anyone making a README, a slide, a badge or CLI text for nixup.

## Name

- Written **nixup**, all lowercase, one word, also at the start of a sentence. Not
  "NixUp", "Nixup" or "nix-up". In code formatting, write `nixup`.
- Pronounced "nix up" (/nɪks ʌp/), like "fix up".
- "nix" is the ecosystem it works in; "up" is what it does: bring things up to date.

## Concept: the lambda that goes up

The mark is a lambda (λ) drawn so that its lower half is an upward arrow (Λ). The tail
and the arrow are separate pieces with a small gap between them: read as a whole it is
the Nix lambda, read in parts it is "Nix" and "up". That is the name.

Geometry, on a 64-unit grid (the SVGs are generated from these numbers):

- Both legs of the arrow and the tail run at 60° from the horizontal.
- Every end is cut horizontally; every corner is sharp. No round caps, no strokes: the
  shapes are closed polygons.
- Stroke width 9 units (measured horizontally), gap 3 units.
- The tail is half the height of the arrow (12 and 24 units). Its right edge is in line
  with the arrow's right outer edge, so the lambda reads as one letter.
- The gap between tail and arrow runs parallel to the arrow's upper-left edge.

The lambda is drawn from scratch. It is not the NixOS snowflake nor any part of it, and
it must not be combined with it: the NixOS logo is a registered trademark and its guide
forbids altering or recombining its lambdas.

## Colour

One primary, GitHub-compatible neutrals, no accent colour. Contrast ratios are WCAG 2.x.

| Name | Hex | Role | Contrast |
|---|---|---|---|
| Blue | `#4A6FC4` | Primary. Logo tile; the tail on light backgrounds; "up" in the light wordmark; links | 4.81:1 on `#FFFFFF` (AA text). 3.93:1 on `#0D1117` (graphics and large text only) |
| Blue, light | `#7EA1E8` | Primary on dark backgrounds: the tail, "up" in the dark wordmark, links | 7.34:1 on `#0D1117` (AA, nearly AAA) |
| Ink | `#1F2328` | Text on light; the arrow on light backgrounds and on the tile | 15.80:1 on `#FFFFFF` |
| Ink muted | `#59636E` | Secondary text on light (taglines, captions) | 6.11:1 on `#FFFFFF` |
| Snow | `#E6EDF3` | Text on dark; the arrow on dark backgrounds | 16.02:1 on `#0D1117` |
| Snow muted | `#9198A1` | Secondary text on dark | 6.50:1 on `#0D1117` |
| Night | `#0D1117` | Dark background (same as GitHub dark) | n/a |
| Paper | `#FFFFFF` | Light background | n/a |

Inside the tile: the white tail on Blue is 4.81:1 and the Ink arrow on Blue is 3.28:1,
both above the 3:1 WCAG minimum for graphics. The mark also reads in one colour, because
the gap carries the shape (see `logo-mono.svg`).

## Typography

| Use | Family | Weight | Notes |
|---|---|---|---|
| Wordmark | Inter | Bold 700 | Outlined in the SVGs, letter-spacing -2.5 %. "nix" in Ink or Snow, "up" in Blue or Blue light |
| Taglines, headings, prose | Inter | Regular 400 / SemiBold 600 | |
| Commands, CLI output, file names | Fira Code | Regular 400 to Medium 500 | Used for the steps line in `social.svg` |

Both are open fonts (SIL OFL 1.1). The wordmark in the SVGs is plain Inter Bold glyphs
converted to paths, so it looks the same everywhere, GitHub included, with no web fonts.
Write CSS stacks with fallbacks:
`Inter, ui-sans-serif, system-ui, sans-serif` and
`"Fira Code", ui-monospace, "SF Mono", Menlo, Consolas, monospace`.
Google Sans Flex is not used in brand assets.

## Logo usage

**Parts.** The mark comes in two versions, with and without the tile, and the lockup is
mark + wordmark + tagline (the headers). The wordmark is never used without the mark,
except in running text where it is just the name.

**Clear space.** Around the mark: at least 1/4 of its width. In the lockup, the gap
between mark and wordmark is about 0.3 × the mark height.

**Minimum size.** Mark: 16 px (favicon); prefer 24 px or more in UI. Lockup: 160 px wide;
below that, use the mark alone.

**Versions.**
- `logo.svg`: on the Blue tile (white tail, Ink arrow). App icon, avatar, favicon,
  social image. Works on light and dark backgrounds.
- `mark-light.svg` / `mark-dark.svg`: without the tile, for light (Blue tail, Ink arrow)
  and dark (Blue light tail, Snow arrow) backgrounds. Used in the headers. Pair them with
  `<picture>` and `prefers-color-scheme`.
- `logo-mono.svg`: one colour, `fill="currentColor"`, no tile. Inline it (or use it where
  the host sets `color`) for badges, docs and print. Loaded through `<img>` it renders
  black, which is invisible on dark backgrounds.

**Do**
- Use the files as they are; scale them proportionally.
- Keep the gap between the tail and the arrow.
- Use the tile version where the mark stands alone; the bare version next to the wordmark.

**Don't**
- Close the gap or give the two pieces round ends; it then becomes a plain λ or a
  rounded doodle.
- Recolour the pieces (the arrow is always Ink or Snow, the tail always Blue or white),
  add gradients, shadows, outlines or glows.
- Rotate, mirror or stretch the mark, or lengthen the tail.
- Put the tile on a mid-blue background where it disappears.
- Rewrite the wordmark in another font, capitalise it, or colour all of it blue.
- Combine the mark with the NixOS snowflake or present nixup as an official NixOS project.

## Tone of voice

nixup reads like a careful colleague reporting what they found. Rules:

1. **Facts first.** Say what happened, to which file, and what was not done.
   Name the file or command exactly (`flake.lock`, `packages/foo`, `nh os switch`).
2. **Never claim more than happened.** "Updated" means written to disk. Say what is
   still up to the user: not committed, not activated.
3. **Calm.** No exclamation marks, no emoji, no hype words ("blazing", "magic",
   "effortless", "seamless"). The only symbol is `⚠` for a likely breaking change.
4. **The user decides.** Questions default to No. When something is refused or
   discarded, say what was left untouched and how to go on.
5. **Short.** One line per fact. Lowercase CLI messages, no trailing chatter.

**CLI output**

| On-brand | Off-brand |
|---|---|
| `flake.lock updated (not committed, nothing activated).` | `✅ All done! Your system is now up to date 🎉` |
| `Discarded. Nothing changed.` | `Oops! Update cancelled.` |
| `flake.lock has uncommitted changes: commit or stash it, then re-run.` | `Error: invalid state.` |
| `⚠ MAJOR version jump 1.x → 2.x` | `🚨 WARNING!!! This update may BREAK your system!` |
| `Build of desktop failed: flake.lock left untouched.` | `Something went wrong. Please try again.` |
| `Everything is up to date.` | `Awesome, you're all caught up! ✨` |

**Release notes**

| On-brand | Off-brand |
|---|---|
| `check: report flatpak updates in their own section` | `🚀 Huge new feature: Flatpak support!` |
| `lock: refuse to start when flake.lock has uncommitted changes` | `Improved safety and reliability.` |
| `pkg: pass passthru.updateScript extraArgs to nix-update (fixes prerelease packages)` | `Various bug fixes and improvements.` |

## Files

| File | What | Size (approx.) |
|---|---|---|
| `logo.svg` | Mark on the Blue tile, 64-unit viewBox | 0.4 KB |
| `mark-light.svg` | Mark without tile, for light backgrounds | 0.3 KB |
| `mark-dark.svg` | Mark without tile, for dark backgrounds | 0.3 KB |
| `logo-mono.svg` | Mark without tile, one colour (`currentColor`), single path | 0.3 KB |
| `header-light.svg` | README banner for light themes, 1280×320, transparent background | 14 KB |
| `header-dark.svg` | README banner for dark themes, 1280×320, transparent background | 14 KB |
| `social.svg` | Source of the social preview, 1280×640, Night background | 21 KB |
| `social.png` | Social preview rendered from `social.svg` (GitHub: Settings → Social preview) | 46 KB |
| `brand.md` | This guide | |

All text in the SVGs is outlined (no font files, no raster images). Each glyph is
defined once in `<defs>` and placed with `<use>`. To change the tagline, set it in
Inter and outline it again; re-render the PNG with
`rsvg-convert -w 1280 -h 640 social.svg -o social.png`.

README snippet:

```html
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="brand/header-dark.svg">
  <img alt="nixup: safe updates for flake-based NixOS. Review first, then decide." src="brand/header-light.svg">
</picture>
```
