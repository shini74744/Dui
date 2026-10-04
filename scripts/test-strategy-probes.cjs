const assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const P=require('../web/assets/js/util/strategy-probes.js');
const L=require('../web/assets/js/util/strategy-probes-i18n.js');
let cases=0;const test=(name,fn)=>{fn();cases++};
const fixture=()=>({
 routing:{balancers:P.types.map(type=>({tag:type,selector:['shared'],strategy:{type}}))},
 outbounds:[{tag:'shared',protocol:'freedom'}],
 observatory:{subjectSelector:['shared'],probeURL:'https://example.test/ping',probeInterval:'11m',enableConcurrency:true,unknown:'keep'},
 burstObservatory:{subjectSelector:['shared'],pingConfig:{destination:'https://example.test/burst',interval:'30m',timeout:'10s',sampling:2,connectivity:'',httpMethod:'HEAD'},future:{value:1}}
});
test('opening and switching never changes saved config',()=>{
 const c=fixture(),before=P.clone(c);
 for(const type of P.types) P.read(c,type);
 assert.deepEqual(c,before);
});
test('four independent settings survive JSON reload and preserve legacy/unknown fields',()=>{
 let c=fixture();const before=P.clone(c);
 P.types.forEach((type,i)=>{
  const draft=P.read(c,type);
  if(type==='leastPing')draft.probeInterval='1m';else draft.pingConfig.interval=(i+1)+'m';
  c=P.update(c,type,draft);
 });
 c=JSON.parse(JSON.stringify(c));
 assert.deepEqual(c.observatory,before.observatory);assert.deepEqual(c.burstObservatory,before.burstObservatory);
 assert.equal(c.strategyObservatory.leastPing.probeInterval,'1m');
 assert.equal(c.strategyObservatory.leastLoad.pingConfig.interval,'2m');
 assert.equal(c.strategyObservatory.random.pingConfig.interval,'3m');
 assert.equal(c.strategyObservatory.roundRobin.pingConfig.interval,'4m');
 assert.equal(c.strategyObservatory.random.future.value,1);assert.equal(c.strategyObservatory.random.pingConfig.httpMethod,'HEAD');
});
test('changing strategy updates its scope and retains inactive settings',()=>{
 let c=fixture();c=P.update(c,'random',P.read(c,'random'));
 c.routing.balancers=c.routing.balancers.filter(b=>b.strategy.type!=='random');
 P.syncSelectors(c);assert.deepEqual(c.strategyObservatory.random.subjectSelector,[]);
 assert.equal(c.strategyObservatory.random.pingConfig.interval,'30m');
 c.routing.balancers.push({tag:'new',selector:['other'],strategy:{type:'random'}});
 P.syncSelectors(c);assert.deepEqual(c.strategyObservatory.random.subjectSelector,['other']);
});
test('selectors are deduplicated per strategy including implicit random',()=>{
 const c={routing:{balancers:[{selector:['a','a']},{selector:['b'],strategy:{type:'leastPing'}}]}};
 assert.deepEqual(P.selectors(c,'random'),['a']);assert.deepEqual(P.selectors(c,'leastPing'),['b']);
});
test('invalid duration/sample/url reject atomically',()=>{
 const c=fixture(),before=P.clone(c);
 for(const bad of ['','0s','-1s','10','10z','Infinitys','1s']) {
  const d=P.read(c,'random');d.pingConfig.interval=bad;
  assert.throws(()=>P.update(c,'random',d));
 }
 for(const bad of [0,1.2,1001,null]) {const d=P.read(c,'random');d.pingConfig.sampling=bad;assert.throws(()=>P.update(c,'random',d))}
 const d=P.read(c,'leastPing');d.probeURL='file:///etc/passwd';assert.throws(()=>P.update(c,'leastPing',d));
 assert.deepEqual(c,before);assert.equal(P.duration('1h2m3.5s'),3723.5);
});
test('all 13 supported languages contain complete native strings',()=>{
 assert.equal(Object.keys(L.maps).length,13);
 for(const [lang,dict]of Object.entries(L.maps)){
  for(const key of L.keys)assert.equal(typeof dict[key],'string',lang+':'+key);
  if(lang!=='en-US')assert.notEqual(dict.title,L.maps['en-US'].title);
 }
 assert.equal(L.get('zh-CN').interval,'检测间隔');
 assert.equal(L.get('unknown').interval,'Probe interval');
});
test('outbound sync follows all four probe scopes but ordinary rename does not',()=>{
 const ctx=vm.createContext({});
 vm.runInContext(fs.readFileSync(__dirname+'/../web/assets/js/util/outbound-references.js','utf8')+';this.refs=DuiOutboundReferences;',ctx);
 let c=fixture();for(const type of P.types)c=P.update(c,type,P.read(c,type));
 const next=ctx.refs.prepare(c,0,{tag:'renamed',protocol:'freedom'},'sync');
 for(const type of P.types)assert.deepEqual(Array.from(next.strategyObservatory[type].subjectSelector),['renamed']);
 const ordinary=ctx.refs.prepare(c,0,{tag:'renamed',protocol:'freedom'},'replace');
 for(const type of P.types)assert.deepEqual(Array.from(ordinary.strategyObservatory[type].subjectSelector),['shared']);
});
console.log(JSON.stringify({passed:true,cases}));
