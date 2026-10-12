# ADR-0008: PWA Removed from the Desktop Main Build

- Status: Accepted
- Date: 2026-09-28
- Related: [ADR-0006](0006-scope-freeze.md) (this document implements the disposition of PWA deep capabilities in its "Deferred / to remove" tier)

## Background

ADR-0006 placed PWA deep capabilities in "Deferred / to remove: no further investment; evaluate removal from the desktop main build". Subsequent cleanup tore down the PWA infrastructure layer by layer (the `vite-plugin-pwa` config and dependency, manifest, service worker, install UI), but **the decision never fully landed**: the README still carried the deferred "installable to desktop" line, `docs/getting-started.md` still listed the PWA positioning, `docs/features/settings.md` still described a nonexistent "install to desktop" setting, `web/public/` still held 3 PWA icons referenced by nothing, and both locale sets retained 11 orphan strings each.

The leftovers are a **narrative conflict**, beyond mere hygiene: the product is a Wails native window application, and "install as a PWA for a standalone window" directly contradicts the native window. Documentation promises, code facts, and product positioning disagree in three places, and each disagreement keeps draining frontend budget and positioning purity.

## Decision

1. **PWA removed from the desktop main build**: no manifest / service worker / install entry point. The desktop install path is singular — platform packages distributed via Releases.
2. **Web build retained**: the frontend is kept; `npm run build` produces output as usual, and browser development preview (vite dev, headless browser access scenarios) is unaffected — it simply stops being an "installable PWA".
3. **Leftovers cleared to zero**: delete the PWA icons, orphan locale strings, and inaccurate documentation lines; route comments switch to a "browser / desktop shell" distinction, and the browser mode is no longer described in PWA narrative.

## Rationale

- Native window and "install as PWA" are two promises of the same value; only one should remain. The Wails single binary is the product's primary form.
- The infrastructure is already dismantled; what remains is maintenance surface: inaccurate claims, orphaned resources, documentation debt. Clearing is a one-time cost; keeping is a recurring cost.
- Keeping the web build costs near zero (the vite config was designed for Wails and non-HTTP origins anyway); what gets closed off is the "PWA narrative", leaving the development experience intact.

## Consequences

- Positive: documentation promises match code facts; `web/public/` keeps only the favicon; the frontend no longer carries dual-state logic such as install state and offline fallback.
- Negative: users who once installed Repotory as a PWA lose "install entry" as a discovery path — but the desktop scenario they actually rely on is covered by the Releases packages, and the manifest has long been gone, so installation was already impossible in practice.
- Leftover: the tension between the dashboard's "productivity facade" (goal rings / daily code volume standards / workday alerts) and the "memory layer" positioning is handled along the 2.0 convergence direction (see the TODO leftovers); out of scope for this document.
