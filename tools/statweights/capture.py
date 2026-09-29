"""Captures each build's sim settings at each phase's BiS gear, as the ComputeStatsRequest its
individual sim page sends, into --out/settings/<build>_p<N>.bin for the Go runner.

    python tools/statweights/capture.py --out tmp/statweights
    python tools/statweights/capture.py --out tmp/statweights --builds warrior_fury,mage_arcane --phases 1,5
"""
import argparse
import base64
import copy
import json
import sys
import time
from pathlib import Path

sys.dont_write_bytecode = True
REPO = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO / 'tools' / 'presetgen'))
import gen  # noqa: E402

# An exported settings JSON keeps the rotation as TypeAuto, without the APL the page resolves, so
# the request the page posts to its worker is what gets saved.
CAPTURE_JS = """(() => {
  if (!Worker.prototype.__swPost) {
    Worker.prototype.__swPost = Worker.prototype.postMessage;
    Worker.prototype.postMessage = function (m, ...rest) {
      if (m && m.msg === 'computeStats') { window.__swLast = m.inputData; window.__swCount = (window.__swCount || 0) + 1; }
      return this.__swPost(m, ...rest);
    };
  }
  return 'ok';
})()"""

B64_JS = """(() => { const b = window.__swLast; let s = ''; for (let i = 0; i < b.length; i += 0x8000) s += String.fromCharCode.apply(null, b.subarray(i, i + 0x8000)); return btoa(s); })()"""

QUIET_SECONDS = 2


def last_request_after(b, action):
    """Runs action, then waits until the page has sent a new request and nothing more for QUIET_SECONDS."""
    before = b.eval('window.__swCount || 0')
    action()
    end = time.time() + 30
    while True:
        count = b.eval('window.__swCount || 0')
        if count > before:
            time.sleep(QUIET_SECONDS)
            if b.eval('window.__swCount || 0') == count:
                break
        if time.time() > end:
            raise gen.CdpError('the page sent no computeStats request')
        time.sleep(0.5)
    return base64.b64decode(b.eval(B64_JS, timeout=30))


def capture_build(driver, build, phases, settings_dir, log):
    todo = [p for p in phases if not (settings_dir / f'{build.key}_p{p}.bin').exists()]
    if not todo:
        log(f'{build.key}: all requested phases already captured')
        return
    meta_path = gen.REPO / 'ui' / build.dir / 'gear_sets' / gen.META_FILE
    meta = json.loads(meta_path.read_text(encoding='utf-8')) if meta_path.exists() else {}
    base = driver.open_build(build)
    driver.browser.eval(CAPTURE_JS)
    for phase in todo:
        path = gen.gear_path(build, phase)
        if not path.exists():
            raise SystemExit(f'{build.key}: no P{phase} gear at {path}')
        settings = copy.deepcopy(base)
        traits = meta.get(gen.gear_stem(build, phase), {}).get('racialTraits')
        if traits:
            settings['player']['racialTraits'] = traits
        items = json.loads(path.read_text(encoding='utf-8'))['items']
        data = last_request_after(driver.browser, lambda: driver.apply_settings(settings, items))
        dest = settings_dir / f'{build.key}_p{phase}.bin'
        tmp = dest.with_suffix('.bin.tmp')
        tmp.write_bytes(data)
        tmp.replace(dest)
        log(f'{build.key} P{phase}: {path.name}, racial traits {traits}, {len(data)} bytes')


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument('--out', required=True, help='work directory: writes settings/, capture.log and the Chrome profile')
    parser.add_argument('--builds', default='all', help="comma-separated keys from presetgen's BUILDS, or all (default)")
    parser.add_argument('--phases', default='1-5', help='e.g. 1-5 or 1,5 (default 1-5)')
    parser.add_argument('--base', default='http://localhost:3333', help='sim server whose UI matches this checkout')
    parser.add_argument('--port', type=int, default=9361, help="Chrome's debugging port (default 9361)")
    args = parser.parse_args()

    sys.stdout.reconfigure(encoding='utf-8', errors='replace')
    build_list = gen.parse_list(args.builds, [b.key for b in gen.BUILDS], 'build')
    phases = gen.parse_phases(args.phases)
    out = Path(args.out).resolve()
    settings_dir = out / 'settings'
    settings_dir.mkdir(parents=True, exist_ok=True)
    log = gen.Log(out / 'capture.log')
    if not gen.server_ok(args.base):
        raise SystemExit(f'the sim server at {args.base} is not answering')
    log('capture: ' + ' '.join(sys.argv[1:]))
    driver = gen.Driver(args, log)
    driver.browser.start()
    try:
        for key in build_list:
            capture_build(driver, gen.BUILDS_BY_KEY[key], phases, settings_dir, log)
    except gen.CdpError as e:
        log(f'! {e}')
        raise
    finally:
        driver.browser.stop()


if __name__ == '__main__':
    main()
