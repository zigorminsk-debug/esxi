#!/usr/bin/env node
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const root=path.resolve(__dirname,'..');
const [mode='--check',dir]=process.argv.slice(2);
function hash(file){return crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');}
function readPin(name){const s=fs.readFileSync(path.join(root,'signing/public',name),'utf8').trim();if(!/^[0-9a-f]{64}$/.test(s))throw new Error('Missing/invalid public pin '+name);return s;}
if(mode==='--check'){
 readPin('android-certificate.sha256');readPin('windows-certificate.sha256');
 for(const name of ['ANDROID_KEYSTORE_BASE64','ANDROID_KEYSTORE_PASSWORD','ANDROID_KEY_PASSWORD','ANDROID_KEY_ALIAS','ANDROID_CERT_SHA256','WINDOWS_CSC_LINK','WINDOWS_CSC_PASSWORD','WINDOWS_CERT_SHA256'])if(!process.env[name])throw new Error('Missing GitHub Actions secret '+name);
 if(process.env.ANDROID_CERT_SHA256!==readPin('android-certificate.sha256')||process.env.WINDOWS_CERT_SHA256!==readPin('windows-certificate.sha256'))throw new Error('Secret signing identities do not match committed public pins');
 console.log('Release signing configuration present.');
}else if(mode==='--android'){
 const apk=path.join(dir,'ESXi-Atlas-android.apk');if(!fs.existsSync(apk))throw new Error('Signed APK missing');
 const version=JSON.parse(fs.readFileSync(path.join(root,'package.json'))).version;
 const manifest={schema:1,packageName:'ai.arena.esxiatlas',version,versionCode:Number(process.env.ATLAS_VERSION_CODE),tag:process.env.ATLAS_TAG,apk:'ESXi-Atlas-android.apk',sha256:hash(apk),certificateSha256:readPin('android-certificate.sha256')};
 if(!Number.isInteger(manifest.versionCode)||manifest.versionCode<10000||!/^v\d+\.\d+\.\d+$/.test(manifest.tag||''))throw new Error('Invalid versionCode or tag');
 fs.writeFileSync(path.join(dir,'android-update.json'),JSON.stringify(manifest,null,2)+'\n');console.log('Android update manifest generated.');
}else throw new Error('Usage: release-manifest.cjs --check | --android <dir>');
