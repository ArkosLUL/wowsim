"""Tiny Chrome DevTools driver for the manual check: nav, eval (expr or .js file), shot, size."""
import asyncio
import base64
import json
import sys
import urllib.request

import websockets

PORT = 9333


def page_ws():
    for t in json.load(urllib.request.urlopen(f'http://127.0.0.1:{PORT}/json')):
        if t['type'] == 'page':
            return t['webSocketDebuggerUrl']
    raise SystemExit('no page')


async def call(ws, method, params=None, _id=[0]):
    _id[0] += 1
    my = _id[0]
    await ws.send(json.dumps({'id': my, 'method': method, 'params': params or {}}))
    while True:
        msg = json.loads(await ws.recv())
        if msg.get('id') == my:
            if 'error' in msg:
                raise RuntimeError(msg['error'])
            return msg['result']


async def main():
    cmd = sys.argv[1]
    async with websockets.connect(page_ws(), max_size=None) as ws:
        if cmd == 'nav':
            await call(ws, 'Page.navigate', {'url': sys.argv[2]})
        elif cmd == 'eval':
            arg = sys.argv[2]
            expr = open(arg, encoding='utf-8').read() if arg.endswith('.js') else arg
            r = await call(ws, 'Runtime.evaluate', {'expression': expr, 'awaitPromise': True, 'returnByValue': True})
            if 'exceptionDetails' in r:
                print('EXCEPTION', json.dumps(r['exceptionDetails'], indent=1)[:2000])
            else:
                v = r.get('result', {}).get('value')
                print(v if isinstance(v, str) else json.dumps(v, indent=1))
        elif cmd == 'shot':
            r = await call(ws, 'Page.captureScreenshot', {'format': 'png', 'captureBeyondViewport': False})
            open(sys.argv[2], 'wb').write(base64.b64decode(r['data']))
        elif cmd == 'size':
            await call(ws, 'Emulation.setDeviceMetricsOverride',
                       {'width': int(sys.argv[2]), 'height': int(sys.argv[3]), 'deviceScaleFactor': 1, 'mobile': False})


asyncio.run(main())
