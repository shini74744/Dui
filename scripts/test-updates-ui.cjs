const fs=require('fs'),path=require('path'),assert=require('assert/strict');
const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const root=path.resolve(__dirname,'..'),read=p=>fs.readFileSync(path.join(root,p),'utf8');
const {DuiUpdateText}=require(root+'/web/assets/js/util/updates.js');
for(const [lang,map]of Object.entries(DuiUpdateText.maps))for(const key of DuiUpdateText.keys)assert.equal(typeof map[key],'string',lang+key);
function html(width,theme,lang){
 const css=['web/assets/ant-design-vue/antd.min.css','web/assets/css/custom.min.css'].map(p=>'<style>'+read(p)+'</style>').join('');
 const libs=['web/assets/vue/vue.min.js','web/assets/ant-design-vue/antd.min.js'].map(p=>'<script>'+read(p)+'</script>').join('');
 const state={supported:true,items:{panel:{current:'v26.9.46',latest:'v26.9.47',available:true},core:{current:'vx-26.6',latest:'vx-26.7',available:true}},checking:false};
 return '<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1">'+css+'<style>body{margin:0;padding:20px;background:'+(theme==='dark'?'#212529':'#f0f2f7')+'}body.dark{color:#ddd}#app{max-width:560px}.dui-product-card{border-radius:24px}</style></head><body class="'+theme+'">'+libs+
 '<script>const LanguageManager={getLanguage:()=>'+JSON.stringify(lang)+'};const themeSwitcher={currentTheme:"'+theme+'"};let serverState='+JSON.stringify(state)+';let requests=0;const axios={get:async()=>({data:{success:true,obj:JSON.parse(JSON.stringify(serverState))}}),post:async(url,data)=>{requests++;if(url.endsWith("/start")){await new Promise(r=>setTimeout(r,250));serverState.job={id:"persisted",kind:data.kind,version:data.version,phase:"connecting",downloaded:0,total:60000000,busy:true};return {data:{success:true,obj:serverState.job}}}return {data:{success:true,obj:serverState}}}};</script><script>'+read('web/assets/js/util/updates.js')+'</script>'+
 '<div id="app"><a-card title="〔DUI-PRO面板〕" class="glass-card dui-product-card"><template #extra><dui-updates ref="u"></dui-updates></template><a-tag color="green">v26.9.46</a-tag><a-tag color="green">Shlii网站</a-tag><a-tag color="purple">〔DUI-PRO 面板〕交流群</a-tag></a-card></div>'+
 '<script>window.app=new Vue({el:"#app"});</script></body></html>';
}
(async()=>{
 const browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH,args:['--no-sandbox']});
 const results=[],errors=[];
 try {
 for(const width of [360,430,620])for(const theme of ['','dark']){
 const page=await browser.newPage({viewport:{width,height:850},isMobile:width<500,hasTouch:width<500});
 page.on('console',m=>{if(m.type()==='error')console.error('CONSOLE',m.text())});page.on('pageerror',e=>{errors.push(e.message);console.error('PAGE',e.message)});await page.setContent(html(width,theme,'zh-CN'));
 await page.locator('.dui-update-pill').first().waitFor();
 const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth);assert.equal(overflow,false);
 const header=await page.locator('.ant-card-head').boundingBox(), pills=await page.locator('.dui-update-pills').boundingBox();
 assert(pills.x>=header.x&&pills.x+pills.width<=header.x+header.width+1,'header overflow');
 if(process.env.DUI_UPDATE_TEST_OUTPUT)await page.screenshot({path:process.env.DUI_UPDATE_TEST_OUTPUT+'/header-'+width+'-'+(theme||'light')+'.png'});
 await page.locator('.dui-update-pill').first().click();
 await page.getByRole('button',{name:'后台下载并更新',exact:true}).click();
 await page.getByText('正在连接下载',{exact:true}).first().waitFor();
 await page.waitForTimeout(350);assert.equal(await page.evaluate(()=>requests),1);
 await page.evaluate(async()=>{serverState.job.phase='downloading';serverState.job.downloaded=24500000;await app.$refs.u.poll()});
 assert.match(await page.locator('.dui-update-byte-count').innerText(),/40%/);
 assert(await page.getByRole('button',{name:'后台下载并更新',exact:true}).isDisabled());
 if(process.env.DUI_UPDATE_TEST_OUTPUT)await page.screenshot({path:process.env.DUI_UPDATE_TEST_OUTPUT+'/progress-'+width+'-'+(theme||'light')+'.png'});
 // Remount simulates returning to the page: status comes from server, not local storage.
 await page.evaluate(async()=>{app.$refs.u.state={items:{panel:{},core:{}}};await app.$refs.u.poll()});
 assert.equal(await page.evaluate(()=>app.$refs.u.job.id),'persisted');
 await page.evaluate(async()=>{serverState.job.phase='rolled_back';serverState.job.busy=false;serverState.job.error='health_check_failed';await app.$refs.u.poll()});
 assert.match(await page.locator('.dui-update-dialog').innerText(),/已恢复原版本/);
 results.push({width,theme:theme||'light',header:true,progress:true,reload:true,rollback:true});await page.close();
 }
 for(const lang of Object.keys(DuiUpdateText.maps)){
 const page=await browser.newPage({viewport:{width:390,height:850}});page.on('console',m=>{if(m.type()==='error')console.error('CONSOLE',m.text())});page.on('pageerror',e=>{errors.push(e.message);console.error('PAGE',e.message)});
 await page.setContent(html(390,'',lang));await page.locator('.dui-update-pill').first().waitFor();await page.locator('.dui-update-pill').first().click();
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,lang+' overflow');await page.close();
 }
 assert.deepEqual(errors,[]);console.log(JSON.stringify({cases:results,locales:13,errors},null,2));
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exit(1)});
