"""Exercise Helm with isolated data, kubeconfig, and native SDK API fixtures."""
import json
from pathlib import Path
import sys
import tempfile
import unittest
from urllib.parse import parse_qs, urlsplit

from support import ROOT, Terminal

sys.path.insert(0, str(ROOT / "tests/fixtures/helm"))
from helm_api_fixture import HelmAPI


class HelmTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory(prefix='plous-helm-test-')
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.api = HelmAPI(self.root)
        self.addCleanup(self.api.close)
        self.namespace_server = self.api.url
        self.config = self.root / 'kubeconfig' 
        self.config.write_text('apiVersion: v1\nkind: Config\ncontexts:\n- name: staging\n  context: {cluster: fixture, namespace: apps}\nclusters:\n- name: fixture\n  cluster: {server: "' + self.namespace_server + '"}\n')
        bin_dir = self.root / 'bin'
        bin_dir.mkdir()
        self.env = {'PATH': str(bin_dir)}
        self.state = self.root / 'state'
    def start(self):
        app = Terminal(self.state, self.config, width=180, height=42, env_overrides=self.env)
        self.addCleanup(app.close)
        app.expect('AI assistant')
        return app

    def open_helm(self):
        app = self.start()
        app.send(':new\rCommerce\r\r')
        app.expect('Add Connection')
        app.send('\r')
        app.expect('Add Context')
        app.send(' \x1b[B\r')
        app.expect('Added 1 Kubernetes')
        app.send('\x03')
        app.assert_exit()
        self.configure_repository()
        app = self.start()
        app.expect_screen('Current Workspace')
        app.send('\x1b[1;3D/staging\r')
        app.expect_screen('Helm')
        app.send('/Releases\r')
        app.expect('shop')
        app.expect('Revision')
        app.expect('deployed')
        app.expect('1.0.0')
        return app

    def configure_repository(self):
        config = self.root / 'repositories.yaml'
        config.write_text(json.dumps({'apiVersion': 'v1', 'repositories': [{'name': 'fixture', 'url': self.api.url}]}))
        cache = self.root / 'repository'
        cache.mkdir(exist_ok=True)
        (cache / 'fixture-index.yaml').write_text(json.dumps({'apiVersion': 'v1', 'entries': {
            'demo': [{'apiVersion': 'v2', 'name': 'demo', 'version': '1.0.0', 'description': 'Fixture chart', 'urls': ['demo.tgz']}]
        }}))
        # Use the same persisted optional paths as connection settings.
        for path in (self.state / 'workspaces').rglob('*.json'):
            if path.name == 'session.json' or 'catalog-cache' in path.parts:
                continue
            try:
                data = json.loads(path.read_text())
            except ValueError:
                continue
            def update(value):
                if isinstance(value, dict):
                    if value.get('tool') == 'kubernetes' and 'settings' in value:
                        value['settings'].update(repository_config=str(config), repository_cache=str(cache))
                    for child in value.values():
                        update(child)
                elif isinstance(value, list):
                    for child in value:
                        update(child)
            update(data)
            path.write_text(json.dumps(data))

    def test_current_context_detects_helm_and_refresh_removes_absent_tool(self):
        self.config.write_text(self.config.read_text().replace("kind: Config\n", "kind: Config\ncurrent-context: staging\n"))
        before = self.config.read_bytes()
        app = self.start()
        app.expect_screen("Current Workspace")
        app.expect_screen("Helm")
        self.assertTrue(self.api.metadata_requests)
        metadata_queries = [parse_qs(urlsplit(path).query) for path in self.api.metadata_requests]
        self.assertTrue(all(query.get("labelSelector") == ["owner=helm"] for query in metadata_queries))
        # Overview also requests release inventory after the one-item discovery read.
        self.assertTrue(any(query.get("limit") == ["1"] for query in metadata_queries))
        app.send("\x1b[1;3D/staging\r")
        app.send("/Releases\r")
        app.expect_screen("shop")
        self.api.records.clear()
        count = len(self.api.metadata_requests)
        app.send("\x1b")
        app.send("/staging\rg")
        app.wait(lambda: len(self.api.metadata_requests) > count, "Helm discovery refresh")
        app.wait(lambda: "Helm" not in "\n".join(app.screen()["rows"]), "absent Helm branch removed")
        self.assertFalse(any(self.state.joinpath("workspaces").iterdir()))
        self.assertEqual(self.config.read_bytes(), before)
        app.send("\x03")
        app.assert_exit()

    def expect_completed(self, app):
        try:
            app.expect('Helm operation completed')
        except AssertionError:
            app.send('!')
            app.resize(600, 24)
            app.send(':f\r')
            app.read(0.2)
            outcomes = [path.read_text() for path in self.state.glob('workspaces/*/operations/*.json')]
            errors = [line for line in app.text().splitlines() if 'unable to build kubernetes' in line]
            self.fail('\n'.join(errors[-2:]) + '\nOutcomes: ' + repr(outcomes) + '\nRequests: ' + repr(self.api.requests))

    def select_section(self, app, section):
        app.send('\x1b')
        app.send('/' + section + '\r')

    def test_release_chart_views_and_guarded_rollback(self):
        app = self.open_helm()
        self.select_section(app, 'Charts')
        app.expect('2')
        app.send('\r')
        app.expect('Resources')
        app.expect('Release: apps/shop')
        app.send(':B\r')
        app.expect('Choose an earlier revision')
        app.send('\x1b[B\r')
        app.expect('historical')
        app.send('B')
        app.expect('Rollback preview ready')
        app.expect('Helm / Charts')
        self.assertFalse((self.root / 'writes.jsonl').exists())
        app.send(':v\r\r')
        self.assertFalse((self.root / 'writes.jsonl').exists(), 'comparison Enter wrote')
        app.send('\x1b')
        app.send('\x1b')
        app.expect('Resources')
        app.send('a\x1b[F\x1b[A')
        app.expect('Resume unfinished action')
        app.send('\r')
        app.expect('Rollback preview')
        app.send('a')
        app.expect('Execute rollback')
        app.send('\x1b[B\x1b[B\x1b[B\r')
        app.expect('Confirm rollback')
        app.send('\x1b[C\r')
        self.expect_completed(app)
        writes = [json.loads(line) for line in (self.root / 'writes.jsonl').read_text().splitlines()]
        self.assertEqual(sum(w['method'] == 'POST' and w['path'].endswith('/secrets') for w in writes), 1)
        self.assertEqual(self.api.objects['/apis/apps/v1/namespaces/apps/deployments/shop']['spec']['replicas'], 1)
        app.send(':3\r')
        app.expect('Rollback shop')
        app.send('\x03')
        app.assert_exit()
        self.assertNotIn('fixture-sensitive', (self.state / 'session.json').read_text())

    def test_release_timeline_and_shared_revision_inspector(self):
        app = self.open_helm()
        app.send('\r')
        app.expect_screen('Revision 2 · latest')
        app.expect_screen('Changes from revision 1')
        app.expect_screen('1 changed')
        self.assertNotIn('fixture-sensitive', app.text())
        app.send('v')
        app.expect_screen('Recorded revision 1')
        app.expect_screen('Recorded revision 2')
        app.expect_screen('replicas: 2')
        app.send('\x1b')
        app.send('\x1b[B\r')
        app.expect_screen('historical')
        app.expect_screen('Recorded configuration')
        app.send('F')
        app.expect_screen('Recorded revision')
        app.send('\x1b[H\r')
        app.expect_screen('↑↓ Resource')
        app.send('\x1b')
        app.resize(181, 43)
        app.expect_screen('Initial installation')
        app.send('B')
        app.expect_screen('Rollback preview ready')
        self.assertFalse((self.root / 'writes.jsonl').exists())
        app.send('\x1b')
        app.resize(180, 42)
        app.expect_screen('Initial installation')
        app.send('\x03')
        app.assert_exit()


    def test_failed_namespace_change_stays_in_new_scope(self):
        app = self.open_helm()
        self.api.namespace_denied = True
        app.send('n')
        app.expect('Access denied')
        app.send('blocked\r\r')
        app.expect('Fixture namespace access denied')
        app.send('!')
        app.expect('Helm error')
        app.send('\x1b')
        start = len(app.output)
        app.send(':N\rapps\r\r')
        app.expect('shop', start)
        self.assertTrue(any('/namespaces/blocked/secrets' in path for path in self.api.requests))
        self.assertFalse((self.root / 'writes.jsonl').exists())


    def test_namespace_picker_sort_filter_and_table_details(self):
        app = self.open_helm()
        app.send('n')
        app.expect('All namespaces')
        app.expect('apps  ✓')
        app.expect('production')
        app.send('prod\r')
        app.expect('No releases in this scope')
        self.assertEqual([path.split('?')[0] for path in self.api.namespace_requests], ['/api/v1/namespaces'])
        start = len(app.output)
        app.send('n\x1b[H\r')
        app.expect('1 releases · all namespaces', start)
        app.send('S')
        app.expect('Sort by')
        app.send('\r')
        app.expect('Name ↓')
        app.send('/nothing\r')
        app.expect('releases match this filter')
        start = len(app.output)
        app.send('\x7f')
        app.expect('shop', start)
        app.send('\r')
        app.expect('Changes from revision 1')
        app.send('\r')
        app.expect('Resources')
        app.expect('Recorded configuration')
        app.expect('Deployment / shop')
        app.send('F')
        app.expect('Recorded revision')
        app.expect('superseded')
        app.send('\x1b')
        app.send('\x1b')
        app.resize(181, 43)
        app.expect('Changes from revision 1')
        self.assertFalse((self.root / 'writes.jsonl').exists())

    def test_inspector_search_refresh_and_direct_resource_reader(self):
        app = self.open_helm()
        app.send('\r')
        app.expect_screen('Changes from revision 1')
        app.send('\r')
        app.expect_screen('Recorded configuration')
        app.send('\r')
        app.expect_screen('Plous Editor')
        app.expect_screen('replicas: 2')
        app.send('\x1b')
        app.send('Fsuperseded')
        app.expect_screen('Recorded revision')
        app.send('\r')
        app.expect_screen('historical')
        app.send('\x1b[C')
        app.expect_screen('Computed values')
        app.send('g')
        app.resize(181, 43)
        app.expect_screen('↑↓ Scroll')
        app.expect_screen('Computed values')
        app.send('\r\x1b[B\r')
        app.resize(180, 42)
        app.expect_screen('Supplied values')
        app.send('/replicas\r')
        app.resize(181, 43)
        app.expect_screen('matches')
        self.assertNotIn('fixture-sensitive', app.text())
        app.send('\x1b')
        app.send('\x1b[D')
        app.expect_screen('Recorded configuration')
        self.assertFalse((self.root / 'writes.jsonl').exists())
