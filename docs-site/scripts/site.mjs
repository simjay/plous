import { watch } from 'node:fs';
import { spawn } from 'node:child_process';
import path from 'node:path';
import { prepare, root } from './prepare.mjs';

const mode = process.argv[2];
if (!['dev', 'build', 'preview'].includes(mode)) throw new Error('Expected dev, build, or preview.');
if (mode !== 'preview') await prepare();
const watchers = [];
if (mode === 'dev') {
  let pending = Promise.resolve();
  for (const directory of ['docs', 'cmd/plous']) {
    watchers.push(watch(path.join(root, directory), () => {
      pending = pending.then(prepare).catch(error => console.error(error.message));
    }));
  }
}
const child = spawn(process.execPath, [
  path.join(root, 'docs-site/node_modules/astro/bin/astro.mjs'), mode,
  // Cached Markdown can retain code-block CSS links after a theme change.
  ...(mode === 'build' ? ['--force'] : []), ...process.argv.slice(3),
], {
  cwd: path.join(root, 'docs-site'), stdio: 'inherit',
  env: { ...process.env, ASTRO_TELEMETRY_DISABLED: '1' },
});
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => child.kill(signal));
child.on('exit', code => {
  watchers.forEach(watcher => watcher.close());
  process.exitCode = code ?? 1;
});
