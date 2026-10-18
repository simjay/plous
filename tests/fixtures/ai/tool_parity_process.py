"""Mock installed runtime exercising the production MCP bridge."""
import json
import sys
import urllib.request

args = sys.argv[1:]
with open(args[args.index('--mcp-config') + 1]) as stream:
    server = json.load(stream)['mcpServers']['plous']
call = json.loads(sys.stdin.readline())


def rpc(method, params, identity):
    data = json.dumps({'jsonrpc': '2.0', 'id': identity, 'method': method, 'params': params}).encode()
    request = urllib.request.Request(server['url'], data=data, headers=server['headers'])
    with urllib.request.urlopen(request) as response:
        message = json.load(response)
    assert 'error' not in message, message
    return message['result']


def emit(value):
    print(json.dumps(value), flush=True)


rpc('initialize', {}, 1)
definitions = rpc('tools/list', {}, 2)['tools']
emit({'type': 'system', 'subtype': 'init', 'tools': ['mcp__plous__' + t['name'] for t in definitions]})
reply = rpc('tools/call', call, 3)
answer = json.dumps({'definitions': definitions, 'delivery': json.loads(reply['content'][0]['text'])})
emit({'type': 'result', 'subtype': 'success', 'result': answer})
