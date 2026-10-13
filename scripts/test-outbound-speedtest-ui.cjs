const fs=require('fs'),assert=require('assert/strict'),path=require('path');
const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const root=path.resolve(__dirname,'..'),dir=process.env.DUI_PROBE_TEST_OUTPUT||require('os').tmpdir();
const read=p=>fs.readFileSync(root+'/'+p,'utf8');
const I=require(root+'/web/assets/js/util/outbound-speedtest-i18n.js');
function html(lang,theme){
 const styles=['web/assets/ant-design-vue/antd.min.css','web/assets/css/custom.min.css'].map(p=>'<style>'+read(p)+'</style>').join('')+
 [...read('web/html/common/page.html').matchAll(/<style(?:[^>]*)>([\s\S]*?)<\/style>/g)].map(m=>m[0]).join('');
 const scripts=['web/assets/vue/vue.min.js','web/assets/ant-design-vue/antd.min.js','web/assets/js/util/outbound-speedtest-i18n.js'].map(p=>'<script>'+read(p)+'</script>').join('');
 const component=read('web/html/settings/xray/outbound_speedtest.html').replace(/\{\{define[^}]*\}\}/g,'').replace(/\{\{end\}\}/g,'');
 const mock=`
 let state={state:'idle',rows:[]};window.requests=[];
 const measurement=(state='testing')=>({state,bytes:26000000,seconds:3.1,mbps:67.1,currentMbps:70.2,progress:31,limitReached:false});
 const row=tag=>({tag,state:'testing',phase:'download',download:measurement(),upload:{...measurement('queued'),bytes:0,progress:0},ping:{state:'done',currentMs:28.4,averageMs:28.4,minMs:25.1,maxMs:31.2,jitterMs:2.3,samples:4,failures:0},progress:15});
 window.uploadPhase=()=>{state.rows.forEach(r=>{r.phase='upload';r.download={...measurement('done'),progress:100,currentMbps:0};r.upload=measurement();r.ping.currentMs=32.1})};
 const HttpUtil={
  async get(url){if(url.endsWith('catalog'))return {success:true,obj:[{tag:'Hong-Kong-Node-A-Long-Label',protocol:'shadowsocks'},{tag:'Tokyo-Node-B',protocol:'vless'}]};return {success:true,obj:JSON.parse(JSON.stringify(state))}},
  async post(url,data){requests.push({url,data});if(url.endsWith('start')){const p=JSON.parse(data.request);state={id:'job1',state:'running',startedAt:1780000000000,threads:p.threads,rows:p.tags.map(row)}}else{state.state='cancelled';state.rows.forEach(r=>r.state='cancelled')};return {success:true,obj:JSON.parse(JSON.stringify(state))}}
 };
 window.complete=()=>{state.state='done';state.rows.forEach(r=>{r.state='done';r.phase='';r.download.state=r.upload.state='done';r.download.progress=r.upload.progress=100;r.download.currentMbps=r.upload.currentMbps=0})};
 window.partial=()=>{state={id:'job1',state:'done',startedAt:1780000000000,threads:1,rows:[{tag:'Hong-Kong-Node-A-Long-Label',state:'partial',bytes:200000000,seconds:2.3,mbps:696.76,currentMbps:0,progress:23,limitReached:false,error:'stream_failed'}]}};
 window.app=new Vue({el:'#app',data:{dirty:false,theme:${JSON.stringify(theme)}}});
 `;
 return '<html><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">'+styles+'<style>body.dark{background:#101114;color:rgba(255,255,255,.85)}</style></head><body class="'+theme+'">'+scripts+'<script>const LanguageManager={getLanguage:()=>'+JSON.stringify(lang)+'};</script>'+component+
 '<main id="app" class="'+theme+'" style="max-width:1400px;margin:auto;padding:16px"><dui-outbound-speedtest ref="speed" :dirty="dirty" :theme="theme"></dui-outbound-speedtest><div style="height:1600px"></div><button id="row-test" @click="$refs.speed.open(\'Tokyo-Node-B\')">Row speed test</button><div style="height:1000px"></div></main><script>'+mock+'</script></body></html>';
}
(async()=>{
 const browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||undefined,args:['--no-sandbox']});
 const errors=[],results=[];
 try{
 for(const width of [320,390,768,1440])for(const theme of ['','dark']){
  const page=await browser.newPage({viewport:{width,height:1000}});
  page.setDefaultTimeout(7000);page.on('pageerror',e=>errors.push(e.message));
  await page.setContent(html('zh-CN',theme));const t=I.get('zh-CN');
  await page.locator('#row-test').scrollIntoViewIfNeeded();
  const before=await page.evaluate(()=>scrollY);
  await page.locator('#row-test').click();
  await page.waitForSelector('.dui-speed-modal .ant-modal');
  await page.waitForFunction(()=>app.$refs.speed.selected[0]==='Tokyo-Node-B');
  assert.equal(await page.evaluate(()=>requests.length),0,'opening starts no traffic');
  await page.getByText(t.multi,{exact:true}).click();
  await page.locator('#dui-speed-threads').click();
  await page.locator('.ant-select-dropdown-menu-item').filter({hasText:/^16$/}).click();
  await page.evaluate(()=>app.dirty=true);assert(await page.getByRole('button',{name:t.start,exact:true}).isDisabled());
  await page.evaluate(()=>app.dirty=false);await page.getByRole('button',{name:t.start,exact:true}).click();
  await page.waitForSelector('.dui-speed-card');
  const req=await page.evaluate(()=>JSON.parse(requests[0].data.request));assert.deepEqual(req,{tags:['Tokyo-Node-B'],threads:16});
  assert.equal(await page.locator('.dui-speed-transfer').count(),2);
  assert((await page.locator('.dui-speed-ping-value').innerText()).includes('28.40'));
  assert(await page.getByRole('button',{name:t.start,exact:true}).isDisabled());
  const overflow=()=>[...document.querySelectorAll('.ant-modal-body,.dui-speed-card,.dui-speed-fields')].some(e=>e.scrollWidth>e.clientWidth+1);
  assert(!await page.evaluate(overflow),'modal overflow '+width);
  if([390,1440].includes(width))await page.screenshot({path:dir+'/duplex-'+width+'-'+(theme||'light')+'.png'});
  await page.getByRole('button',{name:t.close,exact:true}).click();
  await page.waitForSelector('.dui-speed-modal .ant-modal',{state:'hidden'});
  assert.equal(await page.evaluate(()=>scrollY),before,'closing must preserve row position');
  await page.locator('#row-test').click();
  await page.waitForSelector('.dui-speed-card');assert.equal(await page.evaluate(()=>requests.length),1,'reopen starts no duplicate job');
  await page.evaluate(()=>uploadPhase());await page.getByRole('button',{name:t.refresh,exact:true}).click();
  assert((await page.locator('.dui-speed-ping-value').innerText()).includes('32.10'));
  assert((await page.locator('.dui-speed-transfer').nth(1).innerText()).includes('70.20'));
  await page.getByRole('button',{name:t.stop,exact:true}).click();
  await page.waitForFunction(()=>app.$refs.speed.job.state==='cancelled');
  await page.getByText(t.single,{exact:true}).click();await page.getByRole('button',{name:t.start,exact:true}).click();
  assert.equal(await page.evaluate(()=>JSON.parse(requests.at(-1).data.request).threads),1);
  await page.evaluate(()=>uploadPhase());await page.evaluate(()=>complete());await page.getByRole('button',{name:t.refresh,exact:true}).click();
  assert.equal(await page.evaluate(()=>app.$refs.speed.resultState),'done');
  assert((await page.locator('.dui-speed-live').first().innerText()).includes('—'));
  await page.evaluate(()=>partial());await page.getByRole('button',{name:t.refresh,exact:true}).click();
  assert.equal(await page.evaluate(()=>app.$refs.speed.resultState),'partial');
  assert((await page.locator('.dui-speed-progress-label').first().innerText()).includes('23%'));
  assert(!await page.evaluate(overflow),'partial overflow '+width);
  await page.evaluate(()=>app.$refs.speed.open('unsupported-row'));
  assert.equal(await page.evaluate(()=>app.$refs.speed.selected.length),0);
  assert.equal(await page.evaluate(()=>app.$refs.speed.error),'unsupported_outbound');
  assert(await page.getByRole('button',{name:t.start,exact:true}).isDisabled());
  results.push({width,theme:theme||'light'});await page.close();
 }
 for(const lang of Object.keys(I.maps)){
  const page=await browser.newPage({viewport:{width:390,height:900}});page.setDefaultTimeout(7000);page.on('pageerror',e=>errors.push(e.message));
  await page.setContent(html(lang,'dark'));await page.getByRole('button',{name:I.get(lang).title,exact:true}).click();
  assert(await page.getByRole('button',{name:I.get(lang).start,exact:true}).isVisible());
  assert(!await page.evaluate(()=>document.querySelector('.ant-modal-body').scrollWidth>document.querySelector('.ant-modal-body').clientWidth+1),'locale overflow '+lang);
  await page.close();
 }
 assert.deepEqual(errors,[]);console.log(JSON.stringify({passed:true,layouts:results,locales:Object.keys(I.maps).length,errors}));
 }finally{await browser.close()}
})().catch(e=>{console.error(e.stack);process.exit(1)});
