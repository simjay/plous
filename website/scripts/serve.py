"""Serve the static build at its configured Pages base path for local review."""
from functools import partial
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit
import argparse
import os

parser = argparse.ArgumentParser()
parser.add_argument('--port', type=int, default=8046)
args = parser.parse_args()
base = '/' + os.environ.get('PLOUS_SITE_BASE', '/plous').strip('/')
base = base.rstrip('/') + '/'
directory = Path(os.environ.get('PLOUS_SITE_DIST', Path(__file__).resolve().parents[1] / 'dist'))


class Handler(SimpleHTTPRequestHandler):
    def do_GET(self):
        if self.path == '/' and base != '/':
            self.send_response(302)
            self.send_header('Location', base)
            self.end_headers()
            return
        super().do_GET()

    def translate_path(self, path):
        pathname = urlsplit(path).path
        if base != '/' and pathname.startswith(base):
            pathname = '/' + pathname[len(base):]
        elif base != '/':
            return str(directory / '__outside_base__')
        return super().translate_path(pathname)


print(f'Preview: http://127.0.0.1:{args.port}{base}', flush=True)
ThreadingHTTPServer(('127.0.0.1', args.port), partial(Handler, directory=str(directory))).serve_forever()
