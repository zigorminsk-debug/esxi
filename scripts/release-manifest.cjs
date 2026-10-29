#!/usr/bin/env node
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const root=path.resolve(__dirname,'..');
const [mode='--check',dir]=process.argv.slice(2);
function hash(file){return crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');}
function readPin(name){const s=fs.readFileSync(path.join(root,'signing/public',name),'utf8').trim();if(!/^[0-9a-f]{64}$/.test(s))throw new Error('Missing/invalid public pin '+name);return s;}
if(mode==='--check'){
 const identityFile=path.join(root,'signing/repository/identity.json');
 if(!fs.existsSync(identityFile))throw new Error('Missing signing/repository/identity.json. Run scripts/setup-repository-signing.cjs once.');
 const identity=JSON.parse(fs.readFileSync(identityFile,'utf8'));
 if(identity.schema!==1||!identity.android||!identity.windows)throw new Error('Invalid repository signing identity');
 for(const rel of [identity.android.keystore,identity.windows.certificate])if(!fs.existsSync(path.join(root,rel)))throw new Error('Missing signing file '+rel);
 if(!identity.android.storePassword||!identity.android.keyPassword||!identity.android.alias||!identity.windows.password)throw new Error('Incomplete repository signing credentials');
 if(identity.android.certificateSha256!==readPin('android-certificate.sha256')||identity.windows.certificateSha256!==readPin('windows-certificate.sha256'))throw new Error('Repository identity does not match committed public pins');
 console.log('Public repository lab signing identity present and pins match.');
}else if(mode==='--android'){
 const apk=path.join(dir,'ESXi-Atlas-android.apk');if(!fs.existsSync(apk))throw new Error('Signed APK missing');
 const version=JSON.parse(fs.readFileSync(path.join(root,'package.json'))).version;
 const manifest={schema:1,packageName:'ai.arena.esxiatlas',version,versionCode:Number(process.env.ATLAS_VERSION_CODE),tag:process.env.ATLAS_TAG,apk:'ESXi-Atlas-android.apk',sha256:hash(apk),certificateSha256:readPin('android-certificate.sha256')};
 if(!Number.isInteger(manifest.versionCode)||manifest.versionCode<10000||!/^v\d+\.\d+\.\d+$/.test(manifest.tag||''))throw new Error('Invalid versionCode or tag');
 fs.writeFileSync(path.join(dir,'android-update.json'),JSON.stringify(manifest,null,2)+'\n');console.log('Android update manifest generated.');
}else throw new Error('Usage: release-manifest.cjs --check | --android <dir>');
