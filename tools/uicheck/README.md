# uicheck

Drives a running sim page in headless Chrome over the DevTools protocol, for a UI change's manual check:
navigate, run JavaScript in the page, take screenshots. It's how BIS-picker-switch checked the gear
picker's phase filter and the optimizer tab's results.

## Run

Needs Python with `websockets` (`pip install websockets`) and a dev server on the checkout
([dev-environment](../../docs/guide/dev-environment.md#run)).

1. Start Chrome headless on port 9333, which `cdp.py` expects, with a throwaway profile:

   ```sh
   "/c/Program Files/Google/Chrome/Application/chrome.exe" --headless=new --remote-debugging-port=9333 \
     --user-data-dir="$TEMP/uicheck-profile" --window-size=1600,1100 --no-first-run &
   ```

2. Drive the page:

   ```sh
   python tools/uicheck/cdp.py size 1600 1000
   python tools/uicheck/cdp.py nav http://localhost:3334/wotlk/deathknight/
   python tools/uicheck/cdp.py eval tools/uicheck/helpers.js
   python tools/uicheck/cdp.py eval "__t.openSlot(7)"
   python tools/uicheck/cdp.py shot page.png
   ```

   `eval` takes an expression or a `.js` file, waits for a returned promise, and prints the value.

3. Stop Chrome when done: it keeps running in the background.

## helpers.js

Installs `window.__t` for the individual sim's gear tab:

| Helper | Does |
|---|---|
| `sleep(ms)`, `until(fn, ms)` | wait, or poll until `fn` returns something truthy |
| `tab(id)` | switch to a sim tab, e.g. `optimizer-tab` |
| `openSlot(i)` | open the i-th item picker in page order: left column 0-8 (head to ranged), then right 9-16 (hands to trinket 2) |
| `showTab(contentId)`, `pane(contentId)` | switch to and read a picker tab (items, gems, enchants) |
| `setPhase(contentId, phase)`, `search(contentId, text)` | set the picker's phase filter, search its list |
| `listSize(contentId)` | the picker list's row count |
| `catalogFetches()` | the page's fetches of `server_catalog.json` |
