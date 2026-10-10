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
 const row=tag=>({tag,state:'testing',bytes:26000000,seconds:3.1,mbps:67.1,currentMbps:70.2,progress:31,limitReached:false});
 const HttpUtil={
  async get(url){if(url.endsWith('catalog'))return {success:true,obj:[{tag:'Hong-Kong-Node-A-Long-Label',protocol:'shadowsocks'},{tag:'Tokyo-Node-B',protocol:'vless'}]};return {success:true,obj:JSON.parse(JSON.stringify(state))}},
  async post(url,data){requests.push({url,data});if(url.endsWith('start')){const p=JSON.parse(data.request);state={id:'job1',state:'running',threads:p.threads,rows:p.tags.map(row)}}else{state.state='cancelled';state.rows.forEach(r=>r.state='cancelled')};return {success:true,obj:JSON.parse(JSON.stringify(state))}}
 };
 window.complete=()=>{state.state='done';state.rows.forEach(r=>{r.state='done';r.progress=100;r.seconds=10;r.currentMbps=0})};
 window.app=new Vue({el:'#app',data:{dirty:false}});
 `;
 return '<html><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">'+styles+'<style>body.dark{background:#101114;color:rgba(255,255,255,.85)}</style></head><body class="'+theme+'">'+scripts+'<script>const LanguageManager={getLanguage:()=>'+JSON.stringify(lang)+'};</script>'+component+
 '<main id="app" class="'+theme+'" style="max-width:1400px;margin:auto;padding:16px"><dui-outbound-speedtest ref="speed" :dirty="dirty"></dui-outbound-speedtest></main><script>'+mock+'</script></body></html>';
}
(async()=>{
 const browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||undefined,args:['--no-sandbox']});
 const errors=[],results=[];
 try{
 for(const width of [320,390,1440])for(const theme of ['','dark']){
  const page=await browser.newPage({viewport:{width,height:1000}});
  page.setDefaultTimeout(7000);page.setDefaultNavigationTimeout(10000);page.on('pageerror',e=>{errors.push(e.message);console.error('BROWSER',e.message)});await page.setContent(html('zh-CN',theme));const t=I.get('zh-CN');
  await page.getByRole('button',{name:t.title,exact:true}).click();
  await page.locator('#dui-speed-tags').click();
  await page.locator('.ant-select-dropdown-menu-item').filter({hasText:'Hong-Kong-Node-A-Long-Label'}).click();
  await page.locator('.ant-select-dropdown-menu-item').filter({hasText:'Tokyo-Node-B'}).click();
  await page.keyboard.press('Escape');await page.locator('.dui-speed-head h3').click();
  await page.getByText(t.multi,{exact:true}).click();
  await page.locator('#dui-speed-threads').click();await page.locator('.ant-select-dropdown-menu-item').filter({hasText:/^8$/}).click();
  await page.evaluate(()=>app.dirty=true);assert(await page.getByRole('button',{name:t.start,exact:true}).isDisabled());
  await page.evaluate(()=>app.dirty=false);await page.getByRole('button',{name:t.start,exact:true}).click();
  await page.waitForSelector('.dui-speed-card');
  const req=await page.evaluate(()=>JSON.parse(requests[0].data.request));assert.equal(req.threads,8);assert.equal(req.tags.length,2);
  assert.equal(await page.locator('.dui-speed-card').count(),2);
  assert(await page.getByRole('button',{name:t.start,exact:true}).isDisabled());
  assert(await page.getByRole('button',{name:t.stop,exact:true}).isVisible());
  assert(!(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1)),'overflow at '+width);
  // Collapsing/reopening does not start a second test or lose the running task.
  await page.getByRole('button',{name:t.title,exact:true}).click();await page.getByRole('button',{name:t.title,exact:true}).click();
  assert.equal(await page.evaluate(()=>requests.length),1);
  if(width!==320)await page.screenshot({path:dir+'/speedtest-'+width+'-'+(theme||'light')+'.png',fullPage:true});
  await page.getByRole('button',{name:t.stop,exact:true}).click();
  await page.waitForFunction(()=>app.$refs.speed.job.state==='cancelled');
  await page.getByText(t.single,{exact:true}).click();await page.getByRole('button',{name:t.start,exact:true}).click();
  assert.equal(await page.evaluate(()=>JSON.parse(requests.at(-1).data.request).threads),1);
  await page.evaluate(()=>complete());await page.getByRole('button',{name:t.refresh,exact:true}).click();
  await page.waitForFunction(()=>app.$refs.speed.job.state==='done');
  await page.evaluate(()=>app.$refs.speed.open('unsupported-row'));
  assert.equal(await page.evaluate(()=>app.$refs.speed.selected.length),0);
  assert.equal(await page.evaluate(()=>app.$refs.speed.error),'unsupported_outbound');
  assert(await page.getByRole('button',{name:t.start,exact:true}).isDisabled());
  results.push({width,theme:theme||'light'});await page.close();
 }
 for(const lang of Object.keys(I.maps)){
  const page=await browser.newPage({viewport:{width:390,height:900}});page.setDefaultTimeout(7000);page.setDefaultNavigationTimeout(10000);page.on('pageerror',e=>{errors.push(e.message);console.error('BROWSER',e.message)});
  await page.setContent(html(lang,'dark'));await page.getByRole('button',{name:I.get(lang).title,exact:true}).click();
  assert(await page.getByRole('button',{name:I.get(lang).start,exact:true}).isVisible());await page.close();
 }
 assert.deepEqual(errors,[]);console.log(JSON.stringify({passed:true,layouts:results,locales:Object.keys(I.maps).length,errors}));
 }finally{await browser.close()}
})().catch(e=>{console.error(e.stack);process.exit(1)});
