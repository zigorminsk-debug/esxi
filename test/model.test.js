const {test}=require('node:test');const assert=require('node:assert/strict');const {network,validate}=require('../src/model');
const base={lan:'192.168.10.0/24',guest:'192.168.20.0/24',ssid:'Office',guestSsid:'Guest',wan:'DHCP',wanInterface:'ether1',backup:false,backupInterface:'ether2',vpn:'Не нужен',exposed:'',port:'',publishWan:'primary'};
test('valid plan',()=>assert.equal(validate(base).errors.length,0));
test('overlap and non canonical addresses',()=>{assert.ok(validate({...base,guest:'192.168.10.0/25'}).errors.length);assert.equal(network('192.168.10.1/24').canonical,false)});
test('reject invalid publication and disabled backup',()=>assert.ok(validate({...base,exposed:'192.168.20.4',port:'443',publishWan:'backup'}).errors.length>=2));
test('reject duplicate WAN',()=>assert.ok(validate({...base,backup:true,backupInterface:'ether1'}).errors.length));
