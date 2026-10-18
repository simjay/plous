"""Interactive CLI fixture with its own screen, input and conversation persistence."""
import json
import os
from pathlib import Path
import signal
import sys
import termios
import tomllib
import tty
import urllib.request

args = sys.argv[1:]
if args[:1] == ['app-server']:
    for line in sys.stdin:
        message = json.loads(line)
        if 'id' not in message:
            continue
        result = {'userAgent': 'plous/future-fixture (native)'}
        if message['method'] == 'config/read':
            result = {'config': {'mcp_servers': {'unrelated': {}, 'plous': {}}}}
        print(json.dumps({'id': message['id'], 'result': result}), flush=True)
    sys.exit(0)
assert sys.stdin.isatty() and sys.stdout.isatty()
assert '--print' not in args and '--output-format' not in args
if '--mcp-config' in args:
    assert '--disable-slash-commands' not in args
    server = json.loads(Path(args[args.index('--mcp-config') + 1]).read_text())['mcpServers']['plous']
    instructions = args[args.index('--append-system-prompt') + 1]
else:
    values = dict((k.strip(),v.strip()) for i, arg in enumerate(args) if i and args[i-1] == '-c' for k,v in [arg.split('=', 1)])
    servers = tomllib.loads('mcp_servers=' + values['mcp_servers'])['mcp_servers']
    assert not servers['unrelated']['enabled']
    assert not servers['plous']['enabled']
    assert '--no-daemon' in args and '--sandbox' in args
    server = {'url': servers['plous_']['url'],
              'headers': {'Authorization': 'Bearer ' + os.environ['PLOUS_TOOL_TOKEN']}}
    instructions = tomllib.loads('developer_instructions=' + values['developer_instructions'])['developer_instructions']
Path('fixture-cwd').write_text(str(Path.cwd()))
Path('fixture-pid').write_text(str(os.getpid()))
Path('fixture-args.json').write_text(json.dumps(args))
with Path('fixture-launches.jsonl').open('a') as launches:
    launches.write(json.dumps(args) + '\n')
failure = Path('fixture-failure.json')
if failure.exists():
    fault = json.loads(failure.read_text())
    print(fault['message'], flush=True)
    sys.exit(fault['code'])
saved = Path('fixture-history')
if '--resume' in args and not saved.exists():
    print('\x1b[31mNo conversation found with session ID: ' + args[args.index('--resume') + 1] + '\x1b[0m\r\n\x0f', flush=True)
    sys.exit(1)
request_id = 0

def rpc(method, params):
    global request_id
    request_id += 1
    body = json.dumps({'jsonrpc': '2.0', 'id': request_id, 'method': method, 'params': params}).encode()
    request = urllib.request.Request(server['url'], data=body, headers=server['headers'])
    with urllib.request.urlopen(request) as response:
        result = json.load(response)
    assert 'error' not in result, result
    return result['result']

rpc('initialize', {})
tools = rpc('tools/list', {})['tools']
Path('fixture-tools.json').write_text(json.dumps(tools))
assert any(t['name'] == 'history_read' for t in tools)
reply = saved.read_text() if saved.exists() and ('--resume' in args or '--last' in args) else 'Ready'
draft = bytearray()
key = ''
interrupts = 0

def draw(*_):
    size = os.get_terminal_size()
    text = draft.decode('utf-8', errors='replace')
    sys.stdout.write('\x1b[2J\x1b[HNative CLI fixture\r\nSize: %d x %d\r\n%s\r\n%s\r\n> %s' %
                     (size.columns, size.lines, reply, key, text))
    sys.stdout.flush()

original = termios.tcgetattr(0)
tty.setraw(0)
signal.signal(signal.SIGWINCH, draw)
sys.stdout.write('\x1b[?2004h')
draw()
try:
    while True:
        char = os.read(0, 1)
        if not char:
            break
        if char == b'\r':
            text = draft.decode('utf-8', errors='replace')
            if text == '/scroll-fixture':
                sys.stdout.write('\x1b[2J\x1b[H')
                for row in range(90):
                    sys.stdout.write('Earlier reply %03d\r\n' % row)
                reply = 'Latest reply after long output'
                draft.clear()
                draw()
                continue
            if text == '/login':
                reply = 'Native sign-in menu'
                draft.clear()
                draw()
                continue
            if text == 'count helm releases':
                captured = json.loads(instructions.split('Captured connection and page:\n', 1)[1])
                connection = next(c for c in captured['connections'] if c['tool'] == 'helm')
                result = rpc('tools/call', {'name': 'helm_releases_read', 'arguments': {
                    'purpose': 'Count installed releases across namespaces', 'operation': 'list',
                    'connection_id': connection['connection_id'], 'namespace': '*'}})
                delivery = json.loads(result['content'][0]['text'])
                Path('fixture-helm.json').write_text(json.dumps(delivery))
                observed = delivery['observation']
                assert observed['outcome'] == 'completed' and observed['coverage']['complete'], delivery
                reply = 'Helm releases: ' + str(observed['facts']['observed_total']['number'])
            else:
                rpc('tools/call', {'name': 'history_read', 'arguments': {'purpose': 'Read saved evidence', 'operation': 'search', 'query': text}})
                reply = 'Native reply: ' + text
            saved.write_text(reply)
            draft.clear()
            key = ''
        elif char == b'\t':
            key = 'Native Tab'
        elif char == b'\x1b':
            key = 'Native Esc'
        elif char == b'\x03':
            interrupts += 1
            key = 'Native Ctrl+C: ' + str(interrupts)
        else:
            draft.extend(char)
        draw()
finally:
    termios.tcsetattr(0, termios.TCSANOW, original)
