const fs=require('node:fs'),assert=require('node:assert/strict');
const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const base=process.env.DUI_FIXTURE_BASE,out=process.env.DUI_PROBE_TEST_OUTPUT||'/tmp';
const clients=[
 {id:'11111111-1111-4111-8111-111111111111',email:'alpha-user',comment:'Shanghai client',enable:true,totalGB:0,expiryTime:0,subId:'alpha-sub'},
 {id:'22222222-2222-4222-8222-222222222222',email:'beta-user',comment:'Tokyo client',enable:true,totalGB:0,expiryTime:0,subId:'beta-sub'}
];
const inbound=(id,port,remark,users)=>({id,userId:1,port,remark,protocol:'vless',enable:true,up:0,down:0,total:0,allTime:0,expiryTime:0,
 settings:JSON.stringify({clients:users,decryption:'none',fallbacks:[],customExtension:{keep:true}}),
 streamSettings:JSON.stringify({network:'tcp',security:'none',tcpSettings:{header:{type:'none'}}}),
 sniffing:'{"enabled":false}',tag:'inbound-'+port,clientStats:users.map(c=>({email:c.email,enable:true,up:0,down:0,total:0,expiryTime:0}))});
const fixtures=[inbound(4,43009,'Japan dedicated',clients),inbound(9,19000,'Other service',[{...clients[0],id:'33333333-3333-4333-8333-333333333333',email:'other-user'}])];
(async()=>{
 const browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH,args:['--no-sandbox']});
 const errors=[],cases=[];let mutations=0;
 try{
 for(const width of [390,1440])for(const theme of ['light','dark']){
  const context=await browser.newContext({viewport:{width,height:1000}});
  await context.addCookies([...JSON.parse(process.env.DUI_FIXTURE_COOKIES),{name:'lang',value:'zh-CN',url:base+'/'}]);
  await context.addInitScript(theme=>{localStorage.setItem('dark-mode',String(theme==='dark'));localStorage.setItem('isRefreshEnabled','false')},theme);
  const page=await context.newPage();page.setDefaultTimeout(10000);page.on('pageerror',e=>errors.push(e.message));
  await page.route('**/dui/api/inbounds/**',route=>{
   const pathname=new URL(route.request().url()).pathname;
   if(pathname.endsWith('/list'))return route.fulfill({json:{success:true,obj:fixtures}});
   if(pathname.endsWith('/onlines'))return route.fulfill({json:{success:true,obj:[]}});
   if(pathname.endsWith('/lastOnline'))return route.fulfill({json:{success:true,obj:{}}});
   if(pathname.endsWith('/activity') && route.request().method()==='GET')return route.fulfill({json:{success:true,obj:{inbounds:{},clients:{}}}});
   mutations++;console.error('Unexpected fixture request',route.request().method(),pathname);return route.abort();
  });
  await page.goto(base+'/dui/inbounds');
  await page.waitForFunction(()=>typeof app!=='undefined'&&app.loadingStates.fetched&&app.dbInbounds.length===2);
  const original=await page.evaluate(()=>JSON.stringify(app.dbInbounds));
  const input=page.locator('#app input[autofocus]').first();
  async function search(value,ids){
   await input.fill(value);
   await page.waitForFunction(({value,ids})=>app.searchKey===value&&JSON.stringify(app.searchedInbounds.map(i=>i.id))===JSON.stringify(ids),{value,ids});
  }
  async function users(names){
   const row=page.locator('tr.ant-table-row[data-row-key="4"]').first();
   const collapsed=row.locator('.ant-table-row-collapsed');
   if(await collapsed.count())await collapsed.click();
   const shown=page.locator('.ant-table-expanded-row:visible .client-email');
   await page.waitForFunction(names=>JSON.stringify([...document.querySelectorAll('.ant-table-expanded-row .client-email')].filter(e=>e.getClientRects().length).map(e=>e.textContent.trim()))===JSON.stringify(names),names);
   assert.deepEqual(await shown.allTextContents(),names);
  }
  await search('4300',[4]);await users(['alpha-user','beta-user']);
  await page.waitForTimeout(300);await page.screenshot({path:out+'/inbound-search-'+width+'-'+theme+'.png'});
  await search('ALPHA',[4]);await users(['alpha-user']);
  await search('Tokyo client',[4]);await users(['beta-user']);
  await search('Japan dedicated',[4]);await users(['alpha-user','beta-user']);
  await page.evaluate(()=>app.getDBInbounds());await users(['alpha-user','beta-user']);
  await search('absent-user',[]);
  await search('',[4,9]);
  assert.equal(await page.evaluate(()=>JSON.stringify(app.dbInbounds)),original,'search modified original inbound models');
  await search('4300',[4]);await users(['alpha-user','beta-user']);
  cases.push({width,theme,passed:true});await context.close();
 }
 assert.deepEqual(errors,[]);assert.equal(mutations,0);
 fs.writeFileSync(out+'/browser.json',JSON.stringify({passed:true,cases,errors,mutations},null,2));
 console.log(JSON.stringify({passed:true,cases:cases.length,errors,mutations}));
 }finally{await browser.close()}
})().catch(e=>{console.error(e.stack);process.exitCode=1});
