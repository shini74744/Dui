const fs=require('fs'),path=require('path'),assert=require('assert/strict');
const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const root=path.resolve(__dirname,'..'),read=p=>fs.readFileSync(path.join(root,p),'utf8');
const {DuiUpdateText}=require(root+'/web/assets/js/util/updates.js');
for(const [lang,map] of Object.entries(DuiUpdateText.maps))for(const key of DuiUpdateText.keys)assert.equal(typeof map[key],'string',lang+key);
const sourceCard=read('web/html/index.html').match(/<a-card [^>]*class="glass-card dui-product-card"[\s\S]*?<\/a-card>/)[0];
const card=sourceCard.replace(/{{\s*\.cur_ver\s*}}/g,'26.9.49')
 .replace(/{{\s*i18n "pages.index.duiTitle"\s*}}/g,'〔DUI-PRO面板〕')
 .replace(/{{\s*i18n "pages.index.tgPrivateChat"\s*}}/g,'Shlii网站')
 .replace(/{{\s*i18n "pages.index.tgGroupChat"\s*}}/g,'〔DUI-PRO 面板〕交流群');
function html(theme,lang){
 const css=['web/assets/ant-design-vue/antd.min.css','web/assets/css/custom.min.css'].map(p=>'<style>'+read(p)+'</style>').join('');
 const libs=['web/assets/vue/vue.min.js','web/assets/ant-design-vue/antd.min.js'].map(p=>'<script>'+read(p)+'</script>').join('');
 const state={supported:true,items:{panel:{current:'v26.9.48',latest:'v26.9.48',available:false},core:{current:'vx-26.6',latest:'vx-26.6',available:false}},checking:false};
 return '<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1">'+css+
 '<style>body{margin:0;padding:20px;background:'+(theme==='dark'?'#212529':'#f0f2f7')+'}body.dark{color:#ddd}#app{max-width:560px}.dui-product-card{border-radius:24px}</style></head><body class="'+theme+'">'+libs+
 '<script>const LanguageManager={getLanguage:()=>'+JSON.stringify(lang)+'};let serverState='+JSON.stringify(state)+';let requests=[],checkHasOffers=true,failStatus=false;const clone=o=>JSON.parse(JSON.stringify(o));const axios={get:async()=>{if(failStatus)throw Error("network");return {data:{success:true,obj:clone(serverState)}}},post:async(url,data)=>{requests.push({url,data});if(url.endsWith("/start")){await new Promise(r=>setTimeout(r,500));serverState.job={id:"persisted",kind:data.kind,version:data.version,phase:"connecting",downloaded:0,total:60000000,busy:true};return {data:{success:true,obj:clone(serverState.job)}}}serverState.checking=true;setTimeout(()=>{serverState.checking=false;if(checkHasOffers){serverState.items.panel.latest="v26.9.49";serverState.items.panel.available=true;serverState.items.core.latest="vx-26.7";serverState.items.core.available=true}},180);return {data:{success:true,obj:clone(serverState)}}}};</script>'+
 '<script>'+read('web/assets/js/util/updates.js')+'</script><div id="app"><div v-if="visible">'+card+'</div></div><script>window.app=new Vue({el:"#app",data:{visible:true}});</script></body></html>';
}
async function noDialog(page){
 assert.equal(await page.locator('.ant-modal-root,[role="dialog"]').count(),0,'updates must never open a dialog');
}
async function noOverflow(page){
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'page overflow');
 const container=await page.locator('.dui-updates').boundingBox();
 for(const selector of ['.dui-update-check','.dui-update-entry','.dui-update-progress']){
  for(const e of await page.locator(selector).all()){
   const box=await e.boundingBox();if(box)assert(box.x>=container.x-1&&box.x+box.width<=container.x+container.width+1,selector+' overflow');
  }
 }
}
(async()=>{
 const browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH,args:['--no-sandbox']});
 const results=[],errors=[];
 const capture=async(page,name)=>{if(process.env.DUI_UPDATE_TEST_OUTPUT)await page.screenshot({path:process.env.DUI_UPDATE_TEST_OUTPUT+'/'+name+'.png',fullPage:true})};
 try{
 for(const width of [320,390,430,620,1366])for(const theme of ['','dark']){
  const page=await browser.newPage({viewport:{width,height:900},isMobile:width<500,hasTouch:width<500});
  page.on('pageerror',e=>errors.push(e.message));
  await page.setContent(html(theme,'zh-CN'));await page.waitForFunction(()=>app.$refs.updates.state.items.panel.current);
  assert.equal(await page.locator('.dui-update-inline').count(),0);
  assert.equal(await page.locator('.dui-update-check').innerText(),'检查更新');
  assert.equal(await page.locator('.dui-update-check.has-update').count(),0);
  await page.locator('.dui-update-check').click();
  assert.equal(await page.evaluate(()=>requests.filter(r=>r.url.endsWith('/check')).length),1,'one click must call check directly');
  assert.match(await page.locator('.dui-product-heading').innerText(),/正在检查/);
  await noDialog(page);
  // The initial idle poll schedules 60 seconds; checking must accelerate it to 2 seconds.
  await page.waitForFunction(()=>app.$refs.updates.state.items.panel.available,null,{timeout:5000});
  assert.equal(await page.locator('.dui-update-entry').count(),2);
  assert.equal(await page.locator('.dui-update-check').innerText(),'更新版本');
  assert.equal(await page.locator('.dui-update-check').evaluate(e=>getComputedStyle(e).color),theme?'rgb(98, 222, 197)':'rgb(8, 126, 109)');
  await noOverflow(page);await capture(page,'offers-'+width+'-'+(theme||'light'));
  const kind=theme?'core':'panel', row=page.locator('.dui-update-entry[data-kind="'+kind+'"]');
  await row.locator('.dui-update-install').click();
  await page.getByText('正在连接下载',{exact:true}).waitFor();
  assert.equal(await page.locator('.dui-update-entry .dui-update-install:disabled').count(),2,'all update actions disabled while starting');
  await page.waitForFunction(()=>serverState.job?.id==='persisted');
  await page.evaluate(()=>app.$refs.updates.poll());
  assert.deepEqual(await page.evaluate(()=>requests.filter(r=>r.url.endsWith('/start')).map(r=>r.data)),[{kind,version:kind==='panel'?'v26.9.49':'vx-26.7'}]);
  await page.evaluate(()=>{serverState.job.phase='downloading';serverState.job.downloaded=24500000});
  await page.waitForFunction(()=>app.$refs.updates.job.downloaded===24500000,null,{timeout:5000});
  assert.match(await page.locator('.dui-update-byte-count').innerText(),/40%/);
  await noDialog(page);await noOverflow(page);await capture(page,'progress-'+width+'-'+(theme||'light'));
  await page.evaluate(async()=>{app.visible=false;await app.$nextTick();app.visible=true;await app.$nextTick()});
  await page.locator('.dui-update-byte-count').waitFor();
  assert.match(await page.locator('.dui-update-byte-count').innerText(),/40%/,'remount resumes server-side progress');
  await page.evaluate(async()=>{failStatus=true;await app.$refs.updates.poll()});
  assert.match(await page.locator('.dui-update-inline').innerText(),/正在重连/);
  await page.evaluate(async()=>{failStatus=false;serverState.job.phase='rolled_back';serverState.job.busy=false;serverState.job.error='health_check_failed';await app.$refs.updates.poll()});
  assert.match(await page.locator('.dui-update-progress').innerText(),/已恢复原版本/);
  assert.equal(await row.locator('.dui-update-install').isEnabled(),true);
  await noDialog(page);await noOverflow(page);
  results.push({width,theme:theme||'light',directCheck:true,inlineDownload:true,progress:true,remount:true,reconnect:true,rollback:true});await page.close();
 }
 for(const lang of Object.keys(DuiUpdateText.maps)){
  const page=await browser.newPage({viewport:{width:390,height:900}});
  page.on('pageerror',e=>errors.push(e.message));
  await page.setContent(html('',lang));await page.waitForFunction(()=>app.$refs.updates.state.items.panel.current);
  await page.locator('.dui-update-check').click();await page.waitForFunction(()=>!serverState.checking);
  await page.evaluate(()=>app.$refs.updates.poll());
  await noDialog(page);await noOverflow(page);await page.close();
 }
 // Discovering an update must only change the header until the user opens it.
 const availablePage=await browser.newPage({viewport:{width:390,height:900}});
 availablePage.on('pageerror',e=>errors.push(e.message));
 await availablePage.setContent(html('dark','zh-CN'));await availablePage.waitForFunction(()=>app.$refs.updates.state.items.panel.current);
 await availablePage.evaluate(async()=>{serverState.items.core.available=true;serverState.items.core.latest='vx-26.7';await app.$refs.updates.poll()});
 assert.equal(await availablePage.locator('.dui-update-check').innerText(),'更新版本');
 assert.equal(await availablePage.locator('.dui-update-inline').count(),0,'do not automatically expand offers');
 await capture(availablePage,'available-collapsed-390-dark');
 await availablePage.locator('.dui-update-check').click();
 await availablePage.locator('.dui-update-inline').waitFor();
 assert.match(await availablePage.locator('.dui-update-entry[data-kind="core"]').innerText(),/vx-26.7/);
 assert.equal(await availablePage.evaluate(()=>requests.filter(r=>r.url.endsWith('/check')).length),0,'open known offer immediately');
 await noDialog(availablePage);await noOverflow(availablePage);await availablePage.close();
 const page=await browser.newPage({viewport:{width:390,height:900}});
 page.on('pageerror',e=>errors.push(e.message));
 await page.setContent(html('dark','zh-CN'));await page.waitForFunction(()=>app.$refs.updates.state.items.panel.current);
 await page.evaluate(()=>checkHasOffers=false);await page.locator('.dui-update-check').click();
 await page.waitForFunction(()=>!serverState.checking);await page.evaluate(()=>app.$refs.updates.poll());
 assert.equal(await page.locator('.dui-update-current-state').count(),2);
 assert.equal(await page.locator('.dui-update-check').innerText(),'检查更新');
 assert.equal(await page.locator('.dui-update-check.has-update').count(),0);
 await capture(page,'current-390-dark');
 await page.evaluate(async()=>{serverState.checkError='network_error';await app.$refs.updates.poll()});
 assert.equal(await page.locator('.dui-update-current-state').count(),0,'failed check cannot claim current');
 await page.evaluate(async()=>{serverState.checkError='';serverState.supported=false;serverState.items.core.available=true;serverState.items.core.latest='vx-26.7';await app.$refs.updates.poll()});
 assert.equal(await page.locator('.dui-update-install').count(),0,'unsupported install must not offer installation');
 assert.match(await page.locator('.dui-update-inline').innerText(),/此安装方式/);
 await page.evaluate(async()=>{serverState.supported=true;serverState.job={id:'unknown-length',kind:'core',phase:'downloading',busy:true,downloaded:1000,total:0};await app.$refs.updates.poll()});
 assert.equal(await page.locator('.dui-update-indeterminate').count(),1);
 assert.doesNotMatch(await page.locator('.dui-update-progress').innerText(),/%/,'unknown length must not invent percentage');
 await page.evaluate(async()=>{serverState.job={id:'done',kind:'panel',phase:'complete',version:'v26.9.49',busy:false,downloaded:1000,total:1000};await app.$refs.updates.poll()});
 assert.equal(await page.locator('.dui-update-reload').count(),1);
 await page.evaluate(()=>app.$refs.updates.show('core'));await noDialog(page);await noOverflow(page);
 await page.close();assert.deepEqual(errors,[]);
 console.log(JSON.stringify({cases:results,locales:13,greenUpdateLabel:true,offersExpandOnClick:true,unchangedWithoutUpdates:true,currentResult:true,checkFailure:true,unsupported:true,unknownLength:true,panelReload:true,noDialogs:true,errors},null,2));
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exit(1)});
