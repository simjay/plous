"""Local Kubernetes release storage and chart repository for native Helm tests."""
import base64
import gzip
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import io
import json
import tarfile
import threading
import time
from urllib.parse import parse_qs, urlparse


def deployment(name, replicas):
    return {'apiVersion': 'apps/v1', 'kind': 'Deployment',
            'metadata': {'name': name, 'namespace': 'apps'},
            'spec': {'replicas': replicas, 'selector': {'matchLabels': {'app': name}},
                     'template': {'metadata': {'labels': {'app': name}},
                                  'spec': {'containers': [{'name': 'app', 'image': 'example.invalid/demo:1'}]}}}}


def release_record(revision):
    release = {'name': 'shop', 'namespace': 'apps', 'version': revision,
               'chart': {'metadata': {'apiVersion': 'v2', 'name': 'demo', 'version': '1.0.0' if revision == 2 else '0.9.0'},
                         'values': {'replicas': 1}},
               'config': {'replicas': revision, 'password': 'fixture-sensitive'},
               'manifest': json.dumps(deployment('shop', revision)),
               'info': {'status': 'deployed' if revision == 2 else 'superseded',
                        'description': 'Fixture release', 'notes': 'Fixture notes',
                        'first_deployed': '2026-09-30T10:00:00Z', 'last_deployed': '2026-10-01T10:00:00Z'}, 'hooks': []}
    payload = base64.b64encode(base64.b64encode(gzip.compress(json.dumps(release).encode()))).decode()
    return {'apiVersion': 'v1', 'kind': 'Secret', 'type': 'helm.sh/release.v1',
            'metadata': {'name': f'sh.helm.release.v1.shop.v{revision}', 'namespace': 'apps',
                         'labels': {'name': 'shop', 'owner': 'helm', 'status': release['info']['status'], 'version': str(revision)}},
            'data': {'release': payload}}


def chart_archive(name):
    version = '1.0.0' if name == 'demo' else '1.2.3'
    stream = io.BytesIO()
    with tarfile.open(fileobj=stream, mode='w:gz') as archive:
        for path, text in {'Chart.yaml': f'apiVersion: v2\nname: {name}\nversion: {version}\ndescription: Fixture chart\n',
                           'values.yaml': 'replicas: 1\n',
                           'README.md': f'# Chart documentation: {name}\n\nFixture chart documentation\n\n## Configuration\n\n```yaml\nreplicas: 1\n```\n',
                           'templates/deployment.yaml': json.dumps(deployment('{{ .Release.Name }}', 1))}.items():
            data = text.encode()
            header = tarfile.TarInfo(name + '/' + path)
            header.size = len(data)
            archive.addfile(header, io.BytesIO(data))
    return stream.getvalue()


def proto_fields(data):
    """Decode the small Kubernetes Secret protobuf shape used by the SDK."""
    offset = 0
    def number():
        nonlocal offset
        value, shift = 0, 0
        while True:
            byte = data[offset]
            offset += 1
            value |= (byte & 127) << shift
            if byte < 128:
                return value
            shift += 7
    fields = {}
    while offset < len(data):
        tag = number()
        kind = tag & 7
        if kind == 0:
            value = number()
        elif kind == 2:
            length = number()
            value = data[offset:offset + length]
            offset += length
        else:
            raise AssertionError('Unexpected fixture protobuf wire type')
        fields.setdefault(tag >> 3, []).append(value)
    return fields


def decode_secret(data):
    envelope = proto_fields(data[4:])
    secret = proto_fields(envelope[2][0])
    metadata = proto_fields(secret[1][0])
    def pairs(entries, binary=False):
        result = {}
        for raw in entries:
            entry = proto_fields(raw)
            result[entry[1][0].decode()] = base64.b64encode(entry[2][0]).decode() if binary else entry[2][0].decode()
        return result
    return {'apiVersion': 'v1', 'kind': 'Secret', 'type': secret[3][0].decode(),
            'metadata': {'name': metadata[1][0].decode(), 'namespace': metadata[3][0].decode(),
                         'labels': pairs(metadata.get(11, []))},
            'data': pairs(secret[2], binary=True)}


class HelmAPI:
    def __init__(self, root):
        self.root = root
        self.namespace_denied = False
        self.namespace_requests = []
        self.requests = []
        self.metadata_requests = []
        self.records = {r['metadata']['name']: r for r in [release_record(1), release_record(2)]}
        self.objects = {'/apis/apps/v1/namespaces/apps/deployments/shop': deployment('shop', 2)}
        fixture = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_args):
                pass

            def respond(self, data, status=200, content_type='application/json'):
                self.send_response(status)
                self.send_header('Content-Type', content_type)
                self.end_headers()
                try:
                    self.wfile.write(data if isinstance(data, bytes) else json.dumps(data).encode())
                except (BrokenPipeError, ConnectionResetError):
                    pass

            def failure(self, status, message):
                self.respond({'apiVersion': 'v1', 'kind': 'Status', 'status': 'Failure', 'code': status,
                              'reason': 'NotFound' if status == 404 else 'Forbidden', 'message': message}, status)

            def do_GET(self):
                fixture.requests.append(self.path)
                url = urlparse(self.path)
                path = url.path
                if path == '/api/v1/namespaces':
                    fixture.namespace_requests.append(self.path)
                    if fixture.namespace_denied:
                        return self.failure(403, 'namespaces is forbidden')
                    return self.respond({'apiVersion': 'v1', 'kind': 'NamespaceList',
                                         'items': [{'metadata': {'name': name}} for name in ['apps', 'production']]})
                if '/namespaces/blocked/' in path:
                    return self.failure(403, 'Fixture namespace access denied')
                if path == '/api/v1/namespaces/apps/pods/shop':
                    return self.respond({'apiVersion': 'v1', 'kind': 'Pod', 'metadata': {'name': 'shop', 'namespace': 'apps'}, 'status': {'phase': 'Running'}})
                if path == '/apis/apiextensions.k8s.io/v1/customresourcedefinitions':
                    return self.respond({'apiVersion': 'apiextensions.k8s.io/v1', 'kind': 'CustomResourceDefinitionList', 'items': []})
                if path == '/version':
                    return self.respond({'major': '1', 'minor': '34', 'gitVersion': 'v1.34.0'})
                if path == '/api':
                    return self.respond({'apiVersion': 'v1', 'kind': 'APIVersions', 'versions': ['v1']})
                if path == '/apis':
                    return self.respond({'apiVersion': 'v1', 'kind': 'APIGroupList', 'groups': [
                        {'name': 'apps', 'versions': [{'groupVersion': 'apps/v1', 'version': 'v1'}],
                         'preferredVersion': {'groupVersion': 'apps/v1', 'version': 'v1'}}]})
                if path in ('/api/v1', '/apis/apps/v1'):
                    kinds = [('secrets', 'Secret'), ('configmaps', 'ConfigMap'), ('namespaces', 'Namespace'), ('pods', 'Pod')] if path == '/api/v1' else [('deployments', 'Deployment')]
                    return self.respond({'kind': 'APIResourceList', 'apiVersion': 'v1',
                                         'groupVersion': 'v1' if path == '/api/v1' else 'apps/v1',
                                         'resources': [{'name': name, 'kind': kind, 'namespaced': name != 'namespaces',
                                                        'verbs': ['get', 'list', 'create', 'update', 'patch', 'delete']} for name, kind in kinds]})
                if path == '/openapi/v3':
                    return self.respond({'paths': {}})
                if path == '/openapi/v2':
                    return self.respond(base64.b64decode('CgMyLjASDQoHRml4dHVyZRICdjFCAEoA'), content_type='application/octet-stream')
                if path.endswith('/secrets'):
                    labels = parse_qs(url.query).get('labelSelector', [''])[0].split(',')
                    items = list(fixture.records.values()) if '/production/' not in path else []
                    for selector in labels:
                        if '=' in selector:
                            key, value = selector.split('=', 1)
                            items = [item for item in items if item['metadata']['labels'].get(key) == value]
                    if 'as=PartialObjectMetadataList' in self.headers.get('Accept', ''):
                        fixture.metadata_requests.append(self.path)
                        items = [{'metadata': item['metadata']} for item in items[:1]]
                        return self.respond({'apiVersion': 'meta.k8s.io/v1', 'kind': 'PartialObjectMetadataList', 'items': items})
                    return self.respond({'apiVersion': 'v1', 'kind': 'SecretList', 'items': items})
                if path.endswith('/configmaps'):
                    return self.respond({'apiVersion': 'meta.k8s.io/v1', 'kind': 'PartialObjectMetadataList', 'items': []})
                if '/secrets/' in path:
                    record = fixture.records.get(path.rsplit('/', 1)[-1])
                    return self.respond(record) if record else self.failure(404, 'release not found')
                if path in fixture.objects:
                    return self.respond(fixture.objects[path])
                if path == '/index.yaml':
                    entries = {name: [{'apiVersion': 'v2', 'name': name, 'version': '1.0.0' if name == 'demo' else '1.2.3',
                                       'description': 'Fixture chart', 'urls': [name + '.tgz']}] for name in ['demo'] + [f'chart-{i:02d}' for i in range(24)]}
                    return self.respond({'apiVersion': 'v1', 'entries': entries})
                if path.endswith('.tgz'):
                    return self.respond(chart_archive(path[1:-4]), content_type='application/gzip')
                self.failure(404, 'fixture endpoint unavailable: ' + path)

            def mutate(self):
                path = urlparse(self.path).path
                data = self.rfile.read(int(self.headers.get('Content-Length', 0)))
                body = decode_secret(data) if data.startswith(b'k8s\x00') else json.loads(data)
                with (fixture.root / 'writes.jsonl').open('a') as stream:
                    stream.write(json.dumps({'method': self.command, 'path': path, 'body': body}) + '\n')
                if (fixture.root / 'delay-writes').exists():
                    time.sleep(10)
                if '/secrets' in path:
                    fixture.records[body['metadata']['name']] = body
                elif self.command == 'PATCH':
                    obj = fixture.objects[path]
                    # Helm's strategic merge patch updates replicas for this fixture.
                    obj['spec'].update({key: value for key, value in body.get('spec', {}).items() if not key.startswith('$')})
                    body = obj
                else:
                    name = body['metadata']['name']
                    fixture.objects[path + '/' + name if self.command == 'POST' else path] = body
                self.respond(body, 201 if self.command == 'POST' else 200)

            do_POST = mutate
            do_PUT = mutate
            do_PATCH = mutate

        self.server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.url = 'http://127.0.0.1:' + str(self.server.server_address[1])

    def close(self):
        self.server.shutdown()
        self.server.server_close()
