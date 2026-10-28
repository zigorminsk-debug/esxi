#!/usr/bin/env node
/** Generate persistent signing identities locally, upload PRIVATE material to GitHub Secrets.
 * Requires OpenSSL 3+, JDK keytool, authenticated gh with repository Actions-secret write access.
 * Usage: node scripts/setup-signing.cjs [--local-only]
 * Never overwrites an existing identity. Keep local-secrets/ outside Git and back it up offline.
 */
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),cp=require('node:child_process');
const root=path.resolve(__dirname,'..'),dir=path.join(root,'local-secrets'),pub=path.join(root,'signing/public');
const repo=JSON.parse(fs.readFileSync(path.join(root,'package.json'))).build.publish[0];
const slug=repo.owner+'/'+repo.repo;
function run(cmd,args,options={}){return cp.execFileSync(cmd,args,{stdio:['ignore','pipe','pipe'],...options}).toString().trim();}
function tool(name,args){try{run(name,args);}catch{throw new Error('Required tool unavailable: '+name+'. Install OpenSSL and JDK 17+, then retry.');}}
tool('openssl',['version']);tool('keytool',['-help']);
if(!process.argv.includes('--local-only'))tool('gh',['auth','status']);
if(fs.existsSync(dir)&&fs.readdirSync(dir).length)throw new Error('local-secrets already exists. Existing signing identity will NOT be overwritten.');
fs.mkdirSync(dir,{recursive:true,mode:0o700});fs.mkdirSync(pub,{recursive:true});
const password=crypto.randomBytes(32).toString('base64url'),winPassword=crypto.randomBytes(32).toString('base64url');
const secrets={ANDROID_KEYSTORE_PASSWORD:password,ANDROID_KEY_PASSWORD:password,ANDROID_KEY_ALIAS:'esxi-atlas',WINDOWS_CSC_PASSWORD:winPassword};
try{
 run('keytool',['-genkeypair','-keystore',path.join(dir,'android-release.jks'),'-storetype','JKS','-alias','esxi-atlas','-keyalg','RSA','-keysize','3072','-validity','10000','-dname','CN=ESXi Atlas Android, O=ESXi Atlas, C=BY','-storepass:env','ATLAS_STORE_PASS','-keypass:env','ATLAS_KEY_PASS'],{env:{...process.env,ATLAS_STORE_PASS:password,ATLAS_KEY_PASS:password}});
 run('keytool',['-exportcert','-rfc','-keystore',path.join(dir,'android-release.jks'),'-alias','esxi-atlas','-storepass:env','ATLAS_STORE_PASS','-file',path.join(pub,'android-certificate.pem')],{env:{...process.env,ATLAS_STORE_PASS:password}});
 const androidDer=crypto.execFileSync('keytool',['-exportcert','-keystore',path.join(dir,'android-release.jks'),'-alias','esxi-atlas','-storepass:env','ATLAS_STORE_PASS'],{env:{...process.env,ATLAS_STORE_PASS:password},maxBuffer:1024*1024});
 const androidPin=crypto.createHash('sha256').update(androidDer).digest('hex');
 const cfg=path.join(dir,'windows-openssl.cnf');fs.writeFileSync(cfg,'[req]\ndistinguished_name=dn\nprompt=no\nx509_extensions=ext\n[dn]\nCN=ESXi Atlas Code Signing\nO=ESXi Atlas\nC=BY\n[ext]\nbasicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=codeSigning\nsubjectKeyIdentifier=hash\n',{mode:0o600});
 run('openssl',['req','-x509','-newkey','rsa:3072','-nodes','-days','3650','-keyout',path.join(dir,'windows-key.pem'),'-out',path.join(dir,'windows-cert.pem'),'-config',cfg,'-sha256']);
 run('openssl',['pkcs12','-export','-out',path.join(dir,'windows-signing.pfx'),'-inkey',path.join(dir,'windows-key.pem'),'-in',path.join(dir,'windows-cert.pem'),'-name','ESXi Atlas Code Signing','-passout:env','ATLAS_PFX_PASS'],{env:{...process.env,ATLAS_PFX_PASS:winPassword}});
 const winPem=fs.readFileSync(path.join(dir,'windows-cert.pem'),'utf8').match(/-----BEGIN CERTIFICATE-----[\s\S]+?-----END CERTIFICATE-----/)[0];
 fs.writeFileSync(path.join(pub,'windows-certificate.pem'),winPem+'\n');
 const winPin=crypto.createHash('sha256').update(new crypto.X509Certificate(winPem).raw).digest('hex');
 fs.writeFileSync(path.join(pub,'android-certificate.sha256'),androidPin+'\n');fs.writeFileSync(path.join(pub,'windows-certificate.sha256'),winPin+'\n');
 secrets.ANDROID_KEYSTORE_BASE64=fs.readFileSync(path.join(dir,'android-release.jks')).toString('base64');secrets.ANDROID_CERT_SHA256=androidPin;
 secrets.WINDOWS_CSC_LINK=fs.readFileSync(path.join(dir,'windows-signing.pfx')).toString('base64');secrets.WINDOWS_CERT_SHA256=winPin;
 fs.writeFileSync(path.join(dir,'github-secrets.json'),JSON.stringify(secrets),{mode:0o600});
 fs.chmodSync(path.join(dir,'android-release.jks'),0o600);fs.chmodSync(path.join(dir,'windows-signing.pfx'),0o600);fs.chmodSync(path.join(dir,'windows-key.pem'),0o600);
 if(process.argv.includes('--local-only')){console.log('Identity generated locally. Private keys remain in ignored local-secrets/. Upload secrets manually before release.');}
 else{
  for(const [name,value]of Object.entries(secrets)){cp.execFileSync('gh',['secret','set',name,'--repo',slug],{input:value,stdio:['pipe','pipe','pipe']});console.log('Uploaded secret: '+name);}
  console.log('All signing secrets uploaded. Commit ONLY signing/public/. Back up local-secrets/ offline, then remove the working copy when safely stored.');
 }
 console.log('Public Android SHA-256: '+androidPin+'\nPublic Windows SHA-256: '+winPin+'\nPRIVATE KEYS WERE NOT PRINTED.');
}catch(error){throw new Error('Signing setup failed; no release was published. '+error.message+'\nSecurely preserve or delete the incomplete local-secrets directory before retrying. Do not rotate an identity already installed on users\' devices.');}
