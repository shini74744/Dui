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
for(const flag of [undefined,false,true]){
 const existing={privateKey:'existing-private',shortIds:['12345678'],serverNames:['example.com'],settings:{publicKey:'existing-public',fingerprint:'chrome'},duiRequireHybridKeyShare:flag};
 const saved=RealityStreamSettings.fromJson(existing).toJson();
 assert.equal(saved.duiRequireHybridKeyShare,flag===true);
 assert.equal(saved.privateKey,existing.privateKey);
 assert.deepEqual(saved.shortIds,existing.shortIds);
 assert.equal(saved.settings.publicKey,existing.settings.publicKey);
 assert.equal(saved.settings.duiRequireHybridKeyShare,undefined);
 assert.equal(RealityStreamSettings.fromJson(saved).toJson().duiRequireHybridKeyShare,flag===true);
}
assert.equal(new RealityStreamSettings().duiRequireHybridKeyShare,false);
for(const auth of ['X25519, not Post-Quantum','ML-KEM-768, Post-Quantum']){
 const cfg={clients:[],decryption:'mlkem768x25519plus.native.600s.existing-key',encryption:'mlkem768x25519plus.native.0rtt.existing-public',selectedAuth:auth};
 const saved=Inbound.VLESSSettings.fromJson(cfg).toJson();
 assert.equal(saved.decryption,cfg.decryption);assert.equal(saved.encryption,cfg.encryption);assert.equal(saved.selectedAuth,auth);
}
assert.equal(Inbound.VLESSSettings.fromJson({clients:[]}).decryption,'none');
`,c);
const o=context('outbound.js');vm.runInContext(`assert.deepEqual(Outbound.FreedomSettings.fromJson({finalRules:[{action:'block',ip:['127.0.0.0/8']}]}).toJson().finalRules,[{action:'block',ip:['127.0.0.0/8']}]);const tls=TlsStreamSettings.fromJson({allowInsecure:true,pinnedPeerCertSha256:'pin',verifyPeerCertByName:'example.com'}).toJson();assert.equal(tls.pinnedPeerCertSha256,'pin');assert.equal(tls.verifyPeerCertByName,'example.com');assert.equal(tls.allowInsecure,true);`,o);
console.log('UI serialization: 4 protocols, Mbps migration/zero, TLS and trusted proxy round trips passed');
