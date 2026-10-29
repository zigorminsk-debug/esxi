const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '..');
const version = require('../package.json').version;
for (const dir of ['app/vendor','android/app/src/main/assets']) fs.mkdirSync(path.join(root,dir),{recursive:true});
fs.copyFileSync(path.join(root,'shared/model.cjs'),path.join(root,'app/vendor/model.js'));
fs.copyFileSync(path.join(root,'shared/topology.cjs'),path.join(root,'app/vendor/topology.js'));
// gifenc is bundled locally for both native shells; it never fetches a CDN.
const gifenc = fs.readFileSync(require.resolve('gifenc'),'utf8').replace(/\/\/# sourceMappingURL=.*$/m,'');
fs.writeFileSync(path.join(root,'app/vendor/gifenc.js'),'(function(){var exports={};\n'+gifenc+'\nwindow.AtlasGIF=exports;})();\n');
fs.writeFileSync(path.join(root,'app/vendor/content.js'),'window.ESXICourse = '+JSON.stringify(require('../shared/course.cjs'))+';\n');
fs.writeFileSync(path.join(root,'app/version.json'),JSON.stringify({version},null,2)+'\n');
fs.writeFileSync(path.join(root,'app/index.html'),fs.readFileSync(path.join(root,'app/index.html'),'utf8').replace(/name="app-version" content="[^"]+"/,`name="app-version" content="${version}"`));
for (const file of ['index.html','styles.css','topology.css','app.js','topology-editor.js','topology-export.js','version.json']) fs.copyFileSync(path.join(root,'app',file),path.join(root,'android/app/src/main/assets',file));
for (const dir of ['vendor','assets']) fs.cpSync(path.join(root,'app',dir),path.join(root,'android/app/src/main/assets',dir),{recursive:true});
for (const file of ['app.js','topology-editor.js','topology-export.js','vendor/model.js','vendor/topology.js','vendor/gifenc.js','vendor/content.js']) require('node:child_process').execFileSync(process.execPath,['--check',path.join(root,'app',file)]);
console.log('Offline assets prepared for browser, Android and Windows. Version '+version);
