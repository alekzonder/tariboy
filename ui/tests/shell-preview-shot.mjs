// Screenshot the console shell for a visual check against the design handoff.
//
//   npx vite --config vite.workspace-test.config.ts --port 4175 --strictPort &
//   node tests/shell-preview-shot.mjs shell.png
//
// The fixture it loads (tests/shell-preview.tsx) mounts the real App with a
// stubbed daemon, so what is captured is the actual shell, not a mock of it.
import { chromium } from 'playwright';
const out = process.argv[2] ?? '/tmp/shot.png';
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
page.on('console', m => { if (m.type() === 'error') console.error('PAGE ERR', m.text()); });
page.on('pageerror', e => console.error('PAGE EXC', e.message));
await page.goto('http://127.0.0.1:4175/tests/shell-preview.html', { waitUntil: 'networkidle' });
await page.waitForTimeout(1500);
await page.screenshot({ path: out });
await browser.close();
console.log('saved', out);
