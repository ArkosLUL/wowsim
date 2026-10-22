"""Runs the BiS chain: sim servers, the BiS presets, the raid roster, the BiS tooltip dataset, then the stat
weights and DPS rankings. Every step resumes: rerun the same command with the same --work, which also keeps the
options of its first run.

    python tools/bischain/chain.py all --work G:/DevStuff/GitHub/.wave-loop/bis-chain-<date> \\
        --times 'G:/DevStuff/GitHub/.wave-loop/bis-chain-*/presets/*/summary.jsonl' \\
        --switches G:/DevStuff/GitHub/.wave-loop/bis-chain-switches.json

Commands: servers, presets, roster, dataset, assemble, weights, stop, all. See the README.
"""
import argparse
import glob
import json
import os
import re
import shlex
import shutil
import socket
import subprocess
import sys
import threading
import time
import traceback
import types
import urllib.request
from datetime import datetime, timezone
from pathlib import Path

sys.dont_write_bytecode = True
REPO = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO / 'tools' / 'presetgen'))
import gen  # noqa: E402

PY = sys.executable
# Windows looks in System32 before PATH, where bash.exe is WSL's
BASH = next((p for p in [shutil.which('bash')] if p and 'Git' in p), r'C:\Program Files\Git\bin\bash.exe')
# dock.sh hands AC_DSN to docker as -e AC_DSN=<value> when it's set, which would put the login on a command line
ENV = {k: v for k, v in os.environ.items() if k != 'AC_DSN'}
SOURCES_25 = 'raid25,raid25-heroic,vendor,crafted,quest,reputation,achievement,prospecting,world-drop'
# other sessions' and programs' ports, kept free even when nothing listens on them right now
RESERVED = {3333, 3334, 3346, 9346, 9350, 9361}
IMAGE = 'wowsims-wotlk-dev'
WOWSIM = 'G:/DevStuff/GitHub/.wowsim'
PLAYERBOT_ENV = 'G:/DevStuff/GitHub/azerothcore-wotlk-pb/configurationOverrides/Playerbot.env'
CLIENT_DATA = 'azerothcore-wotlk-pb_ac-client-data'
SERVE = './wowsimwotlk --usefs=true --launch=false --host=":3333"'
ACORE_FLAGS = (' --acoreDbcDir=/acdata/dbc --acoreNamesFile=/wowsim/core-raid.txt --acoreLeader=Deathsong'
               ' --acoreWorldBuffMatrix=/acconf/Playerbot.env')
AC_SLOT_TO_ITEM_SLOT = {15: 14, 16: 15, 17: 16}
DEFAULTS = {'builds': 'all', 'phases': '1-5', 'no_write': False, 'preset_servers': 2, 'times': [],
            'dataset_split': '1,2,3:4,5', 'dataset_effort': 'Normal', 'dataset_raiders': 'all', 'sources': SOURCES_25,
            'switches': None, 'rankings_dest': None, 'from_work': None, 'allow_party_mismatch': False}

_lock = threading.Lock()
_taken = set()


class Chain:
    def __init__(self, args):
        self.work = Path(args.work).resolve()
        self.name = self.work.name
        if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_.-]*', self.name):
            raise SystemExit(f'--work must end in a name docker accepts in a container name, not {self.name!r}')
        (self.work / 'logs').mkdir(parents=True, exist_ok=True)
        saved = self.work / 'args.json'
        opts = dict(DEFAULTS)
        if saved.exists():
            opts.update(json.loads(saved.read_text(encoding='utf-8')))
        opts.update({k: v for k, v in vars(args).items() if k not in ('command', 'work')})
        saved.write_text(json.dumps(opts, indent=1), encoding='utf-8')
        self.a = types.SimpleNamespace(**opts)
        self.failed = []

    def log(self, msg):
        line = f'{datetime.now():%Y-%m-%d %H:%M:%S} {msg}'
        with _lock:
            print(line, flush=True)
            with open(self.work / 'chain.log', 'a', encoding='utf-8') as f:
                f.write(line + '\n')

    def fail(self, step, msg):
        self.failed.append(step)
        self.log(f'FAILED {step}: {msg}')

    def guard(self, step, fn, *args):
        """Runs fn, turning any exception into a logged failure: a thread's exception would vanish otherwise."""
        try:
            return fn(*args)
        except BaseException as e:
            with open(self.work / 'logs' / 'errors.log', 'a', encoding='utf-8') as f:
                f.write(f'\n== {datetime.now():%H:%M:%S} {step}\n{traceback.format_exc()}')
            self.fail(step, f'{type(e).__name__}: {e} (traceback in logs/errors.log)')

    def thread(self, step, fn, *args):
        return threading.Thread(target=self.guard, args=(step, fn, *args))

    def run(self, cmd, logname):
        """Runs cmd from the checkout, output appended to logs/<logname>.log; returns the exit code."""
        with open(self.work / 'logs' / f'{logname}.log', 'a', encoding='utf-8') as out:
            out.write(f'\n== {datetime.now():%H:%M:%S} {" ".join(map(str, cmd))}\n')
            out.flush()
            return subprocess.run([str(c) for c in cmd], cwd=REPO, env=ENV, stdout=out, stderr=subprocess.STDOUT).returncode

    def git(self, *args):
        return subprocess.run(['git', '-C', str(REPO), *args], capture_output=True, text=True, check=True).stdout.strip()

    def selected_builds(self):
        return list(gen.BUILDS) if self.a.builds == 'all' else [gen.BUILDS_BY_KEY[k] for k in self.a.builds.split(',')]

    # servers

    def servers_file(self):
        return self.work / 'servers.json'

    def servers(self):
        return json.loads(self.servers_file().read_text(encoding='utf-8'))

    def start_servers(self):
        if self.git('status', '--porcelain', '--', 'sim', 'proto', 'assets'):
            self.log('warning: sim/, proto/ or assets/ has uncommitted changes, so the stat weights and rankings '
                     'will stamp -dirty')
        commit = self.git('rev-parse', 'HEAD')
        roles = [f'p{i + 1}' for i in range(self.a.preset_servers)]
        roles += [f'd{i + 1}' for i in range(len(self.dataset_batches()))]
        roles += [f's{i + 1}' for i in range(len(self.switches()))]
        known = self.servers() if self.servers_file().exists() else {}
        for s in known.values():
            _taken.update({s['port'], s['chrome']})
        servers, built = {}, False
        for role in roles:
            name = f'wotlk-chain-{self.name}-{role}'
            state = docker_state(name)
            if state == 'running' and role in known:
                servers[role] = known[role]
                continue
            if state:
                subprocess.run(['docker', 'rm', '-f', name], capture_output=True)
            port, chrome = free_port(3337), free_port(9348)
            build = not built and not any(docker_state(f'wotlk-chain-{self.name}-{r}') == 'running' for r in roles)
            sh = ('make dist/wotlk/.dirstamp devserver && ' if build else '') + SERVE
            cmd = ['docker', 'run', '-d', '--name', name, '-p', f'{port}:3333', '-e', f'SIM_COMMIT={commit}']
            if role == 'd1':
                # only d1 serves /acoreRoster, so only it gets the login
                cmd += ['--add-host=host.docker.internal:host-gateway', '-v', f'{CLIENT_DATA}:/acdata:ro',
                        '--mount', f'type=bind,source={WOWSIM},target=/wowsim,readonly',
                        '--mount', f'type=bind,source={PLAYERBOT_ENV},target=/acconf/Playerbot.env,readonly',
                        '--env-file', f'{WOWSIM}/prod.env']
                sh += ACORE_FLAGS
            cmd += mounts() + [IMAGE, 'sh', '-c', sh]
            r = subprocess.run(cmd, capture_output=True, text=True)
            if r.returncode != 0:
                raise RuntimeError(f'docker run {name}: {r.stderr.strip()}')
            servers[role] = {'name': name, 'port': port, 'chrome': chrome}
            self.log(f'server {name} on {port} (Chrome {chrome}){", building" if build else ""}')
            if build:
                built = True
                if not wait_http(f'http://localhost:{port}/wotlk/', 1200, name):
                    raise RuntimeError(f'{name} never came up: docker logs {name}')
        for s in servers.values():
            if not wait_http(f'http://localhost:{s["port"]}/wotlk/', 300, s['name']):
                raise RuntimeError(f'{s["name"]} never came up')
        self.servers_file().write_text(json.dumps(servers, indent=1), encoding='utf-8')
        self.log(f'servers up at {commit[:9]}: {", ".join(s["name"] for s in servers.values())}')

    def stop_servers(self, roles=None):
        if not self.servers_file().exists():
            return
        for role, s in self.servers().items():
            if roles is None or role in roles:
                subprocess.run(['docker', 'rm', '-f', s['name']], capture_output=True)
                self.log(f'removed {s["name"]}')

    def stop(self):
        self.stop_servers()
        # gen.py and the driver start Chrome detached, so a killed run leaves it holding its profile
        ps = ("Get-CimInstance Win32_Process -Filter \"Name='chrome.exe'\" | Where-Object { "
              f"($_.CommandLine -like '*\\{self.name}\\*' -or $_.CommandLine -like '*/{self.name}/*') -and "
              "$_.CommandLine -notlike '*--type=*' } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force; $_.ProcessId }")
        r = subprocess.run(['powershell', '-NoProfile', '-Command', ps], capture_output=True, text=True)
        ended = r.stdout.split()
        if ended:
            self.log(f'ended Chrome {", ".join(ended)}')

    # presets

    def presets(self):
        builds = self.selected_builds()
        groups = {}
        for b in builds:
            groups.setdefault(b.dir, []).append(b)
        cost = self.build_times()
        # longest first, so the slowest folders don't start last; a folder runs on one server at a time and keeps
        # its own --out, since two drivers writing one bis_presets.json would race and a rerun must find its summary
        queue = sorted(groups.values(), key=lambda g: -sum(cost(b) for b in g))
        slots = [r for r in self.servers() if r.startswith('p')]
        lock = threading.Lock()

        def worker(role):
            s = self.servers()[role]
            while True:
                with lock:
                    if not queue:
                        return
                    group = queue.pop(0)
                d = group[0].dir
                cmd = [PY, 'tools/presetgen/gen.py', 'run', '--out', (self.work / 'presets' / d).as_posix(),
                       '--effort', 'Normal', '--phases', self.a.phases, '--builds', ','.join(b.key for b in group),
                       '--normal-timeout', '1200', '--base', f'http://localhost:{s["port"]}', '--port', s['chrome']]
                if self.a.no_write:
                    cmd.append('--no-write')
                for attempt in (1, 2):
                    self.log(f'presets {d} on {role}{" (retry)" if attempt == 2 else ""}')
                    if self.run(cmd, f'presets-{role}') == 0:
                        break
                    # a cut-off search keeps the server busy until it notices nobody polls it, 2 min later
                    time.sleep(150)
                else:
                    self.fail('presets', f'{d} failed twice, see logs/presets-{role}.log')

        threads = [self.thread('presets', worker, r) for r in slots]
        for t in threads:
            t.start()
        for t in threads:
            t.join()
        self.check_presets(builds)

    def build_times(self):
        per_build = {}
        for pattern in self.a.times or []:
            for path in glob.glob(pattern):
                for line in Path(path).read_text(encoding='utf-8').splitlines():
                    if line.strip():
                        r = json.loads(line)
                        if r.get('effort') == 'Normal':
                            per_build[r['build']] = per_build.get(r['build'], 0) + r['wallSeconds']
        default = sorted(per_build.values())[len(per_build) // 2] if per_build else 1
        return lambda b: per_build.get(b.key, default * (2 if b.tank else 1))

    def check_presets(self, builds):
        rows, where, dups = {}, {}, []
        for path in sorted((self.work / 'presets').glob('*/summary.jsonl')):
            for line in path.read_text(encoding='utf-8').splitlines():
                if not line.strip():
                    continue
                r = json.loads(line)
                if r.get('effort') != 'Normal':
                    continue
                key = (r['build'], r['phase'])
                # a later row in the same summary is gen.py's own redo (a writing run over a --no-write row)
                if where.get(key, path) != path:
                    dups.append(key)
                rows[key], where[key] = r, path
        want = {(b.key, p) for b in builds for p in phase_list(self.a.phases)}
        got = {k: r for k, r in rows.items() if k in want}
        missing = sorted(want - set(got))
        commits = sorted({(r.get('simCommit') or '')[:9] for r in got.values()})
        flat = sorted(k for k, r in got.items() if (r.get('scoreDelta') or 0) <= 0)
        self.log(f'presets: {len(got)} of {len(want)} settled, sim {commits}, missing {missing}, '
                 f'in two summaries {dups}, no gain {flat}')
        if missing or dups or len(commits) > 1:
            self.fail('presets', 'see the line above')

    # roster and dataset

    def roster(self, fresh=True):
        path = self.work / 'raid.json'
        if path.exists() and not fresh:
            # the batch is keyed on the roster: a new export would start it over
            self.log(f'roster kept from {json.loads(path.read_bytes())["exportedAt"]}; `roster` exports a new one')
            return True
        d1 = self.servers()['d1']
        raw = urllib.request.urlopen(f'http://localhost:{d1["port"]}/acoreRoster', timeout=60).read()
        r = json.loads(raw)
        parties = {}
        for c in r['characters']:
            parties.setdefault(c['subgroup'] + 1, []).append(c['name'])
        parties = {p: sorted(n) for p, n in parties.items()}
        self.log(f'roster exported {r["exportedAt"]}, {len(r["characters"])} characters')
        for p in sorted(parties):
            self.log(f'  party {p}: {", ".join(parties[p])}')
        for c in r['characters']:
            for w in c.get('warnings') or []:
                if 'skill 450' not in w:
                    self.log(f'  ! {c["name"]}: {w}')
        live = live_group()
        problem = ('the live group could not be read' if live is None else 'Deathsong is in no group' if not live
                   else f'the live group differs: {live}' if live != parties else '')
        if problem and not self.a.allow_party_mismatch:
            self.fail('roster', f'{problem}; the parties decide Heroic Presence and party buffs, so check them, '
                                'then rerun with --allow-party-mismatch if the export is right')
            return False
        if problem:
            self.log(f'warning: {problem} (--allow-party-mismatch)')
        path.write_bytes(raw)
        return True

    def dataset_batches(self):
        return [b for b in self.a.dataset_split.split(':') if b]

    def switches(self):
        return json.loads(Path(self.a.switches).read_text(encoding='utf-8')) if self.a.switches else []

    def driver_cmd(self, role, roster, out, phases, stage, raiders='all', retry=False):
        s = self.servers()[role]
        cmd = [PY, 'tools/database/acbis/driver/driver.py', 'run', '--roster', roster.as_posix(), '--out', out.as_posix(),
               '--effort', self.a.dataset_effort, '--phases', phases, '--sources', self.a.sources, '--raiders', raiders,
               '--base', f'http://localhost:{s["port"]}', '--container', s['name'], '--port', s['chrome'],
               '--stage', stage]
        # a raid-DPS job goes quiet near the end of its Alternatives step, longer than the 1800 s default
        return cmd + (['--stall', '10800'] if stage == '2' else []) + (['--retry-failed'] if retry else [])

    def missing_jobs(self, out, phases, stage, raiders):
        """(raider, phase, stage) jobs this pass should have exported but didn't: a failed job counts as settled,
        and the export leaves it out."""
        grid = [l for l in (out / 'driver.log').read_text(encoding='utf-8').splitlines() if ' grid: ' in l]
        if not grid:
            return [('grid', 0, 0)]
        names = {p.split(' (')[0]: '(stage 2)' in p for p in grid[-1].split(' grid: ', 1)[1].split(', ')}
        chosen = {n: s2 for n, s2 in names.items() if raiders == 'all' or n in raiders.split(',')}
        export = out / f'batch-stage{stage}.json'
        have = set()
        if export.exists():
            have = {(e['raider'], e['contentPhase'], e['stage']) for e in json.loads(export.read_bytes())['entries']}
        want = {(n, p, 1) for n in chosen for p in phase_list(phases)}
        if stage == '2':
            want |= {(n, p, 2) for n, s2 in chosen.items() if s2 for p in phase_list(phases)}
        return sorted(want - have)

    def dataset_pass(self, role, roster, out, phases, stage, raiders, logname):
        self.log(f'{logname} phases {phases} stage {stage}')
        for retry in (False, True):
            if self.run(self.driver_cmd(role, roster, out, phases, stage, raiders, retry), logname) != 0:
                self.fail('dataset', f'{logname} stage {stage}, see logs/{logname}.log')
                return False
            missing = self.missing_jobs(out, phases, stage, raiders)
            if not missing:
                return True
            self.log(f'{logname} stage {stage} has no entry for {missing}{"" if retry else "; retrying failed jobs"}')
        self.fail('dataset', f'{logname} stage {stage} still misses {missing}')
        return False

    def dataset(self):
        roster = self.work / 'raid.json'
        if not roster.exists():
            raise RuntimeError('no raid.json: run `roster` first')
        jobs = []
        for i, phases in enumerate(self.dataset_batches()):
            role, out = f'd{i + 1}', self.work / 'bis' / f'd{i + 1}'

            def batch(role=role, out=out, phases=phases):
                for stage in ('1', '2'):
                    if not self.dataset_pass(role, roster, out, phases, stage, self.a.dataset_raiders, f'dataset-{role}'):
                        return
                self.log(f'dataset-{role} done')
            jobs.append(self.thread('dataset', batch))
        for i, sw in enumerate(self.switches()):
            jobs.append(self.thread('switch', self.switch_pass, sw, f's{i + 1}'))
        for t in jobs:
            t.start()
        for t in jobs:
            t.join()

    def switch_copy(self, sw):
        """The roster with the raider on the other build."""
        raider = sw['raider']
        roster = json.loads((self.work / 'raid.json').read_bytes())
        c = [c for c in roster['characters'] if c['name'] == raider]
        if len(c) != 1:
            raise RuntimeError(f'{raider} is not in the roster once')
        c = c[0]
        c['talents'] = sw['talents']
        swap = {int(k): v for k, v in sw.get('glyphs', {}).items()}
        missing = set(swap) - set(c['glyphs']['major'])
        if missing:
            raise RuntimeError(f'{raider} has no glyph {sorted(missing)} to swap')
        c['glyphs']['major'] = [swap.get(g, g) for g in c['glyphs']['major']]
        gear = {g['acSlot']: g for g in c['gear']}
        for slot, item in sw.get('gear', {}).items():
            gear[int(slot)] = {'acSlot': int(slot), 'gems': [], 'extraGem': 0, **item}
        for slot in sw.get('drop', []):
            gear.pop(slot, None)
        c['gear'] = [gear[k] for k in sorted(gear)]
        copy = self.work / f'raid-{raider}.json'
        copy.write_text(json.dumps(roster, indent=1), encoding='utf-8')
        return copy

    def switch_pass(self, sw, role):
        """A raider who changes builds by phase runs alone on a roster copy: the page's item filters are raid-wide."""
        raider = sw['raider']
        copy = self.switch_copy(sw)
        out = self.work / 'switch' / raider
        if sw.get('filters') and not (out / 'storage-snapshot.json').exists():
            self.log(f'switch {raider}: setting filters {sw["filters"]}')
            if not set_raid_filters(out, copy, self.servers()[role], sw['filters'], self.log):
                return self.fail('switch', f'{raider}: the page wrote its own filters back')
        if self.dataset_pass(role, copy, out, sw['phases'], '1', raider, f'switch-{raider}'):
            self.log(f'switch {raider} done')

    def assemble(self):
        done = self.work / 'assemble.done'
        if done.exists():
            return self.log(f'assemble already done; delete {done} to redo it')
        tmp = REPO / 'tmp' / self.name
        tmp.mkdir(parents=True, exist_ok=True)
        batches = [json.loads((self.work / 'bis' / f'd{i + 1}' / 'batch-stage2.json').read_bytes())
                   for i in range(len(self.dataset_batches()))]
        entries, seen = [], set()
        for b in batches:
            # one file holds a stage 2 raider's stage 1 entry too, and acbis takes the latest stage
            keys = {(e['raider'], e['contentPhase']) for e in b['entries']}
            if keys & seen:
                return self.fail('assemble', f'{sorted(keys & seen)} are in two batches')
            seen |= keys
            entries += b['entries']
        if self.a.dataset_raiders != 'all':
            if not self.a.from_work:
                return self.fail('assemble', 'a dataset narrowed by --dataset-raiders would replace the live one with '
                                             'those raiders only; pass --from-work to fill in everyone else')
            prev = REPO / 'tmp' / Path(self.a.from_work).name / 'batch.json'
            if not prev.exists():
                return self.fail('assemble', f'no {prev}: --from-work must name a run the chain assembled')
            old = [e for e in json.loads(prev.read_bytes())['entries'] if (e['raider'], e['contentPhase']) not in seen]
            self.log(f'{len(old)} entries kept from {prev}')
            entries += old
        main_entries = list(entries)
        roster = json.loads((self.work / 'raid.json').read_bytes())
        switch_entries = []
        for sw in self.switches():
            raider, phases = sw['raider'], phase_list(sw['phases'])
            batch = json.loads((self.work / 'switch' / raider / 'batch-stage1.json').read_bytes())
            new = [e for e in batch['entries'] if e['raider'] == raider and e['contentPhase'] in phases]
            if sorted(e['contentPhase'] for e in new) != sorted(phases):
                return self.fail('assemble', f'{raider}: the switch pass has phases {[e["contentPhase"] for e in new]}')
            for e in new:
                items = e['result']['best']['equipment']['items']
                for slot in sw.get('drop', []):
                    i = AC_SLOT_TO_ITEM_SLOT.get(slot)
                    if i is not None and i < len(items) and items[i].get('id'):
                        return self.fail('assemble', f'{raider} P{e["contentPhase"]} wears {items[i]["id"]} in '
                                                     f'emptied slot {slot}')
            (tmp / f'switch-{raider}.json').write_text(json.dumps(dict(batch, entries=new), indent=1), encoding='utf-8')
            shutil.copyfile(self.work / f'raid-{raider}.json', tmp / f'raid-{raider}.json')
            if self.run([PY, 'tools/database/acbis/driver/check_sources.py', tmp / f'switch-{raider}.json',
                         tmp / f'raid-{raider}.json'], 'assemble') != 0:
                return self.fail('assemble', f'{raider}: check_sources found picks outside the kept sources')
            main_entries = [e for e in main_entries if not (e['raider'] == raider and e['contentPhase'] in phases)]
            switch_entries += new
            self.log(f'{raider}: phases {phases} from the switch pass, scores '
                     f'{[round(e["result"]["best"]["score"]) for e in new]}')
            party = [c['subgroup'] + 1 for c in roster['characters'] if c['name'] == raider][0]
            hp = set()
            for log in (self.work / 'bis').glob('*/driver.log'):
                for line in log.read_text(encoding='utf-8').splitlines():
                    m = re.search(rf'(P[{"".join(map(str, phases))}] Heroic Presence, party {party} .*)', line)
                    if m:
                        hp.add(m.group(1)[:160])
            for line in sorted(hp):
                self.log(f'  warning: a Heroic Presence check in {raider}\'s party ran on the other build: {line}')
        merged = main_entries + switch_entries
        sims = sorted({str(e['result'].get('simCommit'))[:9] for e in merged})
        catalogs = sorted({str(e['result'].get('catalogDate')) for e in merged})
        # entries kept from --from-work ran on an older sim, which acbis allows; a pool needs one catalog
        if len(catalogs) != 1 or (len(sims) != 1 and not self.a.from_work):
            return self.fail('assemble', f'the entries ran on sims {sims} and catalogs {catalogs}')
        out = dict(batches[0], entries=merged, exportedAt=datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%S.000Z'))
        (tmp / 'batch-main.json').write_text(json.dumps(dict(out, entries=main_entries), indent=1), encoding='utf-8')
        (tmp / 'batch.json').write_text(json.dumps(out, indent=1), encoding='utf-8')
        shutil.copyfile(self.work / 'raid.json', tmp / 'raid.json')
        self.log(f'merged batch: {len(merged)} entries, sims {sims}, catalog {catalogs[0]}')
        if self.run([PY, 'tools/database/acbis/driver/check_sources.py', tmp / 'batch-main.json', tmp / 'raid.json'],
                    'assemble') != 0:
            return self.fail('assemble', 'check_sources found picks outside the kept sources, see logs/assemble.log')
        rel = shlex.quote(f'tmp/{self.name}')
        acbis_log = self.work / 'logs' / 'acbis.log'
        acbis_log.unlink(missing_ok=True)
        # acbis only takes -dsn, so the login comes from prod.env inside the container and never hits a command line
        code = self.run(['docker', 'run', '--rm', '--add-host=host.docker.internal:host-gateway',
                         '--env-file', f'{WOWSIM}/prod.env', *mounts(), IMAGE, 'sh', '-c',
                         f'go run ./tools/database/acbis -dsn "$AC_DSN" -batch {rel}/batch.json -roster {rel}/raid.json '
                         f'-out {rel}/bis_dataset.sql -luaOut {rel}/bis_dataset.lua'], 'acbis')
        for line in acbis_log.read_text(encoding='utf-8').splitlines():
            if line.startswith('!') or re.match(r'(dataset [0-9a-f]{8}:|\d+ subjects,)', line):
                self.log(f'  acbis: {line}')
        if code != 0:
            return self.fail('assemble', 'acbis failed, see logs/acbis.log')
        done.write_text(datetime.now().isoformat(), encoding='utf-8')
        self.log(f'dataset written to tmp/{self.name}/bis_dataset.sql. Importing it replaces the live dataset and needs '
                 "the user's OK: docker exec -i ac-database sh -c 'mysql -uroot -p\"$MYSQL_ROOT_PASSWORD\" acore_world' "
                 f'< tmp/{self.name}/bis_dataset.sql')

    # stat weights and rankings

    def wait_for_commit(self):
        if self.a.no_write or not self.git('status', '--porcelain', '--', 'ui'):
            return
        self.log('presets written: check and commit ui/*/gear_sets; the chain goes on once ui/ is clean')
        while self.git('status', '--porcelain', '--', 'ui'):
            time.sleep(30)
        self.log(f'ui/ clean at {self.git("rev-parse", "--short", "HEAD")}')

    def weights(self):
        done = self.work / 'weights.done'
        if done.exists():
            return self.log(f'weights already done; delete {done} to redo them')
        p1 = self.servers()['p1']
        sw, rk = f'tmp/statweights-{self.name}', f'tmp/rankings-{self.name}'
        sel = ['--builds', self.a.builds, '--phases', self.a.phases]
        narrowed = ({b.key for b in self.selected_builds()} != {b.key for b in gen.BUILDS}
                    or phase_list(self.a.phases) != [1, 2, 3, 4, 5])
        if self.a.from_work and not (REPO / sw / 'settings').exists():
            # a narrowed run starts from a full one's captures and results, minus the builds it recaptures;
            # weights.json and rankings.json then still cover every build
            prev = REPO / 'tmp' / f'statweights-{Path(self.a.from_work).name}'
            if not (prev / 'settings').exists():
                return self.fail('weights', f'no {prev}/settings: --from-work must name a run the chain finished')
            for d in ('settings', 'results'):
                shutil.copytree(prev / d, REPO / sw / d, dirs_exist_ok=True)
            for b in self.selected_builds():
                for ph in phase_list(self.a.phases):
                    (REPO / sw / 'settings' / f'{b.key}_p{ph}.bin').unlink(missing_ok=True)
        full = [] if self.a.from_work else sel
        steps = [
            ('bundle', [BASH, 'tools/acore/dock.sh', 'exec', 'sh', '-c',
                        'rm -f dist/wotlk/bundle/.dirstamp && make dist/wotlk/.dirstamp']),
            ('capture', [PY, 'tools/statweights/capture.py', '--out', sw, *sel,
                         '--base', f'http://localhost:{p1["port"]}', '--port', free_port(9362)]),
            ('statweights', [BASH, 'tools/acore/dock.sh', 'run', '--tags=with_db', './tools/statweights', '--out', sw, *full]),
            ('statweights merge', [PY, 'tools/statweights/merge.py', '--out', sw, *full]),
        ]
        for step, cmd in steps:
            self.log(f'weights: {step}')
            if self.run(cmd, 'weights') != 0:
                return self.fail('weights', f'{step}, see logs/weights.log')
        if narrowed and not self.a.from_work:
            done.write_text(datetime.now().isoformat(), encoding='utf-8')
            return self.log(f'stat weights in {sw}/weights.json for the chosen builds only; rankings skipped, '
                            'since rankings.json covers every build and phase (use --from-work)')
        (REPO / rk).mkdir(parents=True, exist_ok=True)
        shutil.copytree(REPO / sw / 'settings', REPO / rk / 'settings', dirs_exist_ok=True)
        dest = ['--dest', self.a.rankings_dest] if self.a.rankings_dest else []
        # a test run, which writes its rankings elsewhere, keeps its stat values out of assets/ too
        values = self.a.rankings_dest and f'{sw}/stat_values.json'
        values_dest = ['--dest', values] if values else []
        # publish goes last: rankings' merge would read its new file under assets/ as a dirty sim
        for step, cmd in [('rankings', [BASH, 'tools/acore/dock.sh', 'run', '--tags=with_db', './tools/rankings',
                                        '--out', rk]),
                          ('rankings merge', [PY, 'tools/rankings/merge.py', '--out', rk, *dest]),
                          ('statweights publish', [PY, 'tools/statweights/publish.py', '--out', sw, *values_dest])]:
            self.log(f'weights: {step}')
            if self.run(cmd, 'weights') != 0:
                return self.fail('weights', f'{step}, see logs/weights.log')
        done.write_text(datetime.now().isoformat(), encoding='utf-8')
        self.log(f'stat weights in {sw}/weights.json and {values or "assets/statweights/stat_values.json"}; '
                 f'rankings in {self.a.rankings_dest or "assets/rankings/rankings.json"}')

    def all(self):
        self.start_servers()
        roster_ok = self.guard('roster', self.roster, False)
        presets_roles = [r for r in self.servers() if r.startswith('p')]

        def presets_then_weights():
            try:
                self.presets()
                if 'presets' not in self.failed:
                    self.wait_for_commit()
                    self.weights()
            finally:
                self.stop_servers(presets_roles)

        def dataset_then_assemble():
            try:
                if not roster_ok:
                    return self.log('dataset skipped: no roster')
                self.dataset()
                if not {'dataset', 'switch'} & set(self.failed):
                    self.assemble()
            finally:
                self.stop_servers([r for r in self.servers() if r not in presets_roles])

        threads = [self.thread('presets', presets_then_weights), self.thread('dataset', dataset_then_assemble)]
        for t in threads:
            t.start()
        for t in threads:
            t.join()


def phase_list(spec):
    out = []
    for part in str(spec).split(','):
        lo, _, hi = part.partition('-')
        out += range(int(lo), int(hi or lo) + 1)
    return out


def mounts():
    return ['-v', f'{REPO.as_posix()}:/wotlk', '-v', 'wotlk-gomod:/go/pkg/mod', '-v', 'wotlk-gocache:/root/.cache/go-build',
            '-w', '/wotlk']


def docker_state(name):
    r = subprocess.run(['docker', 'inspect', '-f', '{{.State.Status}}', name], capture_output=True, text=True)
    return r.stdout.strip() if r.returncode == 0 else ''


def free_port(start):
    """The first port from start up that nothing listens on, on any address, and this run hasn't handed out."""
    with _lock:
        for p in range(start, start + 200):
            if p in RESERVED or p in _taken:
                continue
            ok = True
            for host in ('0.0.0.0', '127.0.0.1'):
                with socket.socket() as s:
                    try:
                        s.bind((host, p))
                    except OSError:
                        ok = False
            if ok:
                _taken.add(p)
                return p
    raise RuntimeError(f'no free port from {start}')


def wait_http(url, timeout, container):
    end = time.time() + timeout
    while time.time() < end:
        try:
            with urllib.request.urlopen(url, timeout=5) as r:
                if r.status == 200:
                    return True
        except OSError:
            pass
        if docker_state(container) not in ('running', 'created'):
            return False
        time.sleep(3)
    return False


def live_group():
    """Deathsong's group as {party: [names]} from the live DB; {} when he's in none, None when it can't be read."""
    q = ('SELECT gm.subgroup+1, c.name FROM group_member gm JOIN characters c ON c.guid=gm.memberGuid WHERE gm.guid='
         '(SELECT g.guid FROM group_member g JOIN characters d ON d.guid=g.memberGuid WHERE d.name=\\"Deathsong\\")')
    r = subprocess.run(['docker', 'exec', 'ac-database', 'sh', '-c',
                        f'mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -B -N acore_characters -e "{q}" 2>/dev/null'],
                       capture_output=True, text=True)
    if r.returncode != 0:
        return None
    parties = {}
    for line in r.stdout.splitlines():
        p, n = line.split('\t')
        parties.setdefault(int(p), []).append(n)
    return {p: sorted(n) for p, n in parties.items()}


def set_raid_filters(out, roster, server, filters, log):
    """Imports the roster on out's profile, lays filters over the raid sim's item filters and saves the snapshot
    the acbis driver restores on every load."""
    sys.path.insert(0, str(REPO / 'tools' / 'database' / 'acbis' / 'driver'))
    import driver as D
    key = '__wotlk_raid____currentSettings__'
    edit = ("(() => { const s = JSON.parse(localStorage.getItem(%r)); "
            "s.settings.filters = Object.assign(s.settings.filters || {}, %s); "
            "localStorage.setItem(%r, JSON.stringify(s)); return true; })()") % (key, json.dumps(filters), key)
    read = 'JSON.stringify(JSON.parse(localStorage.getItem(%r)).settings.filters)' % key
    out.mkdir(parents=True, exist_ok=True)
    args = types.SimpleNamespace(out=str(out), roster=str(roster), base=f'http://localhost:{server["port"]}',
                                 port=server['chrome'], server_wait=600, sources='all', container=server['name'])
    dlog = D.Log(out / 'driver.log')
    drv = D.Driver(args, dlog)
    try:
        drv.recover('opening the raid sim for the filters', drv.import_roster)
        time.sleep(5)
        drv.browser.eval(edit)
        time.sleep(5)
        after = drv.browser.eval(read)
        log(f'  filters now {after}')
        if any(after.get(k) != v for k, v in filters.items()):
            return False
        return bool(drv.snapshot())
    finally:
        drv.browser.end()
        dlog.file.close()


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter,
                                argument_default=argparse.SUPPRESS)
    p.add_argument('command', choices=['servers', 'presets', 'roster', 'dataset', 'assemble', 'weights', 'stop', 'all'])
    p.add_argument('--work', required=True, help='work directory outside the repo; its name also names the containers '
                                                 'and tmp/ dirs. Options are saved there and reused')
    p.add_argument('--builds', help="presetgen BUILDS keys for presets and weights (default all)")
    p.add_argument('--phases', help='content phases for presets and weights (default 1-5)')
    p.add_argument('--no-write', action='store_true', help="presets only write their summaries")
    p.add_argument('--preset-servers', type=int, help='default 2')
    p.add_argument('--times', action='append', help="glob of earlier presetgen summary.jsonl files, to start the "
                                                    "slowest folders first; repeatable")
    p.add_argument('--dataset-split', help="phases per dataset batch, one server each (default '1,2,3:4,5')")
    p.add_argument('--dataset-effort', help='default Normal')
    p.add_argument('--dataset-raiders', help='default all; anything else needs --from-work')
    p.add_argument('--sources', help='item sources the dataset keeps (default: 25-man)')
    p.add_argument('--switches', help='JSON list of raiders whose build changes by phase (README)')
    p.add_argument('--rankings-dest', help='rankings file to write instead of assets/rankings/rankings.json')
    p.add_argument('--from-work', help="an earlier full run's --work: a narrowed run's stat weights, rankings and "
                                       "dataset start from its outputs")
    p.add_argument('--allow-party-mismatch', action='store_true', help="go on when the live group can't be read or "
                                                                       "differs from the export")
    args = p.parse_args()
    c = Chain(args)
    if args.command == 'servers':
        c.guard('servers', c.start_servers)
    elif args.command == 'roster':
        c.guard('roster', c.roster, True)
    else:
        c.guard(args.command, getattr(c, args.command))
    if c.failed:
        c.log(f'{args.command} done with failures: {sorted(set(c.failed))}')
        sys.exit(1)
    c.log(f'{args.command} done')


if __name__ == '__main__':
    main()
