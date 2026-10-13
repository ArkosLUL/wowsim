"""Saves the optimizer requests a sim page posts, for sim/optimizer's TestMeasureTank* tests.

Stands between a browser and a sim server. Everything goes through but /optimizeGearAsync: its body
lands in <out dir>/request-<n>.bin (an OptimizeGearRequest, as the page built it) and the page gets
an empty answer, so its run fails at once and the server never starts one.

    python capture_requests.py <out dir> [--listen 3348] [--server http://localhost:3338]

Then drive the page through http://localhost:<listen>, by hand or with the acbis driver's --base.
The requests hold the roster: never commit them. The tests run in the toolchain container, which
only sees the checkout, so the out dir goes under its gitignored tmp/ and is deleted afterwards.
"""
import argparse
import http.server
import pathlib
import threading
import urllib.error
import urllib.request

# hop-by-hop, or wrong once the body is relayed in one piece
SKIPPED_HEADERS = {'connection', 'keep-alive', 'transfer-encoding', 'content-length', 'host'}


class Handler(http.server.BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'
    server_url = ''
    out_dir = pathlib.Path('.')
    count = 0
    lock = threading.Lock()

    def answer(self, status, headers, body):
        self.send_response(status)
        for name, value in headers:
            if name.lower() not in SKIPPED_HEADERS:
                self.send_header(name, value)
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        if self.command != 'HEAD':
            self.wfile.write(body)

    def relay(self):
        body = self.rfile.read(int(self.headers.get('Content-Length') or 0))
        if self.path.endswith('/optimizeGearAsync'):
            with Handler.lock:
                Handler.count += 1
                number = Handler.count
            path = self.out_dir / f'request-{number:03d}.bin'
            path.write_bytes(body)
            print(f'saved {path.name}, {len(body)} bytes', flush=True)
            self.answer(200, [], b'')
            return
        if self.path.endswith('/asyncProgress'):
            self.answer(204, [], b'')
            return
        headers = {name: value for name, value in self.headers.items() if name.lower() not in SKIPPED_HEADERS}
        request = urllib.request.Request(self.server_url + self.path, data=body or None, headers=headers, method=self.command)
        try:
            with urllib.request.urlopen(request, timeout=300) as response:
                self.answer(response.status, response.getheaders(), response.read())
        except urllib.error.HTTPError as e:
            self.answer(e.code, e.headers.items(), e.read())
        except OSError as e:
            self.answer(502, [], str(e).encode())

    do_GET = do_POST = do_HEAD = relay

    def log_message(self, format, *args):
        pass


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument('out')
    parser.add_argument('--listen', type=int, default=3348)
    parser.add_argument('--server', default='http://localhost:3338')
    args = parser.parse_args()
    Handler.server_url = args.server.rstrip('/')
    Handler.out_dir = pathlib.Path(args.out)
    Handler.out_dir.mkdir(parents=True, exist_ok=True)
    print(f'http://localhost:{args.listen} relays to {Handler.server_url}; requests go to {Handler.out_dir}', flush=True)
    http.server.ThreadingHTTPServer(('127.0.0.1', args.listen), Handler).serve_forever()


if __name__ == '__main__':
    main()
