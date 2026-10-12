const fs=require('node:fs'),assert=require('node:assert/strict');
const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const base=process.env.DUI_FIXTURE_BASE;
const out=process.env.DUI_PROBE_TEST_OUTPUT||'/tmp';
const longTag='测试出口-very-long-outbound-name-for-mobile-layout-'.repeat(3);
(async()=>{
 const browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH,args:['--no-sandbox']});
 const results=[],errors=[];
 try{
  for(const width of [320,390,1440])for(const theme of ['light','dark']){
   const context=await browser.newContext({viewport:{width,height:1000}});
   await context.addCookies([...JSON.parse(process.env.DUI_FIXTURE_COOKIES),{name:'lang',value:'zh-CN',url:base+'/'}]);
   await context.addInitScript(theme=>localStorage.setItem('dark-mode',String(theme==='dark')),theme);
   const page=await context.newPage();
   page.on('pageerror',e=>{errors.push(e.message);console.error('BROWSER',e.message)});
   await page.goto(base+'/dui/xray?fixture=1#tpl-outbound');
   await page.waitForFunction(()=>typeof app!=='undefined'&&app.loadingStates.fetched);
   assert.equal(await page.evaluate(()=>app.activePage),'tpl-outbound');
   const keys=width===1440&&theme==='dark'?['tpl-basic','tpl-routing','tpl-outbound','tpl-reverse','tpl-balancer','tpl-dns','tpl-blacklist','tpl-advanced','tpl-kernel']:['tpl-balancer','tpl-routing'];
   for(const key of keys){
    await page.evaluate(key=>app.changePage(key),key);
    await page.reload();
    await page.waitForFunction(key=>typeof app!=='undefined'&&app.loadingStates.fetched&&app.activePage===key,key);
    assert.equal(new URL(page.url()).searchParams.get('fixture'),'1');
    assert(new URL(page.url()).pathname.startsWith('/fixture/'));
    if(key==='tpl-advanced')await page.waitForSelector('.CodeMirror');
   }
   assert.equal(await page.evaluate(()=>themeSwitcher.currentTheme),theme);
   await page.evaluate(()=>app.changePage('tpl-balancer'));
   await page.evaluate(tag=>{const cfg=app.templateSettings;cfg.outbounds.push({tag,protocol:'freedom'});app.templateSettings=cfg;app.addBalancer()},longTag);
   const modal=page.locator('#balancer-modal .ant-modal-content');
   await modal.waitFor({state:'visible'});
   await modal.locator('input.ant-input').first().fill('ui-tier-'+width+'-'+theme);
   await modal.locator('.ant-select').first().click();
   await page.locator('.ant-select-dropdown:visible').getByRole('option',{name:'自定义（分级优先）',exact:true}).click();
   await page.waitForSelector('.dui-priority-level');
   await page.waitForFunction(()=>[...document.querySelectorAll('.ant-select-dropdown')].every(el=>el.offsetParent===null));
   assert.equal(await page.locator('.dui-priority-level').count(),2);
   async function selectTag(index,tag){
    const level=page.locator('.dui-priority-level').nth(index);
    await level.locator('.ant-select').nth(1).click();
    await page.locator('.ant-select-dropdown:visible .ant-select-dropdown-menu-item').filter({hasText:new RegExp('^'+tag+'$')}).click();
    await page.keyboard.press('Escape');
    await page.waitForFunction(()=>[...document.querySelectorAll('.ant-select-dropdown')].every(el=>el.offsetParent===null));
   }
   await selectTag(0,'a');await selectTag(0,'b');await selectTag(0,longTag);await selectTag(1,'c');
   await page.locator('.dui-priority-editor').getByRole('button',{name:/添加一级/}).click();
   await selectTag(2,'direct');
   await page.locator('.dui-priority-level').nth(2).getByRole('button',{name:'上移',exact:true}).click();
   assert.deepEqual(await page.evaluate(()=>balancerModal.balancer.levels.map(l=>l.selector)),[['a','b',longTag],['direct'],['c']]);
   await page.locator('.dui-priority-level').nth(1).getByRole('button',{name:'下移',exact:true}).click();
   const modalOverflow=await modal.evaluate(el=>el.scrollWidth>el.clientWidth+1);
   assert(!modalOverflow,'modal overflow '+width+' '+theme);
   const box=await modal.boundingBox();assert(box.x>=-1&&box.x+box.width<=width+1,'modal outside viewport');
   if(theme==='dark'){await page.waitForTimeout(350);await page.screenshot({path:out+'/priority-'+width+'.png',fullPage:true})}
   await modal.locator('.ant-modal-footer .ant-btn-primary').click();
   await modal.waitFor({state:'hidden'});
   const newBalancer=await page.evaluate(()=>app.templateSettings.routing.balancers.at(-1));
   assert.equal(newBalancer.strategy.type,'priority');
   assert.deepEqual(newBalancer.strategy.settings.levels.map(l=>l.selector),[['a','b',longTag],['c'],['direct']]);
   await page.evaluate(()=>app.editBalancer(app.balancersData.length-1));
   await modal.waitFor({state:'visible'});
   assert.deepEqual(await page.evaluate(()=>balancerModal.balancer.levels.map(l=>l.selector)),[['a','b',longTag],['c'],['direct']]);
   await modal.locator('.ant-modal-footer .ant-btn').first().click();
   // Fallback is visible for standalone routes and the draft survives confirm.
   await page.evaluate(()=>{app.changePage('tpl-routing');app.editRule(1)});
   const rule=page.locator('#rule-modal .ant-modal-content');
   await rule.waitFor({state:'visible'});
   assert.equal(await page.evaluate(()=>ruleModal.rule.fallbackTag),'b');
   assert(await rule.getByText('Fallback',{exact:true}).isVisible());
   if(theme==='dark'){await rule.getByText('Fallback',{exact:true}).scrollIntoViewIfNeeded();await page.waitForTimeout(350);await page.screenshot({path:out+'/fallback-'+width+'.png',fullPage:true})}
   const ruleBox=await rule.boundingBox();assert(ruleBox.x>=-1&&ruleBox.x+ruleBox.width<=width+1,'rule modal outside viewport');
   await rule.locator('.ant-modal-footer .ant-btn-primary').last().click();
   await rule.waitFor({state:'hidden'});
   assert.equal(await page.evaluate(()=>app.templateSettings.routing.rules.find(r=>r.outboundTag==='a')?.fallbackTag),'b');
   await page.evaluate(()=>app.changePage('tpl-balancer'));
   const standalone=page.locator('.dui-probe-accordion .ant-collapse-header').last();
   await standalone.click();
   assert.equal(await page.locator('#probe-interval-outbound').inputValue(),'1m');
   // Current UI no longer displays the legacy badge.
   assert.equal(await page.getByText('旧版共享探测',{exact:true}).count(),0);
   // New consumers must preserve the user's existing legacy interval.
   await page.evaluate(()=>{
    const cfg=DuiStrategyProbes.clone(app.templateSettings);
    delete cfg.strategyObservatory;
    cfg.routing.balancers=[{tag:'legacy-round-robin',selector:['a'],strategy:{type:'roundRobin'}}];
    cfg.burstObservatory={subjectSelector:['a'],pingConfig:{destination:'http://example.test/',interval:'30m',timeout:'5s',sampling:2,connectivity:''}};
    app.templateSettings=cfg;app.addBalancer();
    balancerModal.balancer.selector=['a'];
   });
   await modal.waitFor({state:'visible'});
   await modal.locator('input.ant-input').first().fill('new-default-random');
   await modal.locator('.ant-modal-footer .ant-btn-primary').click();
   await modal.waitFor({state:'hidden'});
   assert.equal(await page.evaluate(()=>DuiStrategyProbes.read(app.templateSettings,'random').pingConfig.interval),'30m');
   assert.equal(await page.evaluate(()=>DuiStrategyProbes.read(app.templateSettings,'roundRobin').pingConfig.interval),'30m');
   await page.evaluate(()=>{app.editBalancer(app.balancersData.length-1);balancerModal.balancer.strategy='leastLoad'});
   await modal.waitFor({state:'visible'});
   await modal.locator('.ant-modal-footer .ant-btn-primary').click();
   await modal.waitFor({state:'hidden'});
   assert.equal(await page.evaluate(()=>DuiStrategyProbes.read(app.templateSettings,'leastLoad').pingConfig.interval),'30m');
   assert.equal(await page.evaluate(()=>DuiStrategyProbes.read(app.templateSettings,'roundRobin').pingConfig.interval),'30m');
   await page.goto(base+'/dui/xray#invalid-fixture-tab');
   await page.waitForFunction(()=>typeof app!=='undefined'&&app.loadingStates.fetched);
   assert.equal(await page.evaluate(()=>app.activePage),'tpl-basic');
   results.push({width,theme,refresh:true,priority:true,fallback:true,layout:true});
   await context.close();
  }
  assert.deepEqual(errors,[]);
  fs.writeFileSync(out+'/route-health-browser.json',JSON.stringify({passed:true,results,errors},null,2));
  console.log(JSON.stringify({passed:true,cases:results.length,errors}));
 }finally{await browser.close()}
})().catch(e=>{console.error(e.stack);process.exit(1)});
