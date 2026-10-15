const fs=require('fs'),vm=require('vm'),assert=require('assert/strict'),path=require('path');
const root=path.resolve(__dirname,'..');
const html=fs.readFileSync(root+'/web/html/inbounds.html','utf8');
const method=html.slice(html.indexOf('            searchInbounds(key) {'),html.indexOf('            filterInbounds() {')).trim().replace(/,$/,'');
assert(method.startsWith('searchInbounds(key) {'));
const context=vm.createContext({console,assert});
for(const file of ['web/assets/js/util/index.js','web/assets/js/model/inbound.js','web/assets/js/model/dbinbound.js'])vm.runInContext(fs.readFileSync(root+'/'+file,'utf8'),context);
vm.runInContext('const search=({'+method+'}).searchInbounds;',context);
vm.runInContext(String.raw`
const clients=[
 {id:'uuid-alpha',email:'Alpha@example.test',comment:'Shanghai customer',subId:'sub-alpha',enable:true},
 {id:'uuid-beta',email:'Beta@example.test',comment:'Tokyo customer',subId:'sub-beta',enable:true}
];
function inbound(id,port,protocol='vless'){
 return new DBInbound({id,port,protocol,remark:'Japan dedicated',enable:true,
  settings:JSON.stringify({clients,decryption:'none',method:'2022-blake3-aes-128-gcm',password:'keep-settings',customExtension:{keep:true}}),
  clientStats:[{email:clients[0].email,up:0,down:0},{email:clients[1].email,up:0,down:887766}],
  streamSettings:'{}',sniffing:'{}',tag:'inbound-'+port});
}
let rows=[inbound(4,43009),new DBInbound({id:9,port:19000,protocol:'socks',remark:'Other service',settings:'{"accounts":[{"user":"socks-user"}]}',clientStats:[]})];
const state={dbInbounds:rows,searchedInbounds:[]},original=JSON.stringify(rows);
function find(key){search.call(state,key);return state.searchedInbounds}
function users(key){const result=find(key);assert.equal(result.length,1,'row for '+key);return JSON.parse(result[0].settings).clients.map(c=>c.email)}
assert.deepEqual(users('4300'),clients.map(c=>c.email),'port hit must retain every client');
assert.deepEqual(users('japan'),clients.map(c=>c.email));
assert.deepEqual(users('vless'),clients.map(c=>c.email));
assert.deepEqual(users('ALPHA@'),['Alpha@example.test']);
assert.deepEqual(users('Tokyo customer'),['Beta@example.test']);
assert.deepEqual(users('sub-beta'),['Beta@example.test']);
assert.deepEqual(users('uuid-alpha'),['Alpha@example.test']);
assert.deepEqual(users('887766'),['Beta@example.test'],'client traffic matches only that client');
assert.equal(JSON.parse(find('alpha')[0].settings).password,'keep-settings');
assert.deepEqual(JSON.parse(find('alpha')[0].settings).customExtension,{keep:true});
assert.equal(find('no-such-value').length,0);
assert.equal(find('socks-user')[0].id,9);
assert.equal(find('').length,2);
assert.equal(JSON.stringify(rows),original,'search must not mutate saved models');
for(const protocol of ['vmess','vless','trojan','shadowsocks','hysteria','tuic','mtproto','amneziawg']){
 state.dbInbounds=[inbound(4,43009,protocol)];
 assert.equal(users('4300').length,2,protocol);
 assert.deepEqual(users('alpha'),['Alpha@example.test'],protocol);
 assert.equal(JSON.parse(find('alpha')[0].settings).customExtension.keep,true,protocol);
 assert.equal(users('4300').length,2,'switching query must restore all clients');
}
console.log('Inbound search: port/remark/protocol, client name/comment/ID/traffic, clearing, preserved settings and 8 protocols passed');
`,context);
