const fs=require('fs'),vm=require('vm'),assert=require('assert');
const root=require('path').resolve(__dirname,'..')+'/';
function context(file){const ctx=vm.createContext({console,assert,RandomUtil:new Proxy({},{get:()=>()=>''}),ObjectUtil:{isEmpty:x=>x==null,clone:x=>x,isArrEmpty:x=>!x?.length},DateUtil:{},Date});vm.runInContext(fs.readFileSync(root+'web/assets/js/model/'+file,'utf8'),ctx);return ctx;}
const c=context('inbound.js');vm.runInContext(`
for(const type of [Inbound.VmessSettings.VMESS,Inbound.VLESSSettings.VLESS,Inbound.TrojanSettings.Trojan,Inbound.ShadowsocksSettings.Shadowsocks]){
 const client=type.fromJson({email:'a',speedLimitMbps:300,totalGB:123,enable:true});
 assert.equal(client.speedLimitMbps,300);const saved=client.toJson();assert.equal(saved.speedLimitMbps,300);assert.equal(saved.totalGB,123);
 assert.equal(type.fromJson({email:'a',speedLimit:1024}).speedLimitMbps,8.388608);
 assert.equal(type.fromJson({email:'a',speedLimit:1024,speedLimitMbps:0}).speedLimitMbps,0);
}
assert.equal(TlsStreamSettings.fromJson({verifyPeerCertInNames:['example.com']}).toJson().verifyPeerCertByName,'example.com');
assert.deepEqual(SockoptStreamSettings.fromJson({trustedXForwardedFor:['127.0.0.1']}).toJson().trustedXForwardedFor,['127.0.0.1']);
`,c);
const o=context('outbound.js');vm.runInContext(`const tls=TlsStreamSettings.fromJson({allowInsecure:true,pinnedPeerCertSha256:'pin',verifyPeerCertByName:'example.com'}).toJson();assert.equal(tls.pinnedPeerCertSha256,'pin');assert.equal(tls.verifyPeerCertByName,'example.com');assert.equal(tls.allowInsecure,true);`,o);
console.log('UI serialization: 4 protocols, Mbps migration/zero, TLS and trusted proxy round trips passed');
