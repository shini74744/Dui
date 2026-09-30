const fs=require('fs'),vm=require('vm'),assert=require('assert'),path=require('path'),crypto=require('crypto').webcrypto;
let keyIndex=0;
const ctx=vm.createContext({assert,crypto,TextEncoder,URLSearchParams,console,location:{hostname:'test.example'},Wireguard:{generateKeypair:()=>({publicKey:'pub'+(++keyIndex),privateKey:'priv'+keyIndex})},RandomUtil:{randomShortIds:()=>['abcdef12'],randomInteger:()=>43210,randomUUID:()=>crypto.randomUUID(),randomLowerAndNum:()=> 'example',randomSeq:()=> 'example'},ObjectUtil:{clone:x=>x,isEmpty:x=>x==null,isArrEmpty:x=>!x?.length},DateUtil:{},Date});
for(const f of ['inbound.js','dbinbound.js'])vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/assets/js/model',f),'utf8'),ctx);
vm.runInContext(`
for(const protocol of [Protocols.TUIC,Protocols.MTPROTO,Protocols.AMNEZIAWG]){
 const i=new Inbound();i.protocol=protocol;assert(i.isHelperProtocol());assert(i.clients.length===1);
 const raw=i.toJson();assert.deepEqual(Inbound.fromJson(raw).toJson(),raw);assert(!i.canEnableReality());assert(!i.canEnableTls());
 const db=new DBInbound();db.protocol=protocol;assert(db.isMultiUser());assert(db.hasLink());
}
const t=new Inbound();t.protocol=Protocols.TUIC;t.clients[0].password=':/?#@中文';t.settings.sni='tls.example';
const link=t.genLink('2001:db8::1',443,'same','title',t.clients[0]);assert(link.startsWith('tuic://'));assert(link.includes('@[2001:db8::1]:443?'));assert(link.includes('sni=tls.example'));assert(!link.includes(':/?#@中文'));
const m=new Inbound();m.protocol=Protocols.MTPROTO;assert(/^ee[0-9a-f]+$/.test(m.clients[0].secret));assert(m.genLink('test.example',443,'same','title',m.clients[0]).startsWith('tg://proxy?'));assert(!('password' in m.clients[0].toJson()));
const a=new Inbound();a.protocol=Protocols.AMNEZIAWG;assert.deepEqual(a.clients[0].allowedIPs,['10.8.0.2/32']);const second=Inbound.HelperClient.create(a.protocol,a.settings);assert.deepEqual(second.allowedIPs,['10.8.0.3/32']);
const config=a.genLink('2001:db8::1',51820,'same','title',a.clients[0]);assert(config.includes('Endpoint = [2001:db8::1]:51820'));assert(config.includes('PrivateKey = '+a.clients[0].privateKey));assert(!config.includes(a.settings.server.privateKey));assert(!config.includes(second.privateKey));
a.settings.server.randomTrailers=true;a.settings.server.disableCookies=false;const advanced=a.genAmneziaConfig('test.example',51820,a.clients[0]);assert(advanced.includes('RandomTrailers = on'));assert(!advanced.includes('DisableCookies'));
a.settings.server.disableCookies=true;assert(a.genAmneziaConfig('test.example',51820,a.clients[0]).includes('DisableCookies = on'));
`,ctx);
console.log('TUIC/MTProto/AmneziaWG model roundtrips, identities, URI escaping and client-only config exports passed');
// These are in-DOM Vue templates: HTML parsers do not self-close custom tags.
// A self-closing alert consumed all following user controls in the browser.
for (const file of ['hysteria','helpers','tun']) {
 const html=fs.readFileSync(path.join(__dirname,'../web/html/form/protocol/'+file+'.html'),'utf8');
 assert(!/<a-[\w-]+(?:[^"'<>]|"[^"]*"|'[^']*')*\/>/.test(html),file+' must explicitly close custom HTML elements');
}
