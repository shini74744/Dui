const fs=require('node:fs'),assert=require('node:assert/strict');
const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const base=process.env.DUI_FIXTURE_BASE,out=process.env.DUI_PROBE_TEST_OUTPUT||'/tmp';
const client={id:'11111111-1111-4111-8111-111111111111',email:'alpha-user',comment:'User note',enable:true,totalGB:0,expiryTime:0,subId:'keep-sub'};
const fixture=()=>({id:4,port:43009,remark:'Primary inbound',protocol:'vless',enable:true,up:123,down:456,total:0,allTime:800,expiryTime:0,trafficReset:'never',deviceLimit:0,
 settings:JSON.stringify({clients:[client],decryption:'none',fallbacks:[],unknown:{keep:true}}),streamSettings:'{"network":"tcp","security":"none"}',sniffing:'{"enabled":false}',tag:'inbound-43009',
 clientStats:[{email:client.email,enable:true,up:123,down:456,total:0,expiryTime:0}]});
(async()=>{
 const browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH,args:['--no-sandbox']});
 const errors=[],cases=[];
 try{
 for(const [width,theme,lang] of [[320,'dark','zh-CN'],[390,'light','zh-CN'],[390,'dark','zh-CN'],[768,'light','zh-CN'],[1024,'dark','zh-CN'],[1440,'light','zh-CN'],[1440,'dark','zh-CN'],[390,'dark','en-US']]){
  const context=await browser.newContext({viewport:{width,height:1000}});
  await context.addCookies([...JSON.parse(process.env.DUI_FIXTURE_COOKIES),{name:'lang',value:lang,url:base+'/'}]);
  await context.addInitScript(theme=>{localStorage.setItem('dark-mode',String(theme==='dark'));localStorage.setItem('isRefreshEnabled','false')},theme);
  const page=await context.newPage();page.setDefaultTimeout(12000);
  page.on('pageerror',e=>errors.push(e.message));
  page.on('console',m=>{if(m.type()==='error'&&m.text().includes('[Vue warn]'))errors.push(m.text())});
  let row=fixture(),saved=[],referenceError=false;
  const refs=[{inboundId:4,email:client.email,rules:[
   {index:2,outboundTag:'Primary-outbound',fallbackTag:'Backup-outbound',regexp:false},
   {index:17,ruleTag:'User group',balancerTag:'Group-'+ 'LongTag'.repeat(14),regexp:true}]}];
  await page.route('**/dui/api/inbounds/**',async route=>{
   const req=route.request(),path=new URL(req.url()).pathname;
   if(path.endsWith('/list'))return route.fulfill({json:{success:true,obj:[row]}});
   if(path.endsWith('/clientRouteReferences'))return route.fulfill({json:referenceError?{success:false,msg:'fixture unavailable'}:{success:true,obj:refs}});
   if(path.endsWith('/onlines'))return route.fulfill({json:{success:true,obj:[]}});
   if(path.endsWith('/lastOnline'))return route.fulfill({json:{success:true,obj:{}}});
   if(path.endsWith('/activity'))return route.fulfill({json:{success:true,obj:{inbounds:{},clients:{}}}});
   if(path.endsWith('/clientBot/get'))return route.fulfill({json:{success:true,obj:{}}});
   const data=Object.fromEntries(new URLSearchParams(req.postData()||''));
   if(path.endsWith('/updateRemark/4')){saved.push({path,data});row.remark=data.remark;return route.fulfill({json:{success:true,msg:''}})}
   if(path.endsWith('/update/4')){saved.push({path,data});row.port=Number(data.port);return route.fulfill({json:{success:true,msg:''}})}
   throw Error('Unexpected request '+path);
  });
  await page.goto(base+'/dui/inbounds');
  await page.waitForFunction(()=>typeof app!=='undefined'&&app.dbInbounds.length===1&&app.clientRouteRefsLoaded);
  await page.locator('tr.ant-table-row[data-row-key="4"] .ant-table-row-collapsed').first().click();
  const badge=page.locator('.ant-table-expanded-row:visible .client-route-button');
  assert.equal(await badge.count(),1);assert.equal((await badge.innerText()).trim(),'');assert.match(await badge.getAttribute('aria-label'),/2/);
  const nameCell=page.locator('.ant-table-expanded-row:visible td.client-name-cell').first();
  const neighbour=nameCell.locator('xpath=following-sibling::td[1]');
  const before=await neighbour.boundingBox();
  for(const name of ['LongUnbrokenUserName'.repeat(18),'测试超长用户名称'.repeat(30)]){
   await nameCell.locator('.client-email').evaluate((el,name)=>{el.textContent=name},name);
   const after=await neighbour.boundingBox();
   assert(Math.abs(before.x-after.x)<1 && Math.abs(before.width-after.width)<1,'long name moved or squeezed adjacent column');
   const bounds=await nameCell.evaluate(el=>{
    const cell=el.getBoundingClientRect(),name=el.querySelector('.client-email'),icon=el.querySelector('.client-route-button');
    return {contained:[name,icon].every(e=>{const r=e.getBoundingClientRect();return r.left>=cell.left&&r.right<=cell.right+1}),
     iconAtStart:icon.getBoundingClientRect().top<name.getBoundingClientRect().top+8,
     wraps:name.getBoundingClientRect().height>parseFloat(getComputedStyle(name).lineHeight)*1.5,overflow:el.scrollWidth>el.clientWidth+1};
   });
   assert(bounds.contained && bounds.iconAtStart && bounds.wraps && !bounds.overflow,'name/icon did not wrap within its cell: '+JSON.stringify(bounds));
  }
  await page.screenshot({path:out+'/metadata-long-name-'+width+'-'+theme+'-'+lang+'.png'});
  await nameCell.locator('.client-email').evaluate((el,name)=>{el.textContent=name},client.email);
  await badge.click();
  const popup=page.locator('.ant-popover:visible .client-route-details');
  await popup.waitFor();assert.match(await popup.innerText(),/#2/);assert.match(await popup.innerText(),/#17/);
  const box=await popup.boundingBox();assert(box.x>=0&&box.x+box.width<=width,'route popup overflows viewport');
  await page.screenshot({path:out+'/metadata-routes-'+width+'-'+theme+'-'+lang+'.png'});
  await badge.click();
  await popup.waitFor({state:'hidden'});
  await page.evaluate(()=>app.openEditClient(4,app.getInboundClients(app.dbInbounds[0])[0]));
  const modal=page.locator('.ant-modal:visible');
  await modal.waitFor();
  const toggle=modal.locator('.client-route-toggle');
  const expander=modal.locator('.client-route-expander');
  const settleExpansion=()=>expander.evaluate(async el=>{
   await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));
   await Promise.all(el.getAnimations().map(a=>a.finished.catch(()=>{})));
  });
  const assertCollapsed=async()=>{
   await settleExpansion();
   assert.equal(await expander.getAttribute('aria-hidden'),'true');
   assert((await expander.boundingBox()).height<1,'collapsed details occupy space');
  };
  assert.equal(await toggle.getAttribute('aria-expanded'),'false');
  await assertCollapsed();
  await page.screenshot({path:out+'/metadata-collapsed-'+width+'-'+theme+'-'+lang+'.png'});
  const heights=await toggle.evaluate(el=>new Promise(resolve=>{
   const body=el.closest('.client-route-editor').querySelector('.client-route-expander'),samples=[body.getBoundingClientRect().height];
   el.click();const start=performance.now();
   function frame(){samples.push(body.getBoundingClientRect().height);if(performance.now()-start<360)requestAnimationFrame(frame);else resolve(samples)}
   requestAnimationFrame(frame);
  }));
  assert(heights.at(-1)>20 && heights.some(h=>h>1 && h<heights.at(-1)-1),'details did not expand smoothly');
  assert.equal(await toggle.getAttribute('aria-expanded'),'true');
  assert.equal(await modal.locator('.client-route-editor .client-route-item').count(),2);
  await toggle.click();
  await assertCollapsed();
  await modal.locator('.ant-form-item input').first().fill('renamed-user');
  await modal.locator('.client-route-rename-warning').waitFor();
  assert.equal(await toggle.getAttribute('aria-expanded'),'false','rename must not force details open');
  await toggle.focus();await page.keyboard.press('Enter');
  await settleExpansion();
  assert.equal(await modal.locator('.client-route-editor .client-route-item').count(),2);
  const mbox=await modal.boundingBox();assert(mbox.x>=0&&mbox.x+mbox.width<=width,'edit modal overflows viewport');
  assert.equal(await modal.evaluate(el=>el.scrollWidth<=el.clientWidth+2),true,'edit content has horizontal overflow');
  await page.screenshot({path:out+'/metadata-edit-'+width+'-'+theme+'-'+lang+'.png'});
  await page.evaluate(()=>clientModal.close());
  await modal.waitFor({state:'hidden'});
  await page.evaluate(()=>app.openEditClient(4,app.getInboundClients(app.dbInbounds[0])[0]));
  await modal.waitFor();
  assert.equal(await modal.locator('.client-route-toggle').getAttribute('aria-expanded'),'false','reopening must start collapsed');
  await assertCollapsed();
  await page.evaluate(()=>clientModal.close());
  await modal.waitFor({state:'hidden'});
  await page.evaluate(()=>app.openEditInbound(4));
  await page.locator('.ant-modal:visible').waitFor();
  // Change through the real form model and click the actual save button.
  await page.evaluate(()=>{inModal.dbInbound.remark='Renamed safely'});
  await page.locator('.ant-modal:visible .ant-modal-footer .ant-btn-primary').click();
  await page.waitForFunction(()=>!inModal.visible&&app.dbInbounds[0].remark==='Renamed safely');
  assert.equal(saved.length,1);assert(saved[0].path.endsWith('/updateRemark/4'));assert.deepEqual(Object.keys(saved[0].data),['remark']);
  assert.equal(JSON.parse(row.settings).unknown.keep,true);
  await page.locator('.ant-modal:visible').waitFor({state:'hidden'});
  await page.evaluate(()=>app.openEditInbound(4));
  await page.evaluate(()=>{inModal.inbound.port=43010});
  await page.locator('.ant-modal:visible .ant-modal-footer .ant-btn-primary').click();
  await page.waitForFunction(()=>!inModal.visible&&app.dbInbounds[0].port===43010);
  assert(saved[1].path.endsWith('/update/4'),'connection change did not use normal save');
  await page.locator('.ant-modal:visible').waitFor({state:'hidden'});
  referenceError=true;await page.evaluate(()=>app.loadClientRouteReferences());
  await page.evaluate(()=>app.openEditClient(4,app.getInboundClients(app.dbInbounds[0])[0]));
  await page.locator('.ant-modal:visible .client-route-editor .ant-alert-warning').waitFor();
  assert.equal(await page.locator('.ant-modal:visible .client-route-item').count(),0,'stale references shown as current');
  cases.push({width,theme,lang,passed:true});await context.close();
 }
 assert.deepEqual(errors,[]);
 fs.writeFileSync(out+'/metadata-browser.json',JSON.stringify({passed:true,cases,errors},null,2));
 console.log(JSON.stringify({browser:true,passed:true,cases:cases.length}));
 }finally{await browser.close()}
})().catch(e=>{console.error(e.stack);process.exitCode=1});
