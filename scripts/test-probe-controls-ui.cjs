const fs=require('node:fs'),assert=require('node:assert/strict');
const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const base=process.env.DUI_FIXTURE_BASE,out=process.env.DUI_PROBE_TEST_OUTPUT||'/tmp';
(async()=>{
 const browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH,args:['--no-sandbox']});
 const errors=[],results=[];
 try{
 for(const width of JSON.parse(process.env.DUI_PROBE_UI_WIDTHS||'[320,390,1440]'))for(const theme of JSON.parse(process.env.DUI_PROBE_UI_THEMES||'["light","dark"]')){
  const context=await browser.newContext({viewport:{width,height:950}});
  await context.addCookies([...JSON.parse(process.env.DUI_FIXTURE_COOKIES),{name:'lang',value:'zh-CN',url:base+'/'}]);
  await context.addInitScript(theme=>localStorage.setItem('dark-mode',String(theme==='dark')),theme);
  const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));
  await page.route('**/dui/xray/ruleSet/sourceMap',route=>route.fulfill({json:{success:true,obj:{}}}));
  await page.route('**/dui/xray/ruleSet/sourceMap/save',route=>route.fulfill({json:{success:true,obj:{}}}));
  await page.goto(base+'/dui/xray#tpl-outbound');
  await page.waitForFunction(()=>typeof app!=='undefined'&&app.loadingStates.fetched);
  const original=await page.evaluate(()=>app.xraySetting);
  const aButton=page.getByRole('button',{name:'查看探测 · a',exact:true});
  const buttonBox=await aButton.boundingBox(),cellBox=await aButton.locator('xpath=ancestor::td').boundingBox();
  assert(buttonBox.x>=cellBox.x && buttonBox.x+buttonBox.width<=cellBox.x+cellBox.width,'probe button outside action cell');
  await aButton.click();
  const modal=page.locator('.ant-modal-content').filter({has:page.locator('.dui-probe-detail')});
  await modal.waitFor({state:'visible'});
  await page.waitForFunction(()=>!app.$refs.outboundProbe.loading);
  assert((await page.evaluate(()=>app.$refs.outboundProbe.history.total))>=3);
  assert((await page.evaluate(()=>app.$refs.outboundProbe.history.rows)).every(r=>r.outbound==='a'));
  assert((await modal.innerText()).includes('单独出口探测'));
  assert((await modal.innerText()).includes('轮询'));
  assert.equal(await page.evaluate(()=>app.xraySetting),original);
  // Switch outbound while its previous request is still in flight: no stale response should win.
  await page.evaluate(()=>{app.$refs.outboundProbe.open('a');app.$refs.outboundProbe.open('b')});
  await page.waitForFunction(()=>!app.$refs.outboundProbe.loading);
  assert.equal(await page.evaluate(()=>app.$refs.outboundProbe.tag),'b');
  assert((await page.evaluate(()=>app.$refs.outboundProbe.history.rows)).every(r=>r.outbound==='b'));
  await page.evaluate(()=>app.$refs.outboundProbe.open('unused-node'));
  await page.waitForFunction(()=>!app.$refs.outboundProbe.loading);
  assert.equal(await page.evaluate(()=>app.$refs.outboundProbe.history.total),0);
  assert((await modal.innerText()).includes('此出口当前没有运行中的探测'));
  await page.evaluate(()=>app.$refs.outboundProbe.open('a'));
  await page.waitForFunction(()=>!app.$refs.outboundProbe.loading);
  if(width<576){assert(await modal.locator('.dui-probe-records-mobile').isVisible());assert(await modal.locator('.dui-probe-record-data strong').first().isVisible())}
  const box=await modal.boundingBox();assert(box.x>=-1&&box.x+box.width<=width+1,'detail outside viewport');
  assert(!(await modal.evaluate(el=>el.scrollWidth>el.clientWidth+1)),'detail overflow');
  if(theme==='dark'){await page.waitForTimeout(300);await page.screenshot({path:out+'/probe-detail-'+width+'.png',fullPage:true})}
  await modal.locator('.ant-modal-close').click();await modal.waitFor({state:'hidden'});
  if(theme==='dark'){await page.waitForTimeout(200);await page.screenshot({path:out+'/probe-buttons-'+width+'.png',fullPage:true})}
  await page.evaluate(()=>app.changePage('tpl-balancer'));
  await page.getByRole('tab',{name:'单独出口探测',exact:false}).click();
  const toggle=page.locator('#probe-enabled-outbound');
  assert(await toggle.isDisabled(),'Fallback must prevent disabling');
  // Stage a fixture with no Fallback and a user-defined interval and extension.
  await page.evaluate(()=>{
    const c=DuiStrategyProbes.clone(app.templateSettings);
    c.routing.rules.forEach(r=>delete r.fallbackTag);
    c.strategyObservatory.outbound.pingConfig.interval='17m';
    c.strategyObservatory.outbound.future={keep:true};app.templateSettings=c;
  });
  await toggle.click();assert.equal(await toggle.getAttribute('aria-checked'),'false');
  assert((await page.locator('.dui-probe-enable').innerText()).includes('待保存'));
  assert.equal(await page.locator('#probe-interval-outbound').inputValue(),'17m');
  await page.locator('#probe-interval-outbound').fill('19m');
  assert.equal(await toggle.getAttribute('aria-checked'),'false','editing parameters unexpectedly enabled');
  await page.evaluate(()=>app.updateXraySetting());
  await page.reload();await page.waitForFunction(()=>typeof app!=='undefined'&&app.loadingStates.fetched);
  await page.getByRole('tab',{name:'单独出口探测',exact:false}).click();
  assert.equal(await toggle.getAttribute('aria-checked'),'false');
  assert.equal(await page.locator('#probe-interval-outbound').inputValue(),'19m');
  assert((await page.locator('.dui-probe-enable').innerText()).includes('未启用'));
  await toggle.click();
  assert.equal(await page.locator('#probe-interval-outbound').inputValue(),'19m');
  assert(await page.evaluate(()=>app.templateSettings.strategyObservatory.outbound.future.keep));
  if(theme==='dark'){await page.waitForTimeout(200);await page.screenshot({path:out+'/probe-switch-'+width+'.png',fullPage:true})}
  await page.evaluate(()=>app.updateXraySetting());
  await page.reload();await page.waitForFunction(()=>typeof app!=='undefined'&&app.loadingStates.fetched);
  await page.getByRole('tab',{name:'单独出口探测',exact:false}).click();
  assert.equal(await toggle.getAttribute('aria-checked'),'true');
  assert((await page.locator('.dui-probe-enable').innerText()).includes('配置已启用'));
  const now=await page.evaluate(()=>app.templateSettings);
  const old=JSON.parse(original);
  assert.deepEqual(now.burstObservatory,old.burstObservatory);
  assert.deepEqual(now.strategyObservatory.roundRobin,old.strategyObservatory.roundRobin);
  const restored=await page.evaluate(async original=>await HttpUtil.post('/dui/xray/update',{xraySetting:original}),original);
  assert(restored.success,'failed restoring isolated fixture');
  results.push({width,theme,passed:true});await context.close();
 }
 assert.deepEqual(errors,[]);
 fs.writeFileSync(out+'/probe-controls-browser.json',JSON.stringify({passed:true,results,errors},null,2));
 console.log(JSON.stringify({passed:true,cases:results.length,feature:'probe-controls'}));
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exitCode=1});
