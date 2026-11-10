import { spawn } from 'node:child_process';
import path from 'node:path';
import { website } from './content.mjs';
import { prepareContent } from './prepare.mjs';
await prepareContent();
await new Promise((resolve,reject)=>{
  const child=spawn('cargo',['check','--locked'],{cwd:path.join(website,'rune'),stdio:'inherit'});
  child.on('error',reject);
  child.on('exit',code=>code===0?resolve():reject(new Error(`cargo check exited ${code}`)));
});
