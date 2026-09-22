"""Runs the raid sim's BiS Batch in headless Chrome for acbis's -batch input.

One pass runs one stage for the chosen raiders, a content phase at a time, and exports the batch to
batch-stage<N>.json after each phase. Stage 2 runs on top of a finished stage 1 in the same batch
(the Chrome profile under --out keeps it), so its export carries both stages.

    python driver.py run --roster raid.json --out DIR --stage 1 [--raiders all] [--phases 1-5]
    python driver.py stop --out DIR
"""
import argparse
import asyncio
import json
import os
import re
import subprocess
import sys
import time
import urllib.request
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[3]
CDP = REPO / 'tools' / 'uicheck' / 'cdp.py'
RAID_JS = REPO / 'tools' / 'uicheck' / 'raid.js'
PAGE_JS = HERE / 'page.js'
CHROME = r'C:\Program Files\Google\Chrome\Application\chrome.exe'
EFFORTS = {'Quick': 0, 'Normal': 1, 'Thorough': 2}
TOOLCHAIN_IMAGES = ('wowsims-wotlk-dev', 'wowsim-toolchain', 'wotlk-toolchain')
# Chrome's localStorage quota per origin, measured: 5.2M chars fit, 5.3M throw
STORAGE_CHARS = 5_242_880


class CdpError(Exception):
    pass


def powershell(script, timeout=60):
    try:
        return subprocess.run(['powershell', '-NoProfile', '-Command', script], capture_output=True, text=True,
                              errors='replace', timeout=timeout).stdout
    except (OSError, subprocess.TimeoutExpired):
        return ''


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

    def version(self):
        try:
            return json.load(urllib.request.urlopen(f'http://127.0.0.1:{self.port}/json/version', timeout=3))
        except (OSError, ValueError):
            return None

    def alive(self):
        return self.version() is not None

    def on_profile(self, cmdline):
        return re.search(r'--user-data-dir="?' + re.escape(str(self.profile)) + r'"?(\s|$)', cmdline or '') is not None

    # only this driver runs Chrome on this profile, so a Chrome on the port with another one isn't ours
    def owned(self):
        return self.on_profile(powershell(
            f'$c = Get-NetTCPConnection -LocalPort {self.port} -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1; '
            'if ($c) { (Get-CimInstance Win32_Process -Filter "ProcessId=$($c.OwningProcess)").CommandLine }'))

    def start(self):
        if self.alive():
            if not self.owned():
                raise SystemExit(f"a Chrome this driver didn't start holds port {self.port}: pass another --port")
            self.log(f'reusing the Chrome on port {self.port}')
            return
        self.profile.mkdir(parents=True, exist_ok=True)
        flags = [
            '--headless=new', f'--remote-debugging-port={self.port}', f'--user-data-dir={self.profile}',
            '--window-size=1600,1100', '--no-first-run', '--no-default-browser-check',
            # the batch polls the server from this page for hours
            '--disable-background-timer-throttling', '--disable-renderer-backgrounding',
            '--disable-backgrounding-occluded-windows', 'about:blank',
        ]
        detached = getattr(subprocess, 'DETACHED_PROCESS', 0) | getattr(subprocess, 'CREATE_NEW_PROCESS_GROUP', 0)
        subprocess.Popen([CHROME, *flags], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, creationflags=detached)
        for _ in range(60):
            if self.alive():
                if not self.owned():
                    raise SystemExit(f"a Chrome this driver didn't start took port {self.port}: pass another --port")
                self.log(f'started Chrome on port {self.port}, profile {self.profile}')
                return
            time.sleep(0.5)
        raise CdpError(f'Chrome did not come up on port {self.port}')

    def stop(self):
        if not self.alive():
            return
        if not self.owned():
            self.log(f"leaving the Chrome on port {self.port} alone: it doesn't run on {self.profile}")
            return
        import websockets

        async def close():
            info = self.version()
            async with websockets.connect(info['webSocketDebuggerUrl'], max_size=None) as ws:
                await ws.send(json.dumps({'id': 1, 'method': 'Browser.close'}))
                try:
                    await asyncio.wait_for(ws.recv(), 5)
                except Exception:
                    pass

        try:
            asyncio.run(close())
        except Exception as e:
            self.log(f'Browser.close failed: {e}')
        for _ in range(30):
            if not self.alive():
                self.log('stopped Chrome')
                return
            time.sleep(0.5)
        self.log('Chrome still answers after Browser.close')

    # for a Chrome that no longer answers: ends the processes on this profile
    def kill(self):
        listing = powershell("Get-CimInstance Win32_Process -Filter \"Name='chrome.exe'\" | "
                             'ForEach-Object { "$($_.ProcessId) $($_.CommandLine)" }')
        pids = [line.split(' ', 1)[0] for line in listing.splitlines() if self.on_profile(line)]
        if pids:
            powershell(f'Stop-Process -Force -ErrorAction SilentlyContinue -Id {",".join(pids)}')
            self.log(f'ended {len(pids)} Chrome processes on this profile')
            time.sleep(2)

    def end(self):
        self.stop()
        self.kill()

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
            raise CdpError(out[:1500])
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


# a reload costs the running job, so one slow answer on a busy machine doesn't count
def server_down(base):
    if server_ok(base):
        return False
    time.sleep(3)
    return not server_ok(base)


def wait_for_server(base, limit, log):
    if server_ok(base):
        return
    log(f'the sim server at {base} is down, waiting up to {limit} s')
    end = time.time() + limit
    while time.time() < end:
        time.sleep(15)
        if server_ok(base):
            log('the sim server is back')
            return
    raise SystemExit(f'the sim server at {base} stayed down')


def repo_head():
    try:
        return subprocess.run(['git', '-C', str(REPO), 'rev-parse', 'HEAD'], capture_output=True, text=True,
                              timeout=30).stdout.strip()
    except (OSError, subprocess.TimeoutExpired):
        return ''


def machine_load(own_container):
    load = {}
    try:
        load['cpu'] = float(powershell('(Get-CimInstance Win32_Processor | Measure-Object -Property LoadPercentage -Average).Average',
                                       timeout=30))
    except ValueError:
        pass
    try:
        out = subprocess.run(['docker', 'ps', '--format', '{{.Names}} {{.Image}}'], capture_output=True, text=True,
                             timeout=30).stdout
        load['toolchains'] = [line.split()[0] for line in out.splitlines()
                              if line.split()[-1] in TOOLCHAIN_IMAGES and line.split()[0] != own_container]
    except (OSError, subprocess.TimeoutExpired):
        pass
    return load


def parse_phases(text):
    phases = set()
    try:
        for part in text.split(','):
            if '-' in part:
                lo, hi = part.split('-')
                phases.update(range(int(lo), int(hi) + 1))
            else:
                phases.add(int(part))
    except ValueError:
        raise SystemExit(f'phases look like 1-5 or 2,4: {text}')
    if not phases or min(phases) < 1 or max(phases) > 5:
        raise SystemExit(f'phases must be within 1-5: {text}')
    return sorted(phases)


def settled(state, phase, wanted, failed_ok=True):
    ends = ('done', 'failed') if failed_ok else ('done',)
    done = {(j['raider'], j['stage']) for j in state['jobs'] if j['phase'] == phase and j['state'] in ends}
    return all(w in done for w in wanted)


class Driver:
    def __init__(self, args, log):
        self.args = args
        self.log = log
        self.out = Path(args.out).resolve()
        self.browser = Browser(args.port, self.out / 'chrome-profile', log)
        self.head = ''

    def state(self):
        return self.browser.eval('JSON.stringify(__bis.state())', timeout=60)

    def snapshot(self):
        try:
            text = self.browser.cdp('eval', '__bis.raidStorage()', timeout=120)
            json.loads(text)
        except (CdpError, ValueError) as e:
            self.log(f'! no storage snapshot this time: {e}')
            return False
        tmp = self.out / 'storage-snapshot.tmp'
        tmp.write_text(text, encoding='utf-8')
        tmp.replace(self.out / 'storage-snapshot.json')
        return True

    # on a light page of the same origin, so the raid page starts from the prepared storage
    def prepare_storage(self):
        self.browser.cdp('nav', f'{self.args.base}/wotlk/net_worker.js')
        end = time.time() + 60
        while self.browser.eval("document.readyState == 'complete' && location.pathname == '/wotlk/net_worker.js'",
                                timeout=30) is not True:
            if time.time() > end:
                raise CdpError('the page for preparing storage did not load')
            time.sleep(0.5)
        self.browser.eval(str(PAGE_JS))
        path = self.out / 'storage-snapshot.json'
        if path.exists():
            self.browser.cdp('load', '__snap', str(path), timeout=120)
        report = self.browser.eval("__bis.prepareStorage(window.__snap || '')", timeout=120)
        if report['restored']:
            self.log(f"put back {report['restored']} from {path.name}: the browser had lost them")
        if report['failed']:
            self.log(f"! couldn't write {report['failed']}: the page re-runs what it lost")

    def open_page(self):
        wait_for_server(self.args.base, self.args.server_wait, self.log)
        self.prepare_storage()
        self.browser.cdp('nav', f'{self.args.base}/wotlk/raid/')
        end = time.time() + 120
        while True:
            try:
                ready = self.browser.eval(
                    "document.readyState == 'complete' && location.pathname == '/wotlk/raid/' && "
                    "!!document.querySelector('#bis-batch-tab .optimizer-setup') && "
                    "performance.getEntriesByType('resource').some(e => e.name.endsWith('/database/db.json') && e.responseEnd > 0)",
                    timeout=30)
                if ready is True:
                    break
            except CdpError:
                pass
            if time.time() > end:
                raise CdpError('the raid page did not load')
            time.sleep(1)
        # the sim sets up and the server check answers a moment after db.json lands
        time.sleep(5)
        for script in (RAID_JS, PAGE_JS):
            if self.browser.eval(str(script)) != 'ok':
                raise CdpError(f"{script.name} didn't install")
        self.browser.eval("__r.tab('bis-batch-tab')")
        self.browser.eval('__bis.touch()')

    def import_roster(self):
        self.browser.cdp('load', '__roster', str(Path(self.args.roster).resolve()))
        report = self.browser.eval('__r.importRoster(__roster)', timeout=180)
        self.log(f'imported the roster: {report}')
        time.sleep(2)
        # a changed roster has its own batch, which the page only stores once something changes
        self.browser.eval('__bis.touch()')

    def raiders(self):
        raiders = self.browser.eval('JSON.stringify(__bis.raiders())')
        if not raiders:
            raise SystemExit('the BiS Batch grid has no raiders after the import')
        return raiders

    def prepare(self, names, phase):
        effort = self.browser.eval(f'__bis.setEffort({json.dumps(self.args.effort)})')
        self.browser.eval(f'__r.pick({json.dumps(names)}, [{phase}])', timeout=120)
        grid = self.browser.eval('JSON.stringify(__r.grid())')
        ticked = sorted(g['name'] for g in grid if g['ticked'])
        state = self.state()
        if ticked != sorted(names) or state['settings'].get('phases') != [phase] or \
                state['settings'].get('effort') != EFFORTS[self.args.effort]:
            raise CdpError(f'the batch setup did not take: ticked {ticked}, settings {state["settings"]}')
        if self.args.stage == 1:
            self.browser.eval(f'__bis.stopAfterStage1({phase}, {json.dumps(names)})')
        self.log(f'P{phase}: {len(names)} raiders ticked, effort {effort}')

    # the page doesn't resume a batch on its own (prepareStorage clears its running flag), so every
    # run starts here, set up for this phase
    def start(self, names, phase, retry=False):
        self.prepare(names, phase)
        end = time.time() + 60
        while True:
            state = self.state()
            if state['wasm']:
                raise CdpError('the page found no sim server')
            label = ''
            if retry and state['retry']:
                label = 'Run failed ones again'
            elif state['start'] and not state['start']['disabled']:
                label = state['start']['label']
            if label and self.browser.eval(f'__bis.click({json.dumps(label)})'):
                break
            if time.time() > end:
                raise CdpError(f"can't start the batch: {state['start']}, status {state['status']!r}")
            time.sleep(1)
        for _ in range(20):
            if self.state()['running']:
                self.log(f'P{phase}: clicked {label}')
                return
            time.sleep(1)
        raise CdpError(f"the batch didn't start: {self.state()['status']!r}")

    # Retries a hung page by starting over in a new Chrome: Chrome itself can be gone too (something
    # on this machine has killed it mid-run). The profile and the storage snapshot bring the batch back.
    def recover(self, what, step):
        for attempt in range(1, 6):
            try:
                if attempt > 1:
                    self.browser.end()
                self.browser.start()
                self.open_page()
                return step()
            except CdpError as e:
                self.log(f'{what}: attempt {attempt} failed: {e}')
                time.sleep(30)
        raise SystemExit(f'{what}: the page would not come back')

    def reload(self, names, phase, wanted, why):
        self.log(f'P{phase}: {why}, reloading the page')

        def resume():
            if not self.browser.eval('__r.rows().length'):
                self.import_roster()
            if not settled(self.state(), phase, wanted):
                self.start(names, phase)

        self.recover(f'P{phase} reload', resume)

    def check_head(self):
        head = repo_head()
        if head and self.head and head != self.head:
            self.log(f'! the checkout moved from {self.head[:12]} to {head[:12]} during the run: a restarted sim '
                     "server builds from it, but its results keep the container's SIM_COMMIT")
            self.head = head

    def export(self):
        try:
            text = self.browser.cdp('eval', "__bis.click('Export JSON') ? __bis.take('optimizer-batch.json') : null",
                                    timeout=180)
        except CdpError as e:
            self.log(f'! the export failed: {e}')
            return False
        if text in ('', 'null'):
            self.log('nothing to export yet')
            return True
        try:
            data = json.loads(text)
        except ValueError as e:
            self.log(f'! the export came back unreadable: {e}')
            return False
        entries = data.get('entries', [])
        # pass 1's file stays own metrics only, even when the batch has stage 2 runs by now
        later = [e for e in entries if e.get('stage', 0) > self.args.stage]
        if later:
            entries = data['entries'] = [e for e in entries if e.get('stage', 0) <= self.args.stage]
            text = json.dumps(data, indent=2)
            self.log(f'left {len(later)} stage 2 entries out of the stage 1 export')
        # one file per pass, so stage 2's export (both stages) never overwrites stage 1's
        path = self.out / f'batch-stage{self.args.stage}.json'
        tmp = path.with_suffix('.tmp')
        tmp.write_text(text, encoding='utf-8')
        tmp.replace(path)
        stages = {}
        for entry in entries:
            stage = entry.get('stage', 0)
            stages[stage] = stages.get(stage, 0) + 1
        commits = sorted({e.get('result', {}).get('simCommit', '') for e in entries})
        self.log(f'exported {len(entries)} entries {stages} to {path}, sim {commits}, catalog {data.get("catalogDate", "")}')
        if any(c in ('', 'unknown') for c in commits):
            self.log('! some results carry no sim commit: pass SIM_COMMIT to the sim container')
        # a job that's already done stays as it ran, whatever --effort this pass asks for
        efforts = {e.get('result', {}).get('settings', {}).get('effort', 'OptimizerEffortQuick') for e in entries}
        if len(efforts) > 1:
            self.log(f'! the export mixes efforts {sorted(efforts)}')
        return True

    def monitor(self, names, phase, wanted, timings):
        stall = self.args.stall
        state = self.state()
        seen = {(j['raider'], j['stage']) for j in state['jobs'] if j['phase'] == phase and j['state'] in ('done', 'failed')}
        snapped = None
        last_settle = last_change = time.time()
        last_status = None
        restarts = stalls = 0
        write_failed = False
        while True:
            if server_down(self.args.base):
                wait_for_server(self.args.base, self.args.server_wait, self.log)
                self.check_head()
                self.reload(names, phase, wanted, 'the server went away mid-run')
                last_change = time.time()
                continue
            try:
                state = self.state()
            except CdpError as e:
                self.reload(names, phase, wanted, f'the page stopped answering ({e})')
                last_change = time.time()
                continue
            now = time.time()
            for job in state['jobs']:
                key = (job['raider'], job['stage'])
                if job['phase'] != phase or job['state'] not in ('done', 'failed') or key in seen:
                    continue
                seen.add(key)
                restarts = stalls = 0
                wall = now - last_settle
                last_settle = now
                load = machine_load(self.args.container)
                record = {'at': time.strftime('%Y-%m-%dT%H:%M:%S'), 'raider': job['raider'], 'phase': phase,
                          'stage': job['stage'], 'state': job['state'], 'effort': self.args.effort,
                          'server_s': round(job['elapsed'], 1), 'wall_s': round(wall, 1), 'sims': job['sims'],
                          'error': job['error'], 'load': load}
                timings.write(json.dumps(record) + '\n')
                timings.flush()
                self.log(f"P{phase} stage {job['stage']} {job['raider']}: {job['state']}, {job['elapsed']:.0f} s on the "
                         f"server, {wall:.0f} s wall, {job['sims']} sims, load {load}, "
                         f"storage {100 * state['needChars'] / STORAGE_CHARS:.0f}%"
                         + (f", error {job['error']}" if job['error'] else ''))
            # a reload can lose jobs, so this follows the settled set, not just newly seen jobs
            done = {(j['raider'], j['phase'], j['stage'], j['state']) for j in state['jobs'] if j['state'] in ('done', 'failed')}
            if done != snapped and self.snapshot():
                snapped = done
            if state['writeFailed'] != write_failed:
                write_failed = state['writeFailed']
                if write_failed:
                    self.log(f"! the batch no longer fits in localStorage ({state['needChars']} chars): the exports "
                             'and storage-snapshot.json still have it all, but a reload re-runs every job since')
                else:
                    self.log('the batch fits in localStorage again')
            if settled(state, phase, wanted):
                if not state['running']:
                    return
                if self.args.stage == 1:
                    self.log(f'P{phase}: stage 1 is done but the batch runs on, stopping it')
                    try:
                        self.browser.eval('__bis.stop()')
                    except CdpError:
                        pass
            elif not state['running']:
                restarts += 1
                if restarts > 3:
                    raise SystemExit(f'P{phase}: the batch keeps stopping: {state["status"]!r}')
                if state['wasm']:
                    self.reload(names, phase, wanted, 'the page lost the sim server')
                    continue
                self.log(f'P{phase}: the batch stopped with runs left ({state["status"]!r}), starting it again')
                try:
                    self.start(names, phase)
                except CdpError as e:
                    self.reload(names, phase, wanted, f"it didn't start again ({e})")
                last_change = time.time()
            elif state['status'] != last_status:
                last_status = state['status']
                last_change = now
            elif now - last_change > stall:
                stalls += 1
                if stalls > 3:
                    raise SystemExit(f'P{phase}: no progress through 3 reloads ({last_status!r})')
                self.reload(names, phase, wanted, f'no progress for {stall} s ({last_status!r})')
                last_change = time.time()
            time.sleep(self.args.poll)

    def run(self):
        args = self.args
        self.head = repo_head()
        self.log(f'sim checkout at {self.head[:12] or "unknown"}')
        self.recover('opening the raid sim', self.import_roster)
        raiders = self.raiders()
        by_name = {r['name']: r for r in raiders}
        self.log('grid: ' + ', '.join(f"{r['name']} ({r['spec']})" for r in raiders))
        names = list(by_name) if args.raiders == 'all' else [n.strip() for n in args.raiders.split(',')]
        unknown = [n for n in names if n not in by_name]
        if unknown:
            raise SystemExit(f'not in the grid (healers sit out): {unknown}')
        dps = [n for n in names if not by_name[n]['tank']]
        pass_start = time.time()
        skipped = []
        exported = None
        with open(self.out / 'timings.jsonl', 'a', encoding='utf-8') as timings:
            for phase in parse_phases(args.phases):
                wanted = [(n, 1) for n in names] if args.stage == 1 else [(n, 2) for n in dps]
                state = self.state()
                if settled(state, phase, wanted, failed_ok=not args.retry_failed):
                    self.log(f'P{phase}: stage {args.stage} already done for these raiders')
                    continue
                if args.stage == 2 and not settled(state, phase, [(n, 1) for n in names]):
                    self.log(f'! P{phase}: stage 1 is not done for every raider here, skipping stage 2 '
                             '(run --stage 1 first, with the same roster file)')
                    skipped.append(phase)
                    continue
                phase_start = time.time()
                try:
                    self.start(names, phase, retry=args.retry_failed)
                except CdpError as e:
                    self.reload(names, phase, wanted, f"the batch didn't start ({e})")
                self.monitor(names, phase, wanted, timings)
                self.log(f'P{phase}: stage {args.stage} settled in {time.time() - phase_start:.0f} s')
                exported = self.export()
        if not exported and not self.export():
            raise SystemExit('the batch could not be exported: run the same command again')
        final = self.state()
        failed = [f"P{j['phase']} s{j['stage']} {j['raider']}" for j in final['jobs'] if j['state'] == 'failed']
        self.log(f'pass done in {time.time() - pass_start:.0f} s; batch {final["storedChars"]} chars, '
                 f'storage {100 * final["needChars"] / STORAGE_CHARS:.0f}%'
                 + (f'; failed: {failed}' if failed else '') + (f'; skipped phases {skipped}' if skipped else ''))


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument('command', choices=['run', 'stop'])
    parser.add_argument('--out', required=True, help='output directory: batch-stage<N>.json, driver.log, timings.jsonl and the Chrome profile holding the batch')
    parser.add_argument('--roster', help='acraid roster JSON (run)')
    parser.add_argument('--stage', type=int, choices=[1, 2], help='1: own metrics for every raider; 2: raid DPS for DPS raiders, over a finished stage 1 (run)')
    parser.add_argument('--raiders', default='all', help="comma-separated names, or all (default)")
    parser.add_argument('--phases', default='1-5', help='content phases, e.g. 1-5 or 2,4 (default 1-5)')
    parser.add_argument('--effort', default='Quick', choices=list(EFFORTS))
    parser.add_argument('--retry-failed', action='store_true', help="run this pass's failed jobs again, once")
    parser.add_argument('--base', default='http://localhost:3346', help='the sim server (default http://localhost:3346)')
    parser.add_argument('--container', default='wotlk-bisdata', help="the sim server's container, left out of the load log")
    parser.add_argument('--port', type=int, default=9346, help="Chrome's debugging port (default 9346)")
    parser.add_argument('--poll', type=float, default=5, help='seconds between checks')
    parser.add_argument('--stall', type=int, default=1800, help='reload after this many seconds without progress')
    parser.add_argument('--server-wait', type=int, default=1800, help='how long to wait for a sim server that went away')
    parser.add_argument('--keep-chrome', action='store_true', help="leave Chrome running when done")
    args = parser.parse_args()

    sys.stdout.reconfigure(encoding='utf-8', errors='replace')
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    log = Log(out / 'driver.log')
    driver = Driver(args, log)
    if args.command == 'stop':
        driver.browser.end()
        return
    if not args.roster or not args.stage:
        parser.error('run needs --roster and --stage')
    log(f'{args.command}: ' + ' '.join(sys.argv[1:]))
    try:
        driver.run()
    finally:
        if not args.keep_chrome:
            driver.browser.end()


if __name__ == '__main__':
    main()
