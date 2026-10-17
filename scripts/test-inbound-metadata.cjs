const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict'),path=require('node:path');
const root=path.resolve(__dirname,'..');
const ctx=vm.createContext({console,assert,window:{crypto:require('node:crypto').webcrypto,location:{protocol:'https:'}},HttpUtil:{get:async()=>({success:true,obj:{}})}});
for(const file of ['web/assets/js/util/index.js','web/assets/js/model/inbound.js','web/assets/js/model/dbinbound.js'])vm.runInContext(fs.readFileSync(root+'/'+file,'utf8'),ctx);
vm.runInContext('HttpUtil.get=async()=>({success:true,obj:{}});',ctx);
const html=fs.readFileSync(root+'/web/html/modals/inbound_modal.html','utf8');
const code=html.slice(html.indexOf('    const inModal ='),html.indexOf('    new Vue(')).replace(/{{[^}]+}}/g,'text');
vm.runInContext(code,ctx);
vm.runInContext(`
const db=new DBInbound({id:1,port:44001,protocol:'vless',remark:'old',enable:true,clientStats:[],
 settings:JSON.stringify({clients:[{id:'uuid',email:'alpha',enable:true}],decryption:'none',unknown:{keep:true}}),
 streamSettings:'{"network":"tcp","security":"none"}',sniffing:'{"enabled":false}',tag:'inbound-44001'});
function open(){inModal.show({dbInbound:db,inbound:db.toInbound(),isEdit:true})}
open();assert(inModal.remarkOnly());
inModal.dbInbound.remark='a new name';assert(inModal.remarkOnly(),'metadata rename not recognized');
inModal.dbInbound.up=4000;inModal.dbInbound.lastTrafficResetTime=900;assert(inModal.remarkOnly(),'live counters must not force reload');
for(const [key,value] of [['enable',false],['total',1000],['expiryTime',100],['trafficReset','monthly'],['deviceLimit',3]]){
 open();inModal.dbInbound[key]=value;assert(!inModal.remarkOnly(),key+' must follow normal save');
}
for(const change of [
 ()=>inModal.inbound.port++,
 ()=>inModal.inbound.listen='127.0.0.1',
 ()=>inModal.inbound.clients[0].id='different',
 ()=>inModal.inbound.clients[0].email='different',
 ()=>inModal.inbound.sniffing.enabled=true,
 ()=>inModal.inbound.stream.security='tls'
]){open();change();assert(!inModal.remarkOnly(),'node configuration was mistaken for metadata')}
inModal.show({isEdit:false});assert(!inModal.remarkOnly(),'new inbound routed as rename');
console.log('Inbound metadata: remark-only path, live counters, connection/settings changes passed');
`,ctx);
