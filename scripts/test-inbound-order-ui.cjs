const fs=require('node:fs'),assert=require('node:assert/strict');
const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const merge=require('../web/assets/js/util/inbound-order.js').merge;
assert.deepEqual(merge([1,4,9,14,20],[4,14],[14,4]),[1,14,9,4,20]);
for(const [before,after] of [[[1,1],[1,1]],[[1,4],[4,99]],[[4,1],[1,4]],[[99],[99]]])assert.equal(merge([1,4,9],before,after),null);
const base=process.env.DUI_FIXTURE_BASE,out=process.env.DUI_PROBE_TEST_OUTPUT||'/tmp';
(async()=>{
 const browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH,args:['--no-sandbox']});
 const cases=[],errors=[],saveResponses=[];let activePage;
 try{
 for(const [width,theme] of JSON.parse(process.env.DUI_ORDER_CASES || '[[390,"dark"],[390,"light"],[1440,"ultra"],[1440,"light"],[320,"ultra"]]')){
  const context=await browser.newContext({viewport:{width,height:950},hasTouch:width<768,isMobile:width<768});
  await context.addCookies([...JSON.parse(process.env.DUI_FIXTURE_COOKIES),{name:'lang',value:'zh-CN',url:base+'/'}]);
  await context.addInitScript(theme=>{localStorage.setItem('dark-mode',String(theme!=='light'));localStorage.setItem('isUltraDarkThemeEnabled',String(theme==='ultra'));localStorage.setItem('isRefreshEnabled','false')},theme);
  const page=await context.newPage();activePage=page;
  page.on('response',async response=>{
   if(response.url().endsWith('/inbounds/reorder')){
    try{saveResponses.push({width,theme,status:response.status(),body:await response.json()});}catch(_e){}
   }
  });
  page.setDefaultTimeout(15000);page.on('pageerror',e=>errors.push(e.message));
  await page.goto(base+'/dui/inbounds');
  await page.waitForFunction(()=>typeof app!=='undefined'&&app.loadingStates.fetched&&app.dbInbounds.length>8);
  await page.evaluate(()=>{app.pageSize=5;});
  const rows=()=>page.locator('.dui-inbound-sort-handle').locator('xpath=ancestor::tr[1]');
  const handles=()=>page.locator('.dui-inbound-sort-handle');
  const ids=()=>page.evaluate(()=>app.dbInbounds.map(r=>r.id));
  const shown=()=>handles().evaluateAll(els=>els.map(el=>Number(el.dataset.inboundId)));
  const idle=()=>page.waitForFunction(()=>!app.inboundOrderSaving&&!app.refreshing&&!app.inboundOrderDragging);
  async function drag(from,to,{cancel=false,outside=false,paused=false}={}){
   await handles().nth(from).scrollIntoViewIfNeeded();
   const s=await handles().nth(from).boundingBox(),t=await rows().nth(to).boundingBox();
   const a={x:s.x+s.width/2,y:s.y+s.height/2},b={x:a.x,y:outside?10:t.y+t.height/2};
   let client;
   if(width<768){
    client=await context.newCDPSession(page);await client.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[a]});
    for(let i=1;i<=8;i++)await client.send('Input.dispatchTouchEvent',{type:'touchMove',touchPoints:[{x:a.x,y:a.y+(b.y-a.y)*i/8}]});
   }else{await page.mouse.move(a.x,a.y);await page.mouse.down();await page.mouse.move(b.x,b.y,{steps:10});}
   if(!outside){
    // Layout can move during an expanded-row animation or edge auto-scroll.
    const target=await rows().nth(to).boundingBox(),y=target.y+target.height/2;
    if(client)await client.send('Input.dispatchTouchEvent',{type:'touchMove',touchPoints:[{x:a.x,y}]});
    else await page.mouse.move(a.x,y);
   }
   if(paused){
    const snapshot=await ids();await page.evaluate(()=>app.getDBInbounds());assert.deepEqual(await ids(),snapshot,'refresh replaced drag');
   }
   if(client){await client.send('Input.dispatchTouchEvent',{type:cancel?'touchCancel':'touchEnd',touchPoints:[]});await client.detach();}
   else{if(cancel)await page.keyboard.press('Escape');await page.mouse.up();}
   await idle();
  }
  // Expanded users must travel with their parent, and never count as sortable rows.
  const initial=await ids(),initialPage=await shown();
  await rows().first().locator('.ant-table-row-expand-icon').click();
  await page.locator('.ant-table-expanded-row:visible').first().waitFor();
  await page.waitForTimeout(350); // Ant table expand animation
  const expandedParent=initialPage[0];
  await drag(0,2,{paused:true});
  const moved=initialPage.slice();moved.splice(2,0,moved.splice(0,1)[0]);
  assert.deepEqual(await ids(),merge(initial,initialPage,moved),'expanded downward drag');
  const source=page.locator('.dui-inbound-sort-handle[data-inbound-id="'+expandedParent+'"]').locator('xpath=ancestor::tr[1]');
  assert(await source.locator('.ant-table-row-expanded').count(),'parent lost expanded state');
  await source.locator('.ant-table-row-expand-icon').click();
  await drag(2,0);assert.deepEqual(await ids(),initial,'upward drag');
  await drag(0,2,{cancel:true});assert.deepEqual(await ids(),initial,'cancel');
  await drag(0,2,{outside:true});assert.deepEqual(await ids(),initial,'outside drop');
  // Mobile scrolling outside the handle remains available.
  assert.equal(await handles().first().evaluate(el=>getComputedStyle(el).touchAction),'none');
  const cellBounds=await rows().first().locator('td').first().boundingBox();
  const handleBounds=await handles().first().boundingBox();
  assert(handleBounds.x>=cellBounds.x&&handleBounds.x+handleBounds.width<=cellBounds.x+cellBounds.width,'handle overflows ID cell');
  const icon=await rows().first().locator('.ant-table-row-expand-icon').boundingBox();
  assert(handleBounds.x+handleBounds.width<=icon.x+1,'handle overlaps expand button');
  if(width<768)assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'mobile page horizontal overflow');
  // Page two: save only its visible slots, keep page one unchanged.
  await page.locator('.dui-inbound-table .ant-pagination-next').click();
  const pageTwo=await shown();assert.equal(pageTwo.length,5);
  await drag(0,4);
  const second=pageTwo.slice();second.push(second.shift());
  const afterPage=merge(initial,pageTwo,second);
  assert.deepEqual(await ids(),afterPage,'page two offset');
  // Reopen from the server, then reorder a filtered subset without losing hidden inbounds.
  await page.reload();await page.waitForFunction(()=>typeof app!=='undefined'&&app.loadingStates.fetched);await idle();
  assert.deepEqual(await ids(),afterPage,'server persisted order');
  await page.evaluate(()=>{app.pageSize=0;app.searchKey='Order group A';app.searchInbounds(app.searchKey);});
  await page.waitForFunction(()=>app.searchedInbounds.length>=3);
  const subset=await shown(),all=await ids();await drag(0,2);
  const reordered=subset.slice();reordered.splice(2,0,reordered.splice(0,1)[0]);
  const afterFilter=merge(all,subset,reordered);assert.deepEqual(await ids(),afterFilter,'filtered hidden slots moved');
  // Keyboard and stale-list conflict use the same persistence and rollback path.
  await handles().first().focus();const keyboardPage=await shown();
  await page.keyboard.press('ArrowDown');await idle();
  assert.equal(await page.evaluate(()=>Number(document.activeElement.dataset.inboundId)),keyboardPage[0],'keyboard focus lost after save');
  const keyboardOrder=keyboardPage.slice();[keyboardOrder[0],keyboardOrder[1]]=[keyboardOrder[1],keyboardOrder[0]];
  assert.deepEqual(await ids(),merge(afterFilter,keyboardPage,keyboardOrder));
  const beforeConflict=await ids();
  const external=beforeConflict.slice().reverse();
  const saved=await context.request.post(base+'/dui/api/inbounds/reorder',{form:{baseIds:JSON.stringify(beforeConflict),ids:JSON.stringify(external)}});
  assert((await saved.json()).success);
  await drag(0,2);assert.deepEqual(await ids(),external,'stale UI overwrote external order');
  // A network failure keeps the old server order, re-enables handles, and reports an error.
  await page.route('**/dui/api/inbounds/reorder',route=>route.fulfill({status:503,json:{message:'test save failure'}}));
  await drag(0,2);assert.deepEqual(await ids(),external,'failed save changed order');
  assert.equal(await handles().first().isDisabled(),false);
  await page.unroute('**/dui/api/inbounds/reorder');
  await page.evaluate(()=>{app.searchKey='';app.searchInbounds('');app.pageSize=5;});
  await page.screenshot({path:out+'/inbound-order-'+width+'-'+theme+'.png',fullPage:true});
  cases.push({width,theme,mouseOrTouch:true,expandedRows:true,cancel:true,pageTwo:true,filtered:true,keyboard:true,reload:true,conflict:true,failure:true});
  console.log('Browser case passed: '+width+' '+theme);
  await context.close();
 }
 // Long touch drag scrolls the page; cancelling leaves no reorder or stuck state.
 const context=await browser.newContext({viewport:{width:390,height:700},hasTouch:true,isMobile:true});
 await context.addCookies(JSON.parse(process.env.DUI_FIXTURE_COOKIES));
 const page=await context.newPage();activePage=page;page.on('pageerror',e=>errors.push(e.message));
 await page.goto(base+'/dui/inbounds');await page.waitForFunction(()=>typeof app!=='undefined'&&app.loadingStates.fetched);
 await page.evaluate(()=>{app.pageSize=0;});const allBefore=await page.evaluate(()=>app.dbInbounds.map(r=>r.id));
 const handle=page.locator('.dui-inbound-sort-handle').first();await handle.scrollIntoViewIfNeeded();
 const box=await handle.boundingBox(),client=await context.newCDPSession(page);
 const scrollPosition=()=>{let sum=0;for(let el=document.querySelector('.dui-inbound-sort-handle');el;el=el.parentElement)sum+=el.scrollTop;return sum};
 const startScroll=await page.evaluate(scrollPosition);
 await client.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[{x:box.x+15,y:box.y+15}]});
 await client.send('Input.dispatchTouchEvent',{type:'touchMove',touchPoints:[{x:box.x+15,y:680}]});
 await page.waitForFunction(before=>{let sum=0;for(let el=document.querySelector('.dui-inbound-sort-handle');el;el=el.parentElement)sum+=el.scrollTop;return sum>before+150},startScroll);
 await client.send('Input.dispatchTouchEvent',{type:'touchCancel',touchPoints:[]});
 assert.deepEqual(await page.evaluate(()=>app.dbInbounds.map(r=>r.id)),allBefore);
 assert.equal(await page.locator('.dui-inbound-drag-source').count(),0);
 await context.close();
 assert.deepEqual(errors,[]);
 const report={passed:true,cases,autoscroll:true,errors};fs.writeFileSync(out+'/inbound-order-browser.json',JSON.stringify(report,null,2));console.log(JSON.stringify(report));
 }catch(error){
  console.error('Recent ordering responses:',JSON.stringify(saveResponses.slice(-12)));
  if(activePage&&!activePage.isClosed())await activePage.screenshot({path:out+'/inbound-order-failure.png',fullPage:true}).catch(()=>{});
  throw error;
 }finally{await browser.close();}
})().catch(e=>{console.error(e.stack);process.exitCode=1});
