'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, 'web', 'app.js'), 'utf8');
new vm.Script(source);
const prefix = source.slice(0, source.indexOf('const SYSTEM'));
const context = vm.createContext({});
vm.runInContext(prefix, context);
assert.equal(vm.runInContext('ERROR_KINDS.captcha', context), '\u4eba\u673a\u9a8c\u8bc1');
assert.equal(vm.runInContext('ERROR_KINDS.no_result', context), '\u65e0\u7ed3\u679c');
console.log('retrieval error labels passed');
