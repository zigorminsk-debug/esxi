#!/usr/bin/env node
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const dir=path.resolve(process.argv[2]||'release-artifacts');
const manifest=JSON.parse(fs.readFileSync(path.join(dir,'android-update.json'),'utf8'));
const pkg=require('../package.json');
const hash=f=>crypto.createHash('sha256').update(fs.readFileSync(f)).digest('hex');
if(manifest.schema!==1||manifest.packageName!=='ai.arena.esxiatlas'||manifest.version!==pkg.version||manifest.apk!=='ESXi-Atlas-android.apk')throw new Error('Invalid Android manifest identity');
if(hash(path.join(dir,manifest.apk))!==manifest.sha256)throw new Error('APK digest mismatch');
if(manifest.certificateSha256!==fs.readFileSync(path.join(__dirname,'../signing/public/android-certificate.sha256'),'utf8').trim())throw new Error('Android signing pin mismatch');
const checksums=fs.readFileSync(path.join(dir,'SHA256SUMS.txt'),'utf8').trim().split(/\r?\n/);
for(const line of checksums){const match=line.match(/^([a-f0-9]{64})  (.+)$/);if(!match||match[2].includes('/')||match[2].includes('\\'))throw new Error('Invalid checksum entry');if(hash(path.join(dir,match[2]))!==match[1])throw new Error('Checksum mismatch '+match[2]);}
for(const name of ['ESXi-Atlas-android.apk','android-update.json','latest.yml'])if(!checksums.some(l=>l.endsWith('  '+name)))throw new Error('Uncovered release asset '+name);
console.log('Release hashes and Android identity verified. Windows Authenticode is additionally checked on the Windows runner and by the installed updater.');
