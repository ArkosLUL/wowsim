# LAN access to the sim

## Context

The user wants the sim UI on their laptop and iPad over the home network. Findings:

- Access already works. The prod `sim` container (`wowsims-wotlk`) publishes `0.0.0.0:3333`. Docker
  Desktop's firewall rule (`com.docker.backend.exe`, Public profile, which is the network's profile) lets
  it in. The PC has a static IP, `192.168.49.35` (DHCP off, gateway `192.168.49.1`).
- The iPad (Brave) failed with `ERR_SSL_PROTOCOL_ERROR` only because Brave tried https first. With
  `http://192.168.49.35:3333/wotlk/` typed out, the sim loads. The user chose plain http, not an https
  proxy.
- Two page quirks appear when the page isn't opened on `localhost`:
  1. `ui/core/components/copy_button.tsx`: browsers hide `navigator.clipboard` outside https/localhost, so
     "Copy to Clipboard" falls back to `alert(data)`. Used by `exporters.tsx` and `talents_picker.tsx`.
  2. `ui/core/components/sim_header.tsx` `addDownloadBinaryLink`: checks
     `document.location.href.includes('localhost')`. Any other host gets an upstream "Download simulator"
     link (`.downbin`), though the sim server is serving the page.
- No firewall, router, Go or compose change is needed.

User decisions: commit straight on `master` in [sim]; then rebuild the prod sim container.
Ownership: both UI files were UI-SETTINGS' (wave U), which merged long ago. No open wave claims them.

## Steps

1. **Tests first** ([testing](../guide/testing.md#ui)), in `tools/uitest/tests/settings/`:
   - `header.spec.ts`:
     - Open a spec page via `127.0.0.1` instead of `localhost`: it isn't "localhost", but it's still a
       secure context. Wait for the `/version` response; expect no `.downbin`.
     - Route `/version` to a 404 (static hosting); expect `.downbin` visible.
   - `import_export.spec.ts`:
     - An init script hides `navigator.clipboard` and stubs `document.execCommand('copy')` to record the
       selected range of `document.activeElement` and return true.
     - Click an exporter's Copy button. Expect the recorded text to equal the exporter textarea, no
       dialog, and the button to show "Copied".
   - Run them against the dev server and see them fail on the old code.
2. **copy_button.tsx**: keep `navigator.clipboard.writeText` when present. Otherwise copy through a
   temporary `<textarea>` + `document.execCommand('copy')`:
   - Readonly, fixed and transparent. Call `select()` and `setSelectionRange(0, len)`: iOS ignores
     `select()` alone.
   - Append it next to the button: the exporter sits in a Bootstrap modal, whose focus trap pulls focus
     out of an element outside it, which empties the selection.
   - Remove it in `finally`. If `execCommand` returns false or throws, fall back to `alert(data)`. Show
     "Copied" only on success.
3. **sim_header.tsx** `addDownloadBinaryLink`: drop the hostname check. Always `fetch('/version')`:
   - JSON comes back: the sim server serves the page. Red link only when `outdated == 2`, as before.
   - Fetch or JSON parse fails: static hosting. Add the plain `.downbin` link.
4. **Doc**: one bullet under Run → Prod in [dev-environment](../guide/dev-environment.md): the LAN
   address, `http://` typed out, and why Brave fails without it.
5. Verify: `npx tsc --noEmit`; eslint each touched file against its HEAD copy; the two specs plus
   `smoke.spec.ts` against the dev server.
6. One fresh reviewer; fix its findings and re-verify.
7. Commit on `master`, then rebuild prod: `SIM_COMMIT=$(git rev-parse HEAD) docker compose up -d --build sim`.

## Verification

- Playwright at `http://192.168.49.35:3333/wotlk/mage/` (not localhost, not a secure context): no
  `.downbin`; an exporter's and the talents' Copy button show "Copied" with no alert.
- At `http://localhost:3333/wotlk/mage/`: still no `.downbin`, and Copy still works.
- The user reloads the sim on the iPad and copies from an export dialog.

## Status

Done. The static host's download link now lands at the toolbar's end, after the fetch fails: accepted,
since this fork never hosts statically.
