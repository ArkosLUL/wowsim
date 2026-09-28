"""Drives the individual sim's BiS Optimizer tab in headless Chrome to generate the phase 1-5 gear
presets for BIS-presets (bis-optimizer.PLAN.md), one spec/tree "build" at a time.

Each build: fresh page state, its tree's talent preset, an Alliance race, all 11 professions, and the
tree's current P1 gear preset as the phase 1 seed. Each later phase seeds from the previous phase's
pick. Writes ui/<spec>/gear_sets/p{N}_bis[_<tree>].gear.json, its entry in that directory's
bis_presets.json (the preset's label and tooltip data), and a summary line per (build, phase) to
--out/summary.jsonl, resumable: a (build, phase, effort) already in the summary is skipped.

    python gen.py run --out DIR --effort Normal
    python gen.py run --out DIR --effort Normal --builds warrior_fury,tank_dk_blood --phases 1,5 --no-write
"""
import argparse
import json
import os
import subprocess
import sys
import time
import urllib.request
from collections import namedtuple
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
CDP = REPO / 'tools' / 'uicheck' / 'cdp.py'
PAGE_JS = HERE / 'page.js'
CHROME = r'C:\Program Files\Google\Chrome\Application\chrome.exe'
META_FILE = 'bis_presets.json'

ALL_PROFESSIONS = [
    'Alchemy', 'Blacksmithing', 'Enchanting', 'Engineering', 'Herbalism', 'Inscription',
    'Jewelcrafting', 'Leatherworking', 'Mining', 'Skinning', 'Tailoring',
]

# talents: the talent preset's name on the Talents tab. seed: the P1 seed, a file in ui/<dir>/gear_sets.
# tree: the gear file's suffix, None for a single-tree spec. name: the preset chip's label after "P<N> ".
Build = namedtuple('Build', 'key dir label talents seed tree name tank race')
BUILDS = [
    Build('dk_blood', 'deathknight', 'DK Blood', 'Blood DPS', 'p1_blood.gear.json', 'blood', 'Blood', False, 'RaceDraenei'),
    Build('dk_frost', 'deathknight', 'DK Frost', 'Frost BL', 'p1_frost.gear.json', 'frost', 'Frost', False, 'RaceDraenei'),
    Build('dk_unholy', 'deathknight', 'DK Unholy', 'Unholy DW', 'p1_uh_dw.gear.json', 'unholy', 'Unholy', False, 'RaceDraenei'),
    Build('hunter_mm', 'hunter', 'Hunter MM', 'Marksman', 'p1_mm.gear.json', 'mm', 'MM', False, 'RaceDraenei'),
    Build('hunter_sv', 'hunter', 'Hunter SV', 'Survival', 'p1_sv.gear.json', 'sv', 'SV', False, 'RaceDraenei'),
    Build('mage_arcane', 'mage', 'Mage Arcane', 'Arcane', 'p1_arcane.gear.json', 'arcane', 'Arcane', False, 'RaceDraenei'),
    Build('mage_fire', 'mage', 'Mage Fire', 'Fire', 'p1_fire.gear.json', 'fire', 'Fire', False, 'RaceDraenei'),
    Build('mage_frost', 'mage', 'Mage Frost', 'Frost', 'p1_frost.gear.json', 'frost', 'Frost', False, 'RaceDraenei'),
    Build('rogue_assassination', 'rogue', 'Rogue Assassination', 'Assassination 13/7', 'p1_assassination.gear.json', 'assassination', 'Assassination', False, 'RaceDwarf'),
    Build('rogue_combat', 'rogue', 'Rogue Combat', 'Combat Fists', 'p1_combat.gear.json', 'combat', 'Combat', False, 'RaceDwarf'),
    Build('rogue_subtlety', 'rogue', 'Rogue Subtlety', 'Hemo Sub', 'p1_hemosub.gear.json', 'subtlety', 'Subtlety', False, 'RaceDwarf'),
    Build('warlock_affliction', 'warlock', 'Warlock Affliction', 'Affliction', 'p1_affliction.gear.json', 'affliction', 'Affliction', False, 'RaceGnome'),
    Build('warlock_demonology', 'warlock', 'Warlock Demonology', 'Demonology', 'p1_demodestro.gear.json', 'demonology', 'Demonology', False, 'RaceGnome'),
    Build('warlock_destruction', 'warlock', 'Warlock Destruction', 'Destruction', 'p1_demodestro.gear.json', 'destruction', 'Destruction', False, 'RaceGnome'),
    Build('warrior_arms', 'warrior', 'Warrior Arms', 'Arms', 'p1_arms.gear.json', 'arms', 'Arms', False, 'RaceDraenei'),
    Build('warrior_fury', 'warrior', 'Warrior Fury', 'Fury', 'p1_fury.gear.json', 'fury', 'Fury', False, 'RaceDraenei'),
    Build('tank_dk_blood', 'tank_deathknight', 'Tank DK Blood', 'Blood', 'p1_blood.gear.json', 'blood', 'Blood', True, 'RaceDraenei'),
    Build('tank_dk_frost', 'tank_deathknight', 'Tank DK Frost', 'Frost', 'p1_frost.gear.json', 'frost', 'Frost', True, 'RaceDraenei'),
    Build('balance_druid', 'balance_druid', 'Balance Druid', 'Phase 3', 'p1.gear.json', None, 'Balance', False, 'RaceNightElf'),
    Build('feral_druid', 'feral_druid', 'Feral Druid', 'Standard', 'p1.gear.json', None, 'Feral', False, 'RaceNightElf'),
    Build('feral_tank_druid', 'feral_tank_druid', 'Feral Tank Druid', 'Standard', 'p1.gear.json', None, 'Feral Tank', True, 'RaceNightElf'),
    Build('elemental_shaman', 'elemental_shaman', 'Elemental Shaman', 'Standard', 'p1.gear.json', None, 'Elemental', False, 'RaceDraenei'),
    Build('enhancement_shaman', 'enhancement_shaman', 'Enhancement Shaman', 'Standard', 'p1.gear.json', None, 'Enhancement', False, 'RaceDraenei'),
    Build('protection_paladin', 'protection_paladin', 'Protection Paladin', 'Baseline Example', 'p1.gear.json', None, 'Protection', True, 'RaceDraenei'),
    Build('retribution_paladin', 'retribution_paladin', 'Retribution Paladin', 'Aura Mastery', 'p1.gear.json', None, 'Retribution', False, 'RaceDraenei'),
    Build('protection_warrior', 'protection_warrior', 'Protection Warrior', 'Standard', 'p1_balanced.gear.json', None, 'Protection', True, 'RaceDraenei'),
    Build('shadow_priest', 'shadow_priest', 'Shadow Priest', 'Standard', 'p1.gear.json', None, 'Shadow', False, 'RaceDraenei'),
    Build('smite_priest', 'smite_priest', 'Smite Priest', 'Standard', 'p1.gear.json', None, 'Smite', False, 'RaceDraenei'),
]
BUILDS_BY_KEY = {b.key: b for b in BUILDS}


class CdpError(Exception):
    pass


class Log:
    def __init__(self, path):
        self.file = open(path, 'a', encoding='utf-8')

    def __call__(self, message):
        line = f'{time.strftime("%Y-%m-%d %H:%M:%S")} {message}'
        try:
            print(line, flush=True)
        except OSError:
            pass
        self.file.write(line + '\n')
        self.file.flush()


class Browser:
    def __init__(self, port, profile, log):
        self.port = port
        self.profile = Path(profile).resolve()
        self.log = log
        self.process = None

    def version(self):
        try:
            return json.load(urllib.request.urlopen(f'http://127.0.0.1:{self.port}/json/version', timeout=3))
        except (OSError, ValueError):
            return None

    def alive(self):
        return self.version() is not None

    def start(self):
        if self.alive():
            self.log(f'reusing the Chrome on port {self.port}')
            return
        self.profile.mkdir(parents=True, exist_ok=True)
        flags = [
            '--headless=new', f'--remote-debugging-port={self.port}', f'--user-data-dir={self.profile}',
            '--window-size=1600,1100', '--no-first-run', '--no-default-browser-check', 'about:blank',
        ]
        detached = getattr(subprocess, 'DETACHED_PROCESS', 0) | getattr(subprocess, 'CREATE_NEW_PROCESS_GROUP', 0)
        self.process = subprocess.Popen([CHROME, *flags], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                                        creationflags=detached)
        for _ in range(60):
            if self.alive():
                self.log(f'started Chrome on port {self.port}, profile {self.profile}')
                return
            time.sleep(0.5)
        raise CdpError(f'Chrome did not come up on port {self.port}')

    # only the Chrome this run started, by its own process tree: other runs and people share the machine
    def stop(self):
        if self.process is None:
            return
        if os.name == 'nt':
            subprocess.run(['taskkill', '/PID', str(self.process.pid), '/T', '/F'], capture_output=True)
        else:
            self.process.terminate()
        self.process = None
        self.log('stopped the Chrome this run started')

    def cdp(self, *args, timeout=120):
        env = dict(os.environ, UICHECK_PORT=str(self.port), PYTHONIOENCODING='utf-8')
        try:
            done = subprocess.run([sys.executable, str(CDP), *args], capture_output=True, encoding='utf-8',
                                   errors='replace', env=env, timeout=timeout)
        except subprocess.TimeoutExpired:
            raise CdpError(f'cdp {args[0]} timed out after {timeout} s')
        if done.returncode != 0:
            lines = done.stderr.strip().splitlines()
            raise CdpError(f'cdp {args[0]} failed: {lines[-1] if lines else done.returncode}')
        out = done.stdout.rstrip('\r\n')
        if out.startswith('EXCEPTION'):
            raise CdpError(out[:2000])
        return out

    def eval(self, expr, timeout=120):
        out = self.cdp('eval', expr, timeout=timeout)
        try:
            return json.loads(out)
        except ValueError:
            return out


def server_ok(base):
    try:
        text = urllib.request.urlopen(f'{base}/wotlk/sim_worker.js', timeout=10).read().decode('utf-8', 'replace')
        return '/asyncProgress' in text
    except OSError:
        return False


def parse_list(text, all_values, kind):
    if not text or text == 'all':
        return list(all_values)
    picked = [v.strip() for v in text.split(',')]
    unknown = [v for v in picked if v not in all_values]
    if unknown:
        raise SystemExit(f'unknown {kind}: {unknown}')
    return picked


def parse_phases(text):
    phases = set()
    for part in (text or '1-5').split(','):
        if '-' in part:
            lo, hi = part.split('-')
            phases.update(range(int(lo), int(hi) + 1))
        else:
            phases.add(int(part))
    if not phases or min(phases) < 1 or max(phases) > 5:
        raise SystemExit(f'phases must be within 1-5: {text}')
    return sorted(phases)


def gear_stem(build, phase):
    return f'p{phase}_bis' + (f'_{build.tree}' if build.tree else '')


def gear_path(build, phase):
    return REPO / 'ui' / build.dir / 'gear_sets' / f'{gear_stem(build, phase)}.gear.json'


def write_gear(build, phase, items):
    path = gear_path(build, phase)
    path.parent.mkdir(parents=True, exist_ok=True)
    # the repo's gear file style: compact items, one per line
    lines = ',\n'.join(f'    {json.dumps(it, separators=(",", ":"))}' for it in items)
    path.write_text('{"items": [\n' + lines + '\n]}\n', encoding='utf-8')
    return path


def write_meta(build, phase, row):
    """Sets (build, phase)'s entry in its spec's bis_presets.json, which presets.ts builds the preset's
    label and tooltip from. Other builds' entries stay as they are."""
    path = REPO / 'ui' / build.dir / 'gear_sets' / META_FILE
    meta = json.loads(path.read_text(encoding='utf-8')) if path.exists() else {}
    # no racial traits in the result means it kept the base race's own
    meta[gear_stem(build, phase)] = {
        'name': f'P{phase} {build.name}', 'build': build.label, 'phase': phase, 'effort': row['effort'],
        'racialTraits': row['racialTraits'] or build.race, 'simCommit': row['simCommit'] or 'unknown',
        'catalogDate': row['catalogDate'] or 'unknown',
    }
    path.write_text(json.dumps(dict(sorted(meta.items())), indent='\t') + '\n', encoding='utf-8')
    return path


class Summary:
    """One line per settled (build, phase, effort) in --out/summary.jsonl, with the pick's items, so a
    rerun skips what's already done and seeds the next phase from it."""

    def __init__(self, out_dir):
        self.path = out_dir / 'summary.jsonl'
        self.done = {}
        if self.path.exists():
            for line in self.path.read_text(encoding='utf-8').splitlines():
                if line.strip():
                    row = json.loads(line)
                    self.done[(row['build'], row['phase'], row['effort'])] = row

    def get(self, build_key, phase, effort):
        return self.done.get((build_key, phase, effort))

    def add(self, row):
        self.done[(row['build'], row['phase'], row['effort'])] = row
        with open(self.path, 'a', encoding='utf-8') as f:
            f.write(json.dumps(row) + '\n')


def ready_expr():
    return ("document.readyState == 'complete' && "
            "!!document.querySelector('.import-dropdown .dropdown-item')")


class Driver:
    def __init__(self, args, log):
        self.args = args
        self.log = log
        self.out = Path(args.out).resolve()
        self.out.mkdir(parents=True, exist_ok=True)
        self.browser = Browser(args.port, self.out / 'chrome-profile', log)
        self.summary = Summary(self.out)

    # a writing run redoes a phase an earlier --no-write run settled, since that one left no files
    def done(self, build, phase):
        row = self.summary.get(build.key, phase, self.args.effort)
        return row is not None and (self.args.no_write or 'gearFile' in row)

    def seed_items(self, build, phase):
        """Phase 1 seeds from the tree's current P1 preset file. A later phase seeds from the previous
        phase's pick at this effort in the summary, else from its gear file."""
        if phase == 1:
            path = REPO / 'ui' / build.dir / 'gear_sets' / build.seed
        else:
            row = self.summary.get(build.key, phase - 1, self.args.effort)
            if row and row.get('items'):
                return row['items']
            path = gear_path(build, phase - 1)
        if not path.exists():
            raise SystemExit(f'{build.label}: no seed gear at {path} (run phase {phase - 1} first)')
        return json.loads(path.read_text(encoding='utf-8'))['items']

    def open_build(self, build):
        url = f'{self.args.base}/wotlk/{build.dir}/'
        b = self.browser
        b.cdp('nav', url, timeout=30)
        end = time.time() + 60
        while b.eval(ready_expr(), timeout=15) is not True:
            if time.time() > end:
                raise CdpError(f'{build.label}: page did not load')
            time.sleep(1)
        b.eval("localStorage.clear(); 'ok'")
        b.cdp('nav', url, timeout=30)
        end = time.time() + 60
        while b.eval(ready_expr(), timeout=15) is not True:
            if time.time() > end:
                raise CdpError(f'{build.label}: page did not reload')
            time.sleep(1)
        if b.eval(str(PAGE_JS)) != 'ok':
            raise CdpError(f'{build.label}: page.js did not install')
        b.eval("__pg.until(() => __pg.ready())", timeout=30)
        b.eval(f"__pg.clickPreset('talents-tab', {json.dumps(build.talents)})", timeout=30)
        settings = b.eval('__pg.exportSettings()', timeout=30)
        if not isinstance(settings, dict):
            raise CdpError(f'{build.label}: exportSettings did not return JSON: {str(settings)[:300]}')
        settings['player']['race'] = build.race
        settings['player']['professions'] = list(ALL_PROFESSIONS)
        return settings

    def apply_settings(self, settings, seed_items):
        settings['player']['equipment'] = {'items': seed_items}
        tmp = self.out / 'settings.tmp.json'
        tmp.write_text(json.dumps(settings), encoding='utf-8')
        self.browser.cdp('load', '__pgSettings', str(tmp), timeout=30)
        r = self.browser.eval('__pg.importSettings(window.__pgSettings)', timeout=30)
        if r != 'ok':
            raise CdpError(f'importSettings: {r}')

    def run_phase(self, build, phase, effort, seed_items, settings):
        self.apply_settings(settings, seed_items)
        setup = self.browser.eval(f"__pg.setupOptimizer({phase}, {json.dumps(effort)})", timeout=30)
        if str(setup.get('phase')) != str(phase):
            raise CdpError(f'{build.label} P{phase}: phase select did not take ({setup})')
        base_timeout = self.args.quick_timeout if effort == 'Quick' else self.args.normal_timeout
        js_timeout_ms = int(base_timeout * (1.6 if build.tank else 1.0) * 1000)
        self.browser.eval(f'__pg.runOptimizer({js_timeout_ms})', timeout=js_timeout_ms // 1000 + 30)
        result = self.browser.eval(f'__pg.exportResult({phase})', timeout=30)
        if not isinstance(result, dict) or not result.get('best'):
            raise CdpError(f'{build.label} P{phase}: no result ({str(result)[:300]})')
        return result

    def run_build(self, build):
        effort = self.args.effort
        phases = [p for p in self.args.phase_list if not self.done(build, p)]
        if not phases:
            self.log(f'{build.label}: all requested phases already in the summary at {effort}')
            return
        self.log(f'{build.label}: phases {phases} at {effort}')
        settings = None
        for phase in phases:
            if settings is None:
                settings = self.open_build(build)
            seed_items = self.seed_items(build, phase)
            t0 = time.time()
            result = self.run_phase(build, phase, effort, seed_items, settings)
            wall = time.time() - t0
            best = result['best']
            row = {
                'build': build.key, 'label': build.label, 'dir': build.dir, 'tree': build.tree, 'phase': phase,
                'effort': effort, 'race': build.race, 'scoreDelta': best.get('scoreDelta'),
                'scoreDeltaSe': best.get('scoreDeltaSe'), 'racialTraits': best.get('racialTraits'),
                'improved': result.get('improved'), 'warnings': result.get('warnings') or [],
                'simCommit': result.get('simCommit'), 'catalogDate': result.get('catalogDate'),
                'totalSims': result.get('totalSims'), 'elapsedSeconds': result.get('elapsedSeconds'),
                'wallSeconds': round(wall, 1), 'scoreStats': result.get('scoreStats'),
                'items': best['equipment']['items'],
            }
            if not self.args.no_write:
                path = write_gear(build, phase, row['items'])
                write_meta(build, phase, row)
                row['gearFile'] = path.relative_to(REPO).as_posix()
            self.summary.add(row)
            self.log(f'{build.label} P{phase} {effort}: {row["scoreDelta"] or 0:+.1f} ± {row["scoreDeltaSe"] or 0:.1f}, '
                     f'{row["racialTraits"]}, {wall:.0f}s' + (f", warnings {row['warnings']}" if row['warnings'] else ''))

    def run(self):
        self.browser.start()
        try:
            for build_key in self.args.build_list:
                build = BUILDS_BY_KEY[build_key]
                try:
                    self.run_build(build)
                except CdpError as e:
                    self.log(f'! {build.label}: {e}')
                    raise
        finally:
            self.browser.stop()


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument('command', choices=['run'])
    parser.add_argument('--out', required=True, help='summary.jsonl, driver.log and the Chrome profile')
    parser.add_argument('--effort', default='Quick', choices=['Quick', 'Normal'])
    parser.add_argument('--builds', default='all', help='comma-separated build keys, or all (default)')
    parser.add_argument('--phases', default='1-5', help='e.g. 1-5 or 1,5 (default 1-5)')
    parser.add_argument('--no-write', action='store_true',
                        help='a spot check: leave the gear files and bis_presets.json alone, only fill the summary')
    parser.add_argument('--base', default='http://localhost:3337', help='the sim server')
    parser.add_argument('--port', type=int, default=9348, help="Chrome's debugging port (default 9348)")
    parser.add_argument('--quick-timeout', type=int, default=150, help='seconds to wait for a Quick run '
                        '(a tank phase can run 60-150 s; one took 144 s and needed a rerun at a higher budget)')
    parser.add_argument('--normal-timeout', type=int, default=420, help='seconds to wait for a Normal run')
    args = parser.parse_args()

    sys.stdout.reconfigure(encoding='utf-8', errors='replace')
    args.build_list = parse_list(args.builds, [b.key for b in BUILDS], 'build')
    args.phase_list = parse_phases(args.phases)
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    log = Log(out / 'driver.log')
    if not server_ok(args.base):
        raise SystemExit(f'the sim server at {args.base} is not answering')
    log(f'run: ' + ' '.join(sys.argv[1:]))
    driver = Driver(args, log)
    driver.run()


if __name__ == '__main__':
    main()
