const fs=require('node:fs'),assert=require('node:assert/strict');
const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const base=process.env.DUI_FIXTURE_BASE,out=process.env.DUI_PROBE_TEST_OUTPUT||'/tmp';
(async()=>{
 const browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH,args:['--no-sandbox']});
 const errors=[],results=[];
 try{
 for(const width of [390,1440])for(const theme of ['light','dark']){
  const context=await browser.newContext({viewport:{width,height:950}});
  await context.addCookies([...JSON.parse(process.env.DUI_FIXTURE_COOKIES),{name:'lang',value:'zh-CN',url:base+'/'}]);
  await context.addInitScript(theme=>localStorage.setItem('dark-mode',String(theme==='dark')),theme);
  const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));
  await page.route('**/dui/xray/ruleSet/sourceMap',route=>route.fulfill({json:{success:true,obj:{}}}));
  await page.route('**/dui/xray/ruleSet/sourceMap/save',route=>route.fulfill({json:{success:true,obj:{}}}));
  let starts=0;
  await page.route('**/dui/xray/speedTest/start',route=>{starts++;return route.fulfill({json:{success:false,msg:'busy'}})});
  await page.goto(base+'/dui/xray#tpl-outbound');
  await page.waitForFunction(()=>typeof app!=='undefined'&&app.loadingStates.fetched);
  const original=await page.evaluate(()=>app.xraySetting);
  const row=page.locator('.dui-outbound-table tbody tr').filter({has:page.getByText('a',{exact:true})}).first();
  await row.locator('.ant-dropdown-trigger').click();
  await page.getByRole('menuitem',{name:'出口测速',exact:false}).click();
  await page.waitForFunction(()=>app.$refs.outboundSpeed.selected[0]==='a'&&!app.$refs.outboundSpeed.loading);
  const modal=page.locator('.dui-speed-modal .ant-modal-content');await modal.waitFor({state:'visible'});
  assert.equal(starts,0);
  assert.equal(await page.evaluate(()=>app.xraySetting),original);
  assert.equal(await page.evaluate(()=>app.$refs.outboundSpeed.theme),theme);
  assert(!(await modal.evaluate(e=>e.scrollWidth>e.clientWidth+1)));
  assert(await modal.getByRole('button',{name:'开始测速',exact:true}).isEnabled());
  await page.screenshot({path:out+'/menu-'+width+'-'+theme+'.png'});
  await modal.getByRole('button',{name:'关闭',exact:true}).click();await modal.waitFor({state:'hidden'});
  assert.equal(starts,0);results.push({width,theme});await context.close();
 }
 assert.deepEqual(errors,[]);
 fs.writeFileSync(out+'/speed-menu-browser.json',JSON.stringify({passed:true,results,errors},null,2));
 console.log(JSON.stringify({passed:true,cases:results.length,feature:'speed-menu-modal'}));
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exitCode=1});
