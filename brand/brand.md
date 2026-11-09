# nixup brand guide

A short reference for anyone making a README, a slide, a badge or CLI text for nixup.

## Name

- Written **nixup**, all lowercase, one word, also at the start of a sentence. Not
  "NixUp", "Nixup" or "nix-up". In code formatting, write `nixup`.
- Pronounced "nix up" (/nɪks ʌp/), like "fix up".
- "nix" is the ecosystem it works in; "up" is what it does: bring things up to date.

## Concept: the held update

The mark is an upward chevron (an update) stopped below a short bar (a gate). The
update is ready, but it waits until you have looked at it and said yes. That is the
whole product: nixup shows what changed upstream, builds in a scratch copy and asks
before it writes anything.

The bar is also a terminal cursor, square-ended like a block cursor, waiting for your
input at a `[y/N]` prompt. It is amber because amber is the colour nixup uses for
"look at this": major version jumps and pending questions.

Why not the other motifs: an arrow alone says "update" but not "careful"; a layered
copy needs too much detail to survive 16 px; a bare prompt (`>_`) says "terminal tool"
and nothing about this one. The held chevron carries both "update" and "review first"
in two shapes.

The shape is original and has nothing to do with the NixOS snowflake. The only nod to
the Nix ecosystem is the blue. Do not add snowflakes, lambdas or hexagons to it.

## Colour

One primary, one accent, GitHub-compatible neutrals. Contrast ratios are WCAG 2.x.

| Name | Hex | Role | Contrast |
|---|---|---|---|
| Gate blue | `#4A6FC4` | Primary. Logo tile; "up" in the light wordmark; links and highlights on light backgrounds | 4.81:1 on `#FFFFFF` (AA text). 3.93:1 on `#0D1117` (graphics and large text only) |
| Gate blue, light | `#7EA1E8` | Primary on dark backgrounds: "up" in the dark wordmark, links, highlights | 7.34:1 on `#0D1117` (AA, nearly AAA) |
| Review amber | `#E3A62F` | Accent. The gate bar; warnings and prompts on dark backgrounds | 8.80:1 on `#0D1117`. 2.15:1 on white: **never use it for text on light** |
| Review amber, dark | `#9A6700` | Accent as text on light backgrounds | 4.87:1 on `#FFFFFF` (AA) |
| Ink | `#1F2328` | Text on light | 15.80:1 on `#FFFFFF` |
| Ink muted | `#59636E` | Secondary text on light (taglines, captions) | 6.11:1 on `#FFFFFF` |
| Snow | `#E6EDF3` | Text on dark | 16.02:1 on `#0D1117` |
| Snow muted | `#9198A1` | Secondary text on dark | 6.50:1 on `#0D1117` |
| Night | `#0D1117` | Dark background (same as GitHub dark) | n/a |
| Paper | `#FFFFFF` | Light background | n/a |

Inside the mark: the white chevron on Gate blue is 4.81:1. The amber bar on blue is only
2.24:1, so the mark never relies on that pair alone; the shape stays readable in one
colour (see `logo-mono.svg`).

The blue tile is 4.81:1 against white and 3.93:1 against Night, which is why the same
colour logo works on both GitHub themes without a variant.

## Typography

| Use | Family | Weight | Notes |
|---|---|---|---|
| Wordmark | Inter | Bold 700 | Outlined in the SVGs, letter-spacing -2.5 %. "nix" in Ink or Snow, "up" in Gate blue or Gate blue light |
| Taglines, headings, prose | Inter | Regular 400 / SemiBold 600 | |
| Commands, CLI output, file names | Fira Code | Regular 400 to Medium 500 | Used for the steps line in `social.svg` |

Both are open fonts (SIL OFL 1.1). The wordmark in the SVGs is plain Inter Bold glyphs
converted to paths, so it looks the same everywhere, GitHub included, with no web fonts.
Write CSS stacks with fallbacks:
`Inter, ui-sans-serif, system-ui, sans-serif` and
`"Fira Code", ui-monospace, "SF Mono", Menlo, Consolas, monospace`.
Google Sans Flex is not used in brand assets.

## Logo usage

**Parts.** The mark (`logo.svg`) and the lockup: mark + wordmark + tagline (the headers).
The wordmark is never used without the mark, except in running text where it is just
the name.

**Clear space.** Around the mark: at least 1/4 of the mark width (16 units on its 64-unit
grid). In the lockup, the gap between mark and wordmark is 0.3 × the mark height.

**Minimum size.** Mark: 16 px (favicon); prefer 24 px or more in UI. Lockup: 160 px wide;
below that, use the mark alone.

**Colour versions.**
- `logo.svg`: full colour. Works on white, Night, and mid-grey backgrounds.
- `logo-mono.svg`: one colour, `fill="currentColor"`, chevron and bar cut out of the tile.
  Inline it (or use it where the host sets `color`) for favicons, badges and docs. Loaded
  through `<img>` it renders black, which is invisible on dark backgrounds: there, use
  `logo.svg`.

**Do**
- Use the files as they are; scale them proportionally.
- Use `header-light.svg` / `header-dark.svg` together through `<picture>` and
  `prefers-color-scheme`.
- Keep the gate bar above the chevron, with the gap.

**Don't**
- Remove the gate bar or close the gap; the mark then becomes a generic "up" arrow.
- Swap the colours (amber tile, blue bar), add gradients, shadows, outlines or glows.
- Rotate the chevron, add a shaft to it, or animate it "launching".
- Put the colour logo on a mid-blue or amber background where the tile disappears.
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
| `logo.svg` | Mark, full colour, transparent background, 64-unit viewBox | 0.4 KB |
| `logo-mono.svg` | Mark, one colour (`currentColor`), single path | 0.5 KB |
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
