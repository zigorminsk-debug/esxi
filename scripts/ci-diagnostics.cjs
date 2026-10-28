// Only used by explicitly unsigned CI jobs. Never attach this step to signing jobs.
// GitHub log downloads are unavailable in some restricted clients; public annotations
// expose the compiler/builder error without credentials or private key material.
const fs = require('node:fs');
const file = process.argv[2] || 'native-build.log';
const text = fs.existsSync(file) ? fs.readFileSync(file, 'utf8').slice(-14000) : 'No build log produced.';
const escape = value => value.replace(/%/g, '%25').replace(/\r/g, '%0D').replace(/\n/g, '%0A');
console.log('::error title=Unsigned build diagnostic::' + escape(text));
console.log(text);
