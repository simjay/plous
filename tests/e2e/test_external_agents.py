"""Native CLI screens run in a PTY without a vendor account."""
import json
import os
from pathlib import Path
import shlex
import sys
import tempfile
import unittest

from support import FIXTURE, ROOT, Terminal

sys.path.insert(0, str(ROOT / 'tests/fixtures/helm'))
from helm_api_fixture import HelmAPI


class ExternalAgentTests(unittest.TestCase):
    def open_agent(self, root, kind, label, moves, kubeconfig=FIXTURE):
        state = root / 'state'
        tools = root / 'bin'
        tools.mkdir()
        binary = tools / kind
        binary.write_text('#!/bin/sh\nexec ' + shlex.quote(sys.executable) + ' ' +
                          shlex.quote(str(ROOT / 'tests/fixtures/ai/terminal_claude.py')) + ' "$@"\n')
        binary.chmod(0o700)
        env = {'PATH': str(tools)}
        app = Terminal(state, kubeconfig, width=160, height=36, env_overrides=env)
        self.addCleanup(app.close)
        app.expect('AI assistant')
        if kubeconfig != FIXTURE:
            app.wait_json(state / 'session.json', lambda s: s.get('connection', '').startswith('kube-'))
        app.send(':s\r')
        app.expect_screen('Agent: Plous')
        app.send('\r')
        app.expect_screen('Selected agent')
        app.send('\x1b[C' * moves + '\r\r')
        app.expect_screen('Agent: ' + label)
        app.send('\x1b')
        app.expect_screen('Native CLI fixture')
        self.assertEqual((root / 'launch' / 'fixture-cwd').read_text(), str((root / 'launch').resolve()))
        app.send(':c\r')
        app.expect_screen('Ctrl+] Return to panel')
        return app, state, env

    def close_agent(self, app):
        app.send('\x1d')
        app.send(':X\r')
        app.expect_screen('Session ended')

    def test_native_ctrl_c_forwards_then_panel_return_allows_exit(self):
        for kind, label, moves in [('claude', 'Claude Code', 1), ('codex', 'Codex', 2)]:
            with self.subTest(kind=kind), tempfile.TemporaryDirectory(prefix='plous-native-interrupt-') as directory:
                app, state, _ = self.open_agent(Path(directory), kind, label, moves)
                pid_file = state.parent / 'launch/fixture-pid'
                pid = int(pid_file.read_text())
                app.expect_screen('Ctrl+C Interrupt agent')
                # Each outer-terminal encoding must deliver one Ctrl+C to the child.
                for count, interrupt in enumerate(('\x03', '\x1b[99;5u', '\x1b[27;5;99~'), 1):
                    app.send(interrupt)
                    app.expect_screen('Native Ctrl+C: ' + str(count))
                    self.assertIsNone(app.process.poll())
                    self.assertEqual(pid, int(pid_file.read_text()))
                    os.kill(pid, 0)
                app.send('after interrupt\r')
                app.expect_screen('Native reply: after interrupt')
                app.send('\x1d')
                app.wait_json(state / 'session.json', lambda s: s['focus'] == 1)
                app.send('\x03')
                app.assert_exit()
                with self.assertRaises(ProcessLookupError):
                    os.kill(pid, 0)
                self.assertEqual((state.parent / 'launch/fixture-history').read_text(), 'Native reply: after interrupt')

    def test_settings_switch_closes_previous_agent_and_keeps_navigation(self):
        with tempfile.TemporaryDirectory(prefix='plous-agent-switch-') as directory:
            root = Path(directory)
            app, state, _ = self.open_agent(root, 'claude', 'Claude Code', 1)
            codex = root / 'bin/codex'
            codex.write_bytes((root / 'bin/claude').read_bytes())
            codex.chmod(0o700)
            pid_file = root / 'launch/fixture-pid'

            current = 'Claude Code'
            for label, movement in [('Codex', '\x1b[C'), ('Claude Code', '\x1b[D'), ('Plous', '\x1b[D')]:
                with self.subTest(agent=label):
                    previous = int(pid_file.read_text())
                    app.send('\x1b[1;3D')
                    app.wait_json(state / 'session.json', lambda s: s['focus'] == 1)
                    app.send(':s\r')
                    app.expect_screen('Agent: ' + current)
                    app.send('\r')
                    app.expect_screen('Selected agent')
                    app.send(movement + '\r' + ('\x1b[B' if label == 'Plous' else '') + '\r')
                    app.expect_screen('Agent: ' + label)
                    with self.assertRaises(ProcessLookupError):
                        os.kill(previous, 0)
                    if label != 'Plous':
                        app.wait(lambda: int(pid_file.read_text()) != previous, 'replacement agent process')
                        app.expect_screen('Native CLI fixture')
                    app.send('\x1b')
                    app.send('cc')
                    if label != 'Plous':
                        app.expect_screen('Ctrl+] Return to panel')
                        app.send(label + '\r')
                        app.expect_screen('Native reply: ' + label)
                    app.send('\x1b[1;3D')
                    app.wait_json(state / 'session.json', lambda s: s['focus'] == 1)
                    app.send('\x1b[1;3C')
                    app.wait_json(state / 'session.json', lambda s: s['focus'] == 2)
                    current = label
            app.send('\x03')
            app.assert_exit()

    def test_provider_setup_preserves_pending_agent_and_native_session(self):
        with tempfile.TemporaryDirectory(prefix='plous-agent-providers-') as directory:
            root = Path(directory)
            app, state, _ = self.open_agent(root, 'claude', 'Claude Code', 1)
            pid = int((root / 'launch/fixture-pid').read_text())
            app.send('\x1b[1;3D:s\r')
            app.expect_screen('Agent: Claude Code')
            self.assertNotIn('OpenAI', '\n'.join(app.screen()['rows']))
            app.send('\r')
            app.expect_screen('Selected agent')
            self.assertNotIn('Provider connections', '\n'.join(app.screen()['rows']))
            app.send('\x1b[D\r\r')
            app.expect_screen('Add provider connection')
            app.expect_screen('Vendor')
            app.send('\rfixture-provider-key\r\r')
            app.expect_screen('Provider connection saved')
            app.expect_screen('OpenAI')
            os.kill(pid, 0)
            app.send('\x1b')
            app.expect_screen('Agent: Claude Code')
            self.assertNotIn('OpenAI', '\n'.join(app.screen()['rows']))
            app.send('\r\x1b[D\r\r')
            app.expect_screen('Edit provider connection')
            app.send('\rdiscarded-draft\rhh')
            app.expect_screen('Global')
            app.send('\x1b')
            app.expect_screen('API Key (saved; blank keeps it)')
            app.send('\x1b')
            app.expect_screen('Provider connections')
            app.send('\x1b')
            app.expect_screen('Agent: Claude Code')
            self.assertEqual(pid, int((root / 'launch/fixture-pid').read_text()))
            os.kill(pid, 0)
            self.assertNotIn('discarded-draft', (state / 'credentials.json').read_text())
            self.assertNotIn('fixture-provider-key', app.text())
            app.send('\x03')
            app.assert_exit()

    def test_native_mouse_focus_and_keyboard_escape(self):
        for kind, label, moves in [('claude', 'Claude Code', 1), ('codex', 'Codex', 2)]:
            with self.subTest(kind=kind), tempfile.TemporaryDirectory(prefix='plous-native-focus-') as directory:
                app, state, _ = self.open_agent(Path(directory), kind, label, moves)
                pid = int((state.parent / 'launch/fixture-pid').read_text())
                app.send('retained draft')
                app.expect_screen('> retained draft')
                # Mouse reporting must stay enabled while the child owns typing.
                for mode in (b'\x1b[?1002', b'\x1b[?1006'):
                    self.assertGreater(app.output.rfind(mode + b'h'), app.output.rfind(mode + b'l'))
                for x, focus in [(70, 1), (5, 0)]:
                    app.mouse_click(x, 20)
                    app.wait_json(state / 'session.json', lambda s: s['focus'] == focus)
                    app.mouse_click(145, 20)
                    app.wait_json(state / 'session.json', lambda s: s['focus'] == 2)
                    app.expect_screen('> retained draft')
                # Legacy, CSI-u and modifyOtherKeys toggle focus without changing native input.
                for toggle in ('\x1d', '\x1b[93;5u', '\x1b[27;5;93~'):
                    app.send(toggle)
                    app.wait_json(state / 'session.json', lambda s: s['focus'] == 0)
                    app.send(toggle)
                    app.wait_json(state / 'session.json', lambda s: s['focus'] == 2)
                    app.expect_screen('Ctrl+] Return to panel')
                    app.expect_screen('> retained draft')
                app.send(' after click')
                app.expect_screen('> retained draft after click')
                self.assertEqual(pid, int((state.parent / 'launch/fixture-pid').read_text()))
                os.kill(pid, 0)
                app.send('\x1d\x03')
                app.assert_exit()

    def test_native_panel_shortcuts_accept_terminal_encodings(self):
        with tempfile.TemporaryDirectory(prefix='plous-native-keys-') as directory:
            app, state, _ = self.open_agent(Path(directory), 'claude', 'Claude Code', 1)
            app.send('retained draft')
            for previous, following in [('\x1b[1;3D', '\x1b[1;3C'),
                                        ('\x1b[1;9D', '\x1b[1;9C'),
                                        ('\x1bb', '\x1bf'),
                                        ('\x1b[57350;3u', '\x1b[57351;3u'),
                                        ('\x1b[57350;33u', '\x1b[57351;33u')]:
                with self.subTest(previous=repr(previous)):
                    app.send(previous)
                    app.wait_json(state / 'session.json', lambda s: s['focus'] == 1)
                    app.send(following)
                    app.wait_json(state / 'session.json', lambda s: s['focus'] == 2)
                    app.expect_screen('> retained draft')
            app.send('\x1d\x03')
            app.assert_exit()

    def test_startup_native_agents_read_captured_helm_releases(self):
        for kind, label, moves in [('claude', 'Claude Code', 1), ('codex', 'Codex', 2)]:
            with self.subTest(kind=kind), tempfile.TemporaryDirectory(prefix='plous-native-helm-') as directory:
                root = Path(directory)
                api = HelmAPI(root)
                self.addCleanup(api.close)
                config = root / 'kubeconfig'
                config.write_text('apiVersion: v1\nkind: Config\ncurrent-context: staging\ncontexts:\n'
                                  '- name: staging\n  context: {cluster: fixture, namespace: apps}\n'
                                  'clusters:\n- name: fixture\n  cluster: {server: "' + api.url + '"}\n')
                before = config.read_bytes()
                app, state, env = self.open_agent(root, kind, label, moves, config)
                app.send('\x1d\x03')
                app.assert_exit()
                app.close()
                # Startup must connect real tools before the user enters the panel.
                app = Terminal(state, config, width=180, height=42, env_overrides=env)
                self.addCleanup(app.close)
                app.expect_screen('Ready')
                tools = json.loads((root / 'launch/fixture-tools.json').read_text())
                names = {tool['name'] for tool in tools}
                self.assertTrue({'kubernetes_resources_read', 'helm_releases_read', 'history_read'} <= names)
                app.send('cc')
                app.send('count helm releases\r')
                app.expect_screen('Helm releases: 1')
                result = json.loads((root / 'launch/fixture-helm.json').read_text())
                self.assertEqual(result['observation']['rows'][0]['identity']['name'], 'shop')
                self.assertEqual(result['observation']['namespace'], '*')
                # The returned reference already exists durably before CLI delivery.
                records = [json.loads(line) for path in state.glob('**/reasoning/*.jsonl')
                           for line in path.read_text().splitlines()]
                helm = [record for record in records if record.get('tool') == 'helm_releases_read']
                self.assertEqual(len(helm), 1)
                self.assertEqual(helm[0]['projection'], result['observation'])
                self.assertTrue(result['ref'].endswith(':' + helm[0]['id']))
                self.assertEqual(config.read_bytes(), before)
                self.assertFalse((root / 'writes.jsonl').exists())
                app.send('\x1d\x03')
                app.assert_exit()

    def test_native_wheel_scrolls_output_and_preserves_prompt(self):
        for kind, label, moves in [('claude', 'Claude Code', 1), ('codex', 'Codex', 2)]:
            with self.subTest(kind=kind), tempfile.TemporaryDirectory(prefix='plous-native-scroll-') as directory:
                app, state, _ = self.open_agent(Path(directory), kind, label, moves)
                app.send('/scroll-fixture\r')
                app.expect_screen('Latest reply after long output')
                app.send('retained native draft')
                app.expect_screen('> retained native draft')
                app.send('\x1b[1;3D')
                app.wait_json(state / 'session.json', lambda s: s['focus'] == 1)
                for _ in range(40):
                    app.send('\x1b[<64;146;13M' * 6)
                    if 'Earlier reply' in '\n'.join(app.screen()['rows']):
                        break
                app.expect_screen('Earlier reply')
                self.assertEqual(json.loads((state / 'session.json').read_text())['focus'], 1)
                app.mouse_click(145, 12)
                app.expect_screen('End / Esc Live')
                before = '\n'.join(app.screen()['rows'])
                app.send('\x1b[5~')
                app.wait(lambda: '\n'.join(app.screen()['rows']) != before, 'Page Up scrollback')
                app.resize(180, 40)
                app.expect_screen('End / Esc Live')
                app.send('\x1b[F')
                app.expect_screen('> retained native draft')
                app.expect_screen('Size: 41 x 34')
                app.mouse_wheel(165, 12, up=True)
                app.expect_screen('End / Esc Live')
                app.send('!')
                app.expect_screen('> retained native draft!')
                app.send('\x1d\x03')
                app.assert_exit()

    def test_native_wheel_boundaries_keep_typing_and_panel_travel(self):
        for kind, label, moves in [('claude', 'Claude Code', 1), ('codex', 'Codex', 2)]:
            with self.subTest(kind=kind), tempfile.TemporaryDirectory(prefix='plous-native-boundaries-') as directory:
                app, state, _ = self.open_agent(Path(directory), kind, label, moves)
                app.send('/scroll-fixture\r')
                app.expect_screen('Latest reply after long output')
                app.send('retained native draft')
                app.expect_screen('> retained native draft')
                up, down = '\x1b[<64;146;13M', '\x1b[<65;146;13M'
                # Exceed the entire bounded history, then keep scrolling at the top.
                app.send(up * 6000)
                app.expect_screen('End / Esc Live')
                top = app.screen()['rows']
                app.send(up * 1000)
                self.assertEqual(app.screen()['rows'], top)
                app.send('\x1b[1;3D')
                app.wait_json(state / 'session.json', lambda s: s['focus'] == 1)
                app.send('\x1b[1;3C')
                app.expect_screen('End / Esc Live')
                app.send(down)
                app.wait(lambda: app.screen()['rows'] != top, 'reverse scroll at top')
                app.send(down * 6000)
                app.wait(lambda: 'End / Esc Live' not in '\n'.join(app.screen()['rows']), 'return to live output')
                app.expect_screen('> retained native draft')
                # Overscroll and type in one input burst to catch event backlogs.
                app.send(down * 1000 + '!\x1b[1;3D')
                app.expect_screen('> retained native draft!')
                app.wait_json(state / 'session.json', lambda s: s['focus'] == 1)
                app.send('\x03')
                app.assert_exit()

    def test_empty_native_session_reopens_and_retains_later_conversation(self):
        for kind, label, moves in [('claude', 'Claude Code', 1), ('codex', 'Codex', 2)]:
            with self.subTest(kind=kind), tempfile.TemporaryDirectory(prefix='plous-native-empty-') as directory:
                app, state, env = self.open_agent(Path(directory), kind, label, moves)
                native = state.parent / 'launch'
                self.close_agent(app)
                self.assertFalse((native / 'fixture-history').exists())
                app.send('cc')
                app.expect_screen('Ctrl+] Return to panel')
                app.expect_screen('Ready')
                app.resize(200, 44)
                app.expect_screen('Size: 46 x 38')
                app.send('\x1d\x03')
                app.assert_exit()
                app.close()

                app = Terminal(state, FIXTURE, width=160, height=36, env_overrides=env)
                self.addCleanup(app.close)
                app.expect_screen('Ready')
                app.send(':c\r')
                app.send('first message\r')
                app.expect_screen('Native reply: first message')
                self.close_agent(app)
                app.send('cc')
                app.expect_screen('Ctrl+] Return to panel')
                app.expect_screen('Native reply: first message')
                app.send('second message\r')
                app.expect_screen('Native reply: second message')
                self.close_agent(app)

                # Explicit New must still open an empty prompt when history exists.
                app.send(':C\r')
                app.expect_screen('Ready')
                self.assertNotIn('Native reply:', '\n'.join(app.screen()['rows']))
                self.close_agent(app)
                # Native history can be removed outside Plous while its marker remains.
                (native / 'fixture-history').unlink()
                app.send('cc')
                app.expect_screen('Ctrl+] Return to panel')
                app.expect_screen('Ready')
                app.send('after removal\r')
                app.expect_screen('Native reply: after removal')
                app.send('\x1d\x03')
                app.assert_exit()

    def test_native_resume_recovery_is_limited_to_missing_conversation(self):
        with tempfile.TemporaryDirectory(prefix='plous-native-error-') as directory:
            app, state, _ = self.open_agent(Path(directory), 'claude', 'Claude Code', 1)
            native = state.parent / 'launch'
            self.close_agent(app)
            session_id = next(state.glob('native-agents/**/session-id')).read_text()
            missing = 'No conversation found with session ID: ' + session_id
            launches = lambda: [json.loads(line) for line in (native / 'fixture-launches.jsonl').read_text().splitlines()]
            for message, code, attempts in [
                ('Authentication required', 1, 1),
                (missing, 2, 1),
                (missing, 0, 1),
                ('Native reply: No conversation found to continue', 1, 1),
                (missing, 1, 2),
            ]:
                with self.subTest(message=message, code=code):
                    before = len(launches())
                    (native / 'fixture-failure.json').write_text(json.dumps({'message': message, 'code': code}))
                    app.send('cc')
                    app.wait(lambda: len(launches()) >= before + attempts, 'native launch')
                    app.expect_screen('Session ended')
                    self.assertEqual(len(launches()) - before, attempts)
                    self.assertIn('--resume', launches()[before])
                    if attempts == 2:
                        self.assertNotIn('--resume', launches()[-1])
            app.send('\x03')
            app.assert_exit()

    def test_native_screen_input_resize_scope_and_restart(self):
        for kind, label, moves in [('claude', 'Claude Code', 1), ('codex', 'Codex', 2)]:
            with self.subTest(kind=kind), tempfile.TemporaryDirectory(prefix='plous-native-') as directory:
                root = Path(directory)
                state = root / 'state'
                (state / 'agent').mkdir(parents=True)
                (state / 'agent/config.yaml').write_text('max_calls: 1\nmax_tokens: 1\ntimeout_seconds: 1\n')
                app, state, env = self.open_agent(root, kind, label, moves)
                app.send('/login\r')
                app.expect_screen('Native sign-in menu')
                app.send('hello\r')
                app.expect_screen('Native reply: hello')
                app.send('second turn\r')
                app.expect_screen('Native reply: second turn')
                app.send('\t')
                app.expect_screen('Native Tab')
                app.send('\x1b')
                app.expect_screen('Native Esc')
                app.send(':s hh /model')
                app.expect_screen('> :s hh /model')
                self.assertNotIn('Selected agent', '\n'.join(app.screen()['rows']))
                pid = int((state.parent / 'launch' / 'fixture-pid').read_text())
                app.send('\x1b[1;3D')
                app.wait_json(state / 'session.json', lambda s: s['focus'] == 1)
                app.send(':s\r')
                app.expect_screen('Agent: ' + label)
                app.send('\x1b[1;3C')
                app.wait_json(state / 'session.json', lambda s: s['focus'] == 2)
                app.expect_screen('> :s hh /model')
                app.send('\x1b[1;3D')
                app.expect_screen('Agent: ' + label)
                app.send('\x1b')
                app.send('\x1b[1;3C')
                app.send('\x1b[1;3C')
                app.wait_json(state / 'session.json', lambda s: s['focus'] == 0)
                app.send('\x1b[1;3D')
                app.expect_screen('> :s hh /model')
                self.assertEqual(pid, int((state.parent / 'launch' / 'fixture-pid').read_text()))
                os.kill(pid, 0)
                app.resize(200, 44)
                app.expect_screen('Size: 46 x 38')
                app.send('\x1d')
                pid = int((state.parent / 'launch' / 'fixture-pid').read_text())
                app.send('ii')
                app.expect_screen('Size: 96 x 38')
                app.expect_screen('> :s hh /model')
                app.send('ii')
                app.wait_json(state / 'session.json', lambda s: s.get('right_hidden'))
                os.kill(pid, 0)
                app.send(':i\r')
                app.expect_screen('Size: 46 x 38')
                app.send('cc')
                app.expect_screen('Ctrl+] Return to panel')
                self.assertEqual(pid, int((state.parent / 'launch' / 'fixture-pid').read_text()))
                app.send('\x1d')
                app.send('ii')
                app.expect_screen('Size: 96 x 38')
                app.send(':s\r')
                app.expect_screen('Agent: ' + label)
                app.send('\x1b')
                app.send(':X\r')
                app.expect_screen('Session ended')
                evidence = lambda: [json.loads(line) for path in state.glob('**/reasoning/*.jsonl')
                                   for line in path.read_bytes().split(b'\n')[:-1] if line]
                app.wait(lambda: bool(evidence()), 'native tool observations')
                pid = int((state.parent / 'launch' / 'fixture-pid').read_text())
                with self.assertRaises(ProcessLookupError):
                    os.kill(pid, 0)
                app.send('\x03')
                app.assert_exit()
                app.close()

                restarted = Terminal(state, FIXTURE, width=160, height=36, env_overrides=env)
                self.addCleanup(restarted.close)
                restarted.expect_screen('Native reply: second turn')
                restarted.expect_screen('Size: 76 x 30')
                restarted.send('\x03')
                restarted.assert_exit()
                restarted.close()
                (root / 'bin' / kind).unlink()
                unavailable = Terminal(state, FIXTURE, width=160, height=36, env_overrides=env)
                self.addCleanup(unavailable.close)
                unavailable.expect_screen('unavailable')
                self.assertNotIn('Connect a vendor', '\n'.join(unavailable.screen()['rows']))
                unavailable.send('\x03')
                unavailable.assert_exit()
