const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const C = require('../shared/course.cjs');
const read = file => fs.readFileSync(path.join(__dirname, '..', file), 'utf8');

test('Documented screenshot sources are allowed in both native shells', () => {
  const app = read('app/app.js');
  const android = read('android/app/src/main/java/ai/arena/esxiatlas/MainActivity.java');
  const desktop = read('desktop/main.cjs');
  const urls = [
    ...[...app.matchAll(/source:'(https:\/\/[^']+)'/g)].map(x => x[1]),
    ...Object.values(C.sources).filter(x => typeof x === 'string' && x.startsWith('https://')),
  ];
  assert.ok(urls.length >= 14);
  for (const url of urls) {
    const host = new URL(url).hostname;
    assert.ok(android.includes(`"${host}"`), `Android missing ${host}`);
    assert.ok(desktop.includes(`'${host}'`), `Windows missing ${host}`);
  }
});

test('Android exports use the system document picker and bounded, typed binary files', () => {
  const android = read('android/app/src/main/java/ai/arena/esxiatlas/MainActivity.java');
  assert.match(android, /@JavascriptInterface public boolean saveTextFile/);
  assert.match(android, /@JavascriptInterface public boolean saveBinaryFile/);
  assert.match(android, /Intent\.ACTION_CREATE_DOCUMENT/);
  assert.match(android, /StandardCharsets\.UTF_8/);
  assert.match(android, /esxi-atlas-plan\.md/);
  assert.match(android, /esxi-atlas-project\.json/);
  assert.match(android, /esxi-atlas-topology\.gif/);
  assert.match(android, /esxi-atlas-topology\.pdf/);
  assert.match(android, /MAX_BINARY_EXPORT = 12 \* 1024 \* 1024/);
  assert.match(android, /pending_export/); // private temporary file survives activity recreation
  assert.match(android, /validGif/);assert.match(android, /validPdf/);
});
