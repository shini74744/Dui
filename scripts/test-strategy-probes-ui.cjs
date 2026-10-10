const fs=require('fs'),assert=require('assert/strict');
const {chromium}=require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const root=require('path').resolve(__dirname,'..'),work=process.env.DUI_PROBE_TEST_OUTPUT || require('os').tmpdir();
const read=n=>fs.readFileSync(root+'/'+n,'utf8');
const P=require(root+'/web/assets/js/util/strategy-probes.js'),I=require(root+'/web/assets/js/util/strategy-probes-i18n.js');
const base={routing:{balancers:P.types.map(type=>({tag:type,selector:['shared'],strategy:{type}}))},outbounds:[{tag:'shared',protocol:'freedom'}],
 observatory:{subjectSelector:['shared'],probeURL:'https://example.test/ping',probeInterval:'10m',enableConcurrency:true},
 burstObservatory:{subjectSelector:['shared'],pingConfig:{destination:'https://example.test/check',interval:'30m',timeout:'10s',sampling:2,connectivity:''}}};
function html(lang,width,theme){
 const styles=['web/assets/ant-design-vue/antd.min.css','web/assets/css/custom.min.css'].map(p=>'<style>'+read(p)+'</style>').join('')+
 [...read('web/html/common/page.html').matchAll(/<style(?:[^>]*)>([\s\S]*?)<\/style>/g)].map(m=>m[0]).join('');
 const scripts=['web/assets/vue/vue.min.js','web/assets/ant-design-vue/antd.min.js','web/assets/js/util/strategy-probes.js','web/assets/js/util/strategy-probes-i18n.js'].map(p=>'<script>'+read(p)+'</script>').join('');
 const component=read('web/html/settings/xray/strategy_probes.html').replace(/\{\{define[^}]*\}\}/g,'').replace(/\{\{end\}\}/g,'');
 return '<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">'+styles+'<style>body.dark{background:#101114;color:rgba(255,255,255,.85)}</style></head><body class="'+theme+'">'+scripts+
 '<script>const LanguageManager={getLanguage:()=>'+JSON.stringify(lang)+'};</script>'+component+
 '<main id="app" style="padding:16px;max-width:1000px;margin:auto" class="'+theme+'"><a-button id="save" :disabled="!valid" @click="save">Save</a-button><dui-strategy-probes ref="probes" :key="revision" :value="config" @input="config=$event" @validity="valid=$event"></dui-strategy-probes></main>'+
 '<script>window.historyCalls=[];const HttpUtil={async get(url){const q=new URL(url,\"http://test.local\").searchParams;const strategy=q.get(\"strategy\"),page=Number(q.get(\"page\")),tag=q.get(\"outbound\");historyCalls.push({strategy,page,tag});await new Promise(resolve=>setTimeout(resolve,strategy===\"leastPing\"?80:5));return {success:true,obj:{rows:[{id:page*10,time:1780000000000,strategy,outbound:tag||strategy+\"-node\",status:\"success\",delay:12.3},{id:page*10+1,time:1780000001000,strategy,outbound:tag||strategy+\"-node\",status:\"network_unavailable\",delay:0}],page,total:55,tags:[strategy+\"-node\"],state:\"ok\",gap:false}}}};</script>'+
 '<script>window.app=new Vue({el:"#app",data:{config:'+JSON.stringify(base)+',valid:true,revision:0},methods:{save(){window.saved=JSON.stringify(this.config)},reload(){this.revision++;this.valid=true;this.config=JSON.parse(window.saved)}}});</script></body></html>';
}
async function openStrategy(page,name){
 const header=page.getByRole('tab',{name,exact:false});
 if(await header.getAttribute('aria-expanded')!=='true')await header.click();
}
(async()=>{
 const browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH || undefined,args:['--no-sandbox']});
 const errors=[],results=[];
 try{
 for(const lang of Object.keys(I.maps)){
  const page=await browser.newPage({viewport:{width:390,height:900}});
  page.on('pageerror',e=>{errors.push(e.message);console.error('BROWSER',e.message)});
  await page.setContent(html(lang,390,'dark'));await page.waitForSelector('.dui-probes');
  const txt=I.get(lang);
  assert.equal(await page.locator('.dui-probes h3').innerText(),txt.title);
  for(const type of P.types) assert.equal(await page.getByRole('tab',{name:txt[type],exact:false}).count(),1);
  await openStrategy(page,txt.leastPing);
  assert.equal(await page.locator('label[for="probe-interval-leastPing"]').innerText(),txt.interval);
  await openStrategy(page,txt.random);
  assert.equal(await page.locator('label[for="probe-timeout-random"]').innerText(),txt.timeout);
  assert.equal(await page.evaluate(()=>JSON.stringify(app.config)),JSON.stringify(base));
  await page.getByRole('button',{name:txt.history,exact:true}).click();
  await page.waitForSelector('.dui-probe-history .ant-table-row');
  assert(await page.locator('.dui-probe-history').getByText(txt.network_unavailable,{exact:true}).isVisible());
  results.push({case:'locale',lang});await page.close();
 }
 for(const width of [320,390,1100])for(const theme of ['', 'dark']){
  const page=await browser.newPage({viewport:{width,height:1000}});
  page.on('pageerror',e=>{errors.push(e.message);console.error('BROWSER',e.message)});await page.setContent(html('zh-CN',width,theme));
  const t=I.get('zh-CN');
  for(const [i,type]of P.types.entries()){
   await openStrategy(page,t[type]);
   await page.locator('#probe-interval-'+type).fill((i+1)+'m');
   await page.locator('#probe-interval-'+type).blur();
   await page.waitForFunction(([type,v])=>(app.config.strategyObservatory?.[type]?.probeInterval||app.config.strategyObservatory?.[type]?.pingConfig?.interval)===v,[type,(i+1)+'m']);
  }
  await page.locator('#save').click();await page.evaluate(()=>app.reload());
  for(const [i,type]of P.types.entries()){
   await openStrategy(page,t[type]);
   assert.equal(await page.locator('#probe-interval-'+type).inputValue(),(i+1)+'m');
  }
  const config=await page.evaluate(()=>app.config);
  assert.deepEqual(config.observatory,base.observatory);assert.deepEqual(config.burstObservatory,base.burstObservatory);
  await openStrategy(page,t.random);
  await page.locator('#probe-interval-random').fill('1s');await page.locator('#probe-interval-random').blur();
  assert(await page.locator('#save').isDisabled());
  await openStrategy(page,t.leastPing);assert(await page.locator('#save').isDisabled());
  await openStrategy(page,t.random);assert.equal(await page.locator('#probe-interval-random').inputValue(),'1s');
  await page.locator('#probe-interval-random').fill('45s');await page.locator('#probe-interval-random').blur();
  assert(await page.locator('#save').isEnabled());
  await page.getByText(t.advanced,{exact:true}).click();await page.getByRole('textbox',{name:t.advanced}).fill('{');
  await page.getByRole('button',{name:t.apply,exact:true}).click();assert(await page.locator('#save').isDisabled());
  await page.getByRole('button',{name:t.reset,exact:true}).click();assert(await page.locator('#save').isEnabled());
  let raw=JSON.parse(await page.getByRole('textbox',{name:t.advanced}).inputValue());
  raw.pingConfig.interval='2m';raw.pingConfig.httpMethod='HEAD';raw.customFuture={keep:true};
  await page.getByRole('textbox',{name:t.advanced}).fill(JSON.stringify(raw));
  await page.getByRole('button',{name:t.apply,exact:true}).click();
  await page.getByText(t.advanced,{exact:true}).click();
  assert.equal(await page.locator('#probe-interval-random').inputValue(),'2m');
  await page.locator('#probe-timeout-random').fill('7s');await page.locator('#probe-timeout-random').blur();
  assert.equal(await page.evaluate(()=>app.config.strategyObservatory.random.customFuture.keep),true);
  assert.equal(await page.evaluate(()=>app.config.strategyObservatory.leastLoad.pingConfig.interval),'2m');
  await page.getByRole('button',{name:t.history,exact:true}).click();
  await page.waitForSelector('.dui-probe-history .ant-table-row');
  assert(await page.locator('.dui-probe-history').getByText('12.3 ms',{exact:true}).isVisible());
  await page.getByRole('button',{name:t.nextHistory,exact:true}).click();
  await page.waitForFunction(()=>app.$refs.probes.history.page===2);
  await openStrategy(page,t.leastPing);
  await openStrategy(page,t.roundRobin);
  await page.waitForFunction(()=>app.$refs.probes.history.rows[0]?.strategy==='roundRobin');
  await page.waitForTimeout(100);
  assert.equal(await page.evaluate(()=>app.$refs.probes.history.rows[0].strategy),'roundRobin');
  await page.getByRole('button',{name:t.refreshHistory,exact:true}).click();
  await page.waitForFunction(()=>!app.$refs.probes.historyLoading);
  assert.equal(await page.evaluate(()=>app.$refs.probes.history.page),1);
  const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1);
  assert(!overflow,'horizontal overflow at '+width);
  if(theme==='dark'&&(width===390||width===1100))await page.screenshot({path:work+'/strategy-probes-'+width+'.png',fullPage:true});
  results.push({case:'edit-reload-invalid-json-isolation-layout',width,theme:theme||'light'});await page.close();
 }
 assert.deepEqual(errors,[]);
 fs.writeFileSync(work+'/browser-results.json',JSON.stringify({passed:true,cases:results.length,results,errors},null,2));
 console.log(JSON.stringify({passed:true,cases:results.length,errors}));
 }finally{await browser.close()}
})().catch(e=>{console.error(e.stack);process.exit(1)});
