const fs = require('fs'), path = require('path'), assert = require('assert/strict');
const {chromium} = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const root = path.resolve(__dirname, '..'), read = p => fs.readFileSync(path.join(root,p), 'utf8');
const order = require(root+'/web/assets/js/util/outbound-order.js');
const xray = read('web/html/xray.html');
const between = (a,b) => xray.slice(xray.indexOf(a),xray.indexOf(b,xray.indexOf(a)));
const base = {
 outbounds: [
  {tag:'A',protocol:'freedom',settings:{domainStrategy:'AsIs'},future:{keep:1}},
  {tag:'__dui_blacklist_internal',protocol:'blackhole'},
  {tag:'B-new',protocol:'freedom'}, {tag:'C',protocol:'blackhole'},
  {tag:'D',protocol:'freedom'}, {tag:'E',protocol:'freedom'},
  {tag:'__dui_blacklist_last',protocol:'blackhole'}],
 routing:{rules:[{outboundTag:'B-old',domain:['example.test']},{outboundTag:'C'}],balancers:[{tag:'balancer',selector:['B-new','D']}]},
 strategyObservatory:{random:{subjectSelector:['B-new','D']}}
};
for (const args of [[null,1],[0,NaN],[-1,1],[1,9],[0,0]]) assert.equal(order.move(base.outbounds,...args),base.outbounds);
const ordered=order.move(base.outbounds,0,4);
assert.deepEqual(ordered.map(x=>x.tag),['B-new','__dui_blacklist_internal','C','D','E','A','__dui_blacklist_last']);
assert.equal(ordered[5],base.outbounds[0]);assert.equal(ordered[1],base.outbounds[1]);
assert.equal(base.outbounds[0].tag,'A');
const translate = raw => raw.replace(/\{\{\s*i18n\s+"([^"]+)"\s*\}\}/g,(_,key)=>({
 'pages.xray.outbound.dragSort':'拖动排序', 'pages.xray.outbound.orderHint':'拖动调整顺序，保存后生效。首个出站为默认出口。',
 'pages.xray.rules.first':'置顶','pages.xray.rules.last':'置底','pages.xray.rules.up':'上移','pages.xray.rules.down':'下移',
 'pages.xray.outbound.addOutbound':'添加出站','pages.xray.outbound.tag':'标签','pages.xray.outbound.address':'地址','pages.inbounds.traffic':'流量','pages.inbounds.resetTraffic':'重置流量','protocol':'协议',
 'edit':'编辑','delete':'删除'
}[key]||key));
function html(width,theme) {
 const css=['web/assets/ant-design-vue/antd.min.css','web/assets/css/custom.min.css'].map(p=>'<style>'+read(p)+'</style>').join('');
 const libs=['web/assets/vue/vue.min.js','web/assets/ant-design-vue/antd.min.js','web/assets/js/util/outbound-references.js','web/assets/js/util/outbound-order.js','web/assets/js/util/outbound-speedtest-i18n.js'].map(p=>'<script>'+read(p)+'</script>').join('');
 const speedComponent=read('web/html/settings/xray/outbound_speedtest.html').replace(/\{\{define[^}]*\}\}|\{\{end\}\}/g,'');
 const speedMocks='<script>const LanguageManager={getLanguage:()=>"zh-CN"};const HttpUtil={async get(url){return {success:true,obj:url.endsWith("catalog")?[{tag:"A",protocol:"freedom"},{tag:"B-new",protocol:"freedom"}]:{state:"idle",rows:[]}}}};</script>';
 const tpl=translate(read('web/html/settings/xray/outbounds.html').replace(/\{\{define[^}]*\}\}|\{\{end\}\}/g,''));
 const columns=translate(between('  const outboundColumns = [','  const reverseColumns = ['));
 const methods=translate(between('      editOutbound(index) {','      addReverse() {'));
 const computed=between('      templateSettings: {','      inboundSettings: {')+between('      outboundData: {','      reverseData: {');
 return '<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1">'+css+'<style>body.dark{background:#101114;color:#ddd}main{padding:12px}.ant-table-wrapper{max-width:100%}</style></head><body class="'+theme+'">'+libs+speedMocks+speedComponent+
 '<script>const duiIsHiddenBlacklistTag=t=>String(t||"").startsWith("__dui_blacklist_");const themeSwitcher={currentTheme:"'+theme+'",isDarkTheme:'+JSON.stringify(theme==='dark')+'};const Protocols={};const outModal={show:o=>window.edited=o,close(){}};'+columns+'</script>'+
 '<main id="app"><button id="save" @click="saved=xraySetting">保存</button><button id="reload" @click="xraySetting=saved">重新打开</button>'+tpl+'</main>'+
 '<script>window.app=new Vue({el:"#app",delimiters:["[[","]]"],data:{xraySetting:'+JSON.stringify(JSON.stringify(base))+',saved:'+JSON.stringify(JSON.stringify(base))+',pendingOutboundRenames:[{tag:"B-new",previousTags:["B-old"]}],routeSourceMap:{keep:[1]},saveBtnDisable:true,speedTestLabel:"测速",refreshing:false,isMobile:'+(width<768)+',outboundColumns},computed:{'+computed+'},methods:{'+methods+
 'findOutboundAddress(){return []},findOutboundTraffic(o){return o.tag+" traffic"},addOutbound(){},showWarp(){},refreshOutboundTraffic(){},resetOutboundTraffic(){}}});</script></body></html>';
}
(async()=>{
 const browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH,args:['--no-sandbox']});
 const errors=[],results=[];
 try{
 for(const width of [390,1100])for(const theme of ['','dark']){
  const page=await browser.newPage({viewport:{width,height:850},hasTouch:width<768,isMobile:width<768});
  page.on('pageerror',e=>errors.push(e.message));await page.setContent(html(width,theme));
  const rows=page.locator('.dui-outbound-table tr.ant-table-row'), handles=page.locator('.dui-outbound-sort-handle');
  await handles.first().waitFor();
  await page.getByRole('button',{name:'下载测速',exact:true}).click();
  assert(await page.getByRole('button',{name:'开始测速',exact:true}).isDisabled());
  assert(!(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1)), 'speed panel overflow');
  await page.getByRole('button',{name:'下载测速',exact:true}).click();
  const tags=()=>page.evaluate(()=>app.templateSettings.outbounds.filter(o=>!o.tag.startsWith('__dui_blacklist_')).map(o=>o.tag));
  async function drag(from,to,cancel=false,outside=false){
   const source=await handles.nth(from).boundingBox(),target=await rows.nth(to).boundingBox();
   const a={x:source.x+source.width/2,y:source.y+source.height/2},b={x:a.x,y:outside?10:target.y+target.height/2};
   if(width<768){
    const client=await page.context().newCDPSession(page);
    await client.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[a]});
    for(let i=1;i<=8;i++)await client.send('Input.dispatchTouchEvent',{type:'touchMove',touchPoints:[{x:a.x+(b.x-a.x)*i/8,y:a.y+(b.y-a.y)*i/8}]});
    await client.send('Input.dispatchTouchEvent',{type:cancel?'touchCancel':'touchEnd',touchPoints:[]});await client.detach();
   }else{
    await page.mouse.move(a.x,a.y);await page.mouse.down();await page.mouse.move(b.x,b.y,{steps:12});
    if(cancel)await page.keyboard.press('Escape');await page.mouse.up();
   }
   await page.waitForTimeout(50);
  }
  await drag(0,4);assert.deepEqual(await tags(),['B-new','C','D','E','A'],'down '+width);
  await drag(4,0);assert.deepEqual(await tags(),['A','B-new','C','D','E'],'up '+width);
  await drag(0,3,true);assert.deepEqual(await tags(),['A','B-new','C','D','E'],'cancel');
  await drag(0,3,false,true);assert.deepEqual(await tags(),['A','B-new','C','D','E'],'outside');
  await handles.nth(1).focus();await page.keyboard.press('ArrowDown');
  assert.deepEqual(await tags(),['A','C','B-new','D','E'],'keyboard');
  await page.locator('#save').click();await page.evaluate(()=>app.replaceOutbound(0,4));
  await page.locator('#reload').click();assert.deepEqual(await tags(),['A','C','B-new','D','E'],'reload');
  await page.evaluate(()=>app.editOutbound(2));
  assert.equal(await page.evaluate(()=>edited.outbound.tag),'B-new');
  assert.deepEqual(await page.evaluate(()=>edited.previousTags),['B-old']);
  const current=await page.evaluate(()=>app.templateSettings);
  assert.deepEqual(current.routing,base.routing);assert.deepEqual(current.strategyObservatory,base.strategyObservatory);
  assert.deepEqual(current.outbounds[1],base.outbounds[1]);assert.deepEqual(current.outbounds[6],base.outbounds[6]);
  assert.deepEqual(current.outbounds[0],base.outbounds[0]);assert.deepEqual(await page.evaluate(()=>app.routeSourceMap),{keep:[1]});
  await rows.nth(2).locator('.ant-dropdown-trigger').click();await page.getByRole('menuitem').filter({hasText:'置底'}).click();
  assert.deepEqual(await tags(),['A','C','D','E','B-new'],'menu');
  if(process.env.DUI_ORDER_TEST_OUTPUT)await page.screenshot({path:process.env.DUI_ORDER_TEST_OUTPUT+'/outbound-order-'+width+'-'+(theme||'light')+'.png',fullPage:true});
  results.push({width,theme:theme||'light',mouseOrTouch:true,cancel:true,keyboard:true,menu:true,reload:true,referencesPreserved:true});
  await page.close();
 }
 // Long list scrolls while holding the handle; cancellation and data changes leave no stale gesture.
 const page=await browser.newPage({viewport:{width:1100,height:650}});
 page.on('pageerror',e=>errors.push(e.message));await page.setContent(html(1100,'dark'));
 await page.evaluate(()=>{app.$el.style.cssText='height:650px;overflow:auto';const c=app.templateSettings;for(let i=0;i<25;i++)c.outbounds.push({tag:'extra-'+i,protocol:'freedom'});app.templateSettings=c;});
 const h=await page.locator('.dui-outbound-sort-handle').first().boundingBox();
 await page.mouse.move(h.x+10,h.y+10);await page.mouse.down();await page.mouse.move(h.x+10,630,{steps:12});
 await page.waitForFunction(()=>app.$el.scrollTop>300);await page.mouse.up();
 assert.equal(await page.locator('.dui-outbound-dragging').count(),0);
 await page.evaluate(()=>app.$el.scrollTop=0);
 const first=page.locator('.dui-outbound-sort-handle').first();await first.scrollIntoViewIfNeeded();
 const f=await first.boundingBox();await page.mouse.move(f.x+10,f.y+10);await page.mouse.down();
 const before=await page.evaluate(()=>app.xraySetting);
 await page.evaluate(()=>app.xraySetting=JSON.stringify({...app.templateSettings,remarks:'external draft change'}));
 await page.mouse.move(f.x+10,f.y+200);await page.mouse.up();
 assert.deepEqual(await page.evaluate(()=>app.templateSettings.outbounds),JSON.parse(before).outbounds);
 await page.close();
 assert.deepEqual(errors,[]);console.log(JSON.stringify({passed:true,cases:results,autoscroll:true,staleGestureCancelled:true,errors}));
 }finally{await browser.close()}
})().catch(e=>{console.error(e.stack);process.exit(1)});
