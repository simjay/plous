"""Keyboard protocol coverage, independent of terminal branding and user settings."""

from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

from support import FIXTURE, Terminal


PROTOCOLS = {
    'xterm': dict(left='\x1b[D', right='\x1b[C', home='\x1b[H', end='\x1b[F',
                  backspace='\x7f', delete='\x1b[3~', newline='\n', escape='\x1b',
                  enter='\r', tab='\t', backtab='\x1b[Z',
                  previous='\x1b[1;3D', following='\x1b[1;3C', toggle='\x1d', quit='\x03'),
    'application-cursor': dict(left='\x1bOD', right='\x1bOC', home='\x1bOH', end='\x1bOF',
                              previous='\x1b\x1bOD', following='\x1b\x1bOC'),
    'escape-prefix': dict(previous='\x1b\x1b[D', following='\x1b\x1b[C'),
    'mac-word-arrows': dict(previous='\x1bb', following='\x1bf'),
    'iterm-meta': dict(previous='\x1b[1;9D', following='\x1b[1;9C'),
    'csi-u': dict(backspace='\x1b[127u', newline='\x1b[13;2u', escape='\x1b[27u',
                  enter='\x1b[13u', tab='\x1b[9u', backtab='\x1b[9;2u',
                  toggle='\x1b[93;5u', quit='\x1b[99;5u'),
    'kitty-locks': dict(left='\x1b[1;129D', right='\x1b[1;129C', home='\x1b[1;129H',
                       end='\x1b[1;129F', backspace='\x1b[127;129u', delete='\x1b[3;129~',
                       newline='\x1b[13;130u', escape='\x1b[27;129u',
                       enter='\x1b[13;129u', tab='\x1b[9;129u', backtab='\x1b[9;130u',
                       previous='\x1b[1;131D', following='\x1b[1;131C', toggle='\x1b[93;133u', quit='\x1b[99;133u'),
    'modify-other-keys': dict(backspace='\x1b[27;1;127~', newline='\x1b[27;2;13~',
                              enter='\x1b[27;1;13~', tab='\x1b[27;1;9~', backtab='\x1b[27;2;9~',
                              escape='\x1b[27;1;27~', toggle='\x1b[27;5;93~', quit='\x1b[27;5;99~'),
    'win32-input': dict(left='\x1b[37;75;0;1;0;1_', right='\x1b[39;77;0;1;0;1_',
                       home='\x1b[36;71;0;1;0;1_', end='\x1b[35;79;0;1;0;1_',
                       backspace='\x1b[8;14;8;1;0;1_', delete='\x1b[46;83;0;1;0;1_',
                       newline='\x1b[13;28;13;1;16;1_', escape='\x1b[27;1;27;1;0;1_',
                       enter='\x1b[13;28;13;1;0;1_', tab='\x1b[9;15;9;1;0;1_',
                       backtab='\x1b[9;15;9;1;16;1_',
                       previous='\x1b[37;75;0;1;2;1_', following='\x1b[39;77;0;1;2;1_',
                       toggle='\x1b[221;27;29;1;8;1_', quit='\x1b[67;46;3;1;8;1_'),
}


class KeyboardTests(unittest.TestCase):
    def exercise(self, root, protocol, prefix=None):
        keys = dict(PROTOCOLS['xterm'], **protocol)
        state = root / 'state'
        app = Terminal(state, FIXTURE, command_prefix=prefix)
        self.addCleanup(app.close)
        app.expect('AI assistant')
        app.send(':c' + keys['enter'])
        app.send('A界B' + keys['left'] + keys['backspace'] + 'é' + keys['right'])
        app.send(keys['home'] + keys['delete'] + keys['end'] + keys['newline'] + 'tail')
        draft = 'éB\ntail'
        app.wait_json(state / 'session.json', lambda s: s['drafts'].get('') == draft)
        app.send(keys['tab'] + keys['backtab'])
        app.wait_json(state / 'session.json', lambda s: s['focus'] == 2 and s['drafts'].get('') == draft)
        app.send(keys['toggle'])
        app.wait_json(state / 'session.json', lambda s: s['focus'] == 1)
        app.send(keys['toggle'])
        app.wait_json(state / 'session.json', lambda s: s['focus'] == 2 and s['drafts'].get('') == draft)
        app.send(keys['previous'])
        app.wait_json(state / 'session.json', lambda s: s['focus'] == 1)
        app.send(keys['tab'])
        app.wait_json(state / 'session.json', lambda s: s['focus'] == 0)
        app.send(keys['backtab'])
        app.wait_json(state / 'session.json', lambda s: s['focus'] == 0)
        app.send(keys['tab'])
        app.wait_json(state / 'session.json', lambda s: s['focus'] == 1)
        app.send('\x1b[200~cc:quit\x1b[201~')
        app.wait_json(state / 'session.json', lambda s: s['focus'] == 1)
        app.send(keys['previous'])
        app.wait_json(state / 'session.json', lambda s: s['focus'] == 0)
        app.send(keys['toggle'])
        app.wait_json(state / 'session.json', lambda s: s['focus'] == 2)
        app.send(keys['toggle'])
        app.wait_json(state / 'session.json', lambda s: s['focus'] == 0)
        app.send(keys['following'])
        app.wait_json(state / 'session.json', lambda s: s['focus'] == 1)
        app.send(keys['following'] + ' more')
        app.wait_json(state / 'session.json', lambda s: s['drafts'].get('') == draft + ' more')
        app.send('\x1b[200~ bb :q\n界\x1b[201~')
        app.wait_json(state / 'session.json', lambda s: s['drafts'].get('') == draft + ' more bb :q\n界')
        app.send(keys['escape'])
        app.send('hh')
        app.expect_screen('Global · Navigation')
        app.send(keys['quit'])
        app.assert_exit()

    def test_keyboard_protocols_preserve_focus_and_text(self):
        for name, keys in PROTOCOLS.items():
            with self.subTest(protocol=name), tempfile.TemporaryDirectory(prefix='plous-keys-') as directory:
                self.exercise(Path(directory), keys)

    def test_backspace_control_byte_edits_text(self):
        with tempfile.TemporaryDirectory(prefix='plous-backspace-') as directory:
            self.exercise(Path(directory), {'backspace': '\x08'})

    def test_kitty_release_events_do_not_repeat_navigation(self):
        with tempfile.TemporaryDirectory(prefix='plous-key-release-') as directory:
            state = Path(directory) / 'state'
            app = Terminal(state, FIXTURE)
            self.addCleanup(app.close)
            app.expect('AI assistant')
            app.send(':c\rkept')
            app.send('\x1b[1;3:1D\x1b[1;3:3D')
            app.wait_json(state / 'session.json', lambda s: s['focus'] == 1)
            app.send('\x1b[1;3:1C\x1b[1;3:3C')
            app.wait_json(state / 'session.json', lambda s: s['focus'] == 2)
            app.send('\x1b[93;5:1u\x1b[93;5:2u\x1b[93;5:3u')
            app.wait_json(state / 'session.json', lambda s: s['focus'] == 1)
            app.send('\x1b[93;5:1u\x1b[93;5:3u')
            app.wait_json(state / 'session.json', lambda s: s['focus'] == 2)
            app.send('\x1b[13;2:1u\x1b[13;2:3uend')
            app.wait_json(state / 'session.json', lambda s: s['drafts'].get('') == 'kept\nend')
            app.send('\x03')
            app.assert_exit()

    @unittest.skipUnless(shutil.which('tmux'), 'tmux is not installed')
    def test_keyboard_through_isolated_tmux(self):
        with tempfile.TemporaryDirectory(prefix='plous-tmux-') as directory:
            root = Path(directory)
            socket = str(root / 'tmux.sock')
            tmux = [shutil.which('tmux'), '-S', socket, '-f', '/dev/null']
            try:
                self.exercise(root, PROTOCOLS['xterm'], tmux + ['new-session', '-s', 'keyboard'])
            finally:
                subprocess.run(tmux + ['kill-server'], capture_output=True, timeout=5)
