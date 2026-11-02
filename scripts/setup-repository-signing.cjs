#!/usr/bin/env node
/**
 * Generate the PUBLIC-BY-DESIGN lab signing identity requested by the repository owner.
 * Private keys and passwords are committed under signing/repository/. Anyone with read
 * access can sign packages as this project. Do not reuse this identity for production.
 * Requires OpenSSL 3+. Refuses to overwrite an existing identity.
 */
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const crypto = require('node:crypto');
const cp = require('node:child_process');
const root = path.resolve(__dirname, '..');
const dir = path.join(root, 'signing/repository');
const pub = path.join(root, 'signing/public');
const identityFile = path.join(dir, 'identity.json');
if (fs.existsSync(identityFile)) throw new Error('signing/repository/identity.json already exists. Existing identity will NOT be overwritten.');
function run(cmd, args, env = {}) {
  return cp.execFileSync(cmd, args, { stdio: ['ignore', 'pipe', 'pipe'], env: { ...process.env, ...env } });
}
run('openssl', ['version']);
fs.mkdirSync(dir, { recursive: true });
fs.mkdirSync(pub, { recursive: true });
const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'esxi-atlas-signing-'));
const password = () => crypto.randomBytes(32).toString('base64url');
const androidPassword = password();
const windowsPassword = password();
function pin(pem) {
  return crypto.createHash('sha256').update(new crypto.X509Certificate(pem).raw).digest('hex');
}
function makeCert(name, subject, days, extra) {
  const key = path.join(temp, name + '-key.pem');
  const cert = path.join(temp, name + '-cert.pem');
  run('openssl', ['req', '-x509', '-newkey', 'rsa:3072', '-nodes', '-days', String(days), '-sha256',
    '-keyout', key, '-out', cert, '-subj', subject, ...extra]);
  return { key, cert, pem: fs.readFileSync(cert, 'utf8') };
}
function makeP12(out, material, alias, pass) {
  // Use widely supported PKCS#12 algorithms so Windows signtool and Java/Android can import it.
  run('openssl', ['pkcs12', '-export', '-out', out, '-inkey', material.key, '-in', material.cert,
    '-name', alias, '-keypbe', 'PBE-SHA1-3DES', '-certpbe', 'PBE-SHA1-3DES', '-macalg', 'sha1',
    '-passout', 'env:ATLAS_P12_PASS'], { ATLAS_P12_PASS: pass });
}
try {
  const android = makeCert('android', '/CN=ESXi Atlas Android/O=ESXi Atlas/C=BY', 10000,
    ['-addext', 'basicConstraints=critical,CA:FALSE', '-addext', 'keyUsage=critical,digitalSignature,keyEncipherment']);
  const windows = makeCert('windows', '/CN=ESXi Atlas Code Signing/O=ESXi Atlas/C=BY', 3650,
    ['-addext', 'basicConstraints=critical,CA:FALSE', '-addext', 'keyUsage=critical,digitalSignature', '-addext', 'extendedKeyUsage=codeSigning']);
  const androidP12 = path.join(dir, 'android-release.p12');
  const windowsPfx = path.join(dir, 'windows-signing.pfx');
  makeP12(androidP12, android, 'esxi-atlas', androidPassword);
  makeP12(windowsPfx, windows, 'ESXi Atlas Code Signing', windowsPassword);
  const androidPin = pin(android.pem);
  const windowsPin = pin(windows.pem);
  fs.writeFileSync(path.join(pub, 'android-certificate.pem'), android.pem);
  fs.writeFileSync(path.join(pub, 'windows-certificate.pem'), windows.pem);
  fs.writeFileSync(path.join(pub, 'android-certificate.sha256'), androidPin + '\n');
  fs.writeFileSync(path.join(pub, 'windows-certificate.sha256'), windowsPin + '\n');
  const identity = {
    schema: 1,
    warning: 'PUBLIC LAB IDENTITY. Private keys and passwords are intentionally committed. Anyone can sign packages as ESXi Atlas. Not for production.',
    createdAt: new Date().toISOString(),
    android: {
      keystore: 'signing/repository/android-release.p12',
      storeType: 'PKCS12',
      alias: 'esxi-atlas',
      storePassword: androidPassword,
      keyPassword: androidPassword,
      certificateSha256: androidPin
    },
    windows: {
      certificate: 'signing/repository/windows-signing.pfx',
      password: windowsPassword,
      publisherName: 'ESXi Atlas Code Signing',
      certificateSha256: windowsPin
    }
  };
  fs.writeFileSync(identityFile, JSON.stringify(identity, null, 2) + '\n');
  fs.writeFileSync(path.join(dir, 'README.md'), '# Public lab signing identity\n\nThese keys and passwords are intentionally public at the repository owner\'s request. They are not a security boundary. Do not reuse them for production or trust them outside this lab.\n');
  console.log('Repository lab signing identity generated.');
  console.log('Android SHA-256: ' + androidPin);
  console.log('Windows SHA-256: ' + windowsPin);
} finally {
  fs.rmSync(temp, { recursive: true, force: true });
}
