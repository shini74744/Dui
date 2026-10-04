const fs = require('fs'), vm = require('vm'), assert = require('assert/strict');
const root = require('path').resolve(__dirname, '..');
const read = name => fs.readFileSync(root + '/' + name, 'utf8');
const plain = value => JSON.parse(JSON.stringify(value));
const context = vm.createContext({ console });
vm.runInContext(read('web/assets/js/util/outbound-references.js') + '\nthis.refs = DuiOutboundReferences;', context);
const { refs } = context;
const oldTag = 'HK-HKT-B-DDG', newTag = 'HK-HKT-NEW';
const node = tag => ({tag, protocol:'freedom', settings:{domainStrategy:'AsIs'}});
const fixture = () => ({
    outbounds:[node('direct'), node(oldTag), node('other')],
    routing:{domainStrategy:'AsIs',rules:Array.from({length:12},(_,i)=>({type:'field',domain:['domain:example'+i+'.test'],outboundTag:i===11?oldTag:'direct'}))},
    dns:{hosts:{'HK-HKT-B-DDG':'192.0.2.1'}}, log:{loglevel:'warning'}
});
let cases = 0;
function test(name, fn) { fn(); cases++; }

test('rename updates route 12, preserves order, conditions and original input',()=>{
    const before = fixture(), snapshot = plain(before), edited = {...node(newTag),settings:{domainStrategy:'UseIPv4'}};
    const next = plain(refs.prepare(before,1,edited,'sync'));
    assert.deepEqual(before,snapshot);
    assert.equal(next.outbounds.length,3);
    assert.equal(next.outbounds[1].tag,newTag);
    assert.equal(next.outbounds[1].settings.domainStrategy,'UseIPv4');
    const expected = plain(before);expected.outbounds[1]=edited;expected.routing.rules[11].outboundTag=newTag;
    assert.deepEqual(next,expected);
    assert.deepEqual(plain(refs.inspect(before,oldTag)).routes,[12]);
});
test('ordinary confirm keeps the original route unchanged',()=>{
    const next=plain(refs.prepare(fixture(),1,node(newTag),'replace'));
    assert.equal(next.outbounds[1].tag,newTag);assert.equal(next.routing.rules[11].outboundTag,oldTag);assert.equal(next.outbounds.length,3);
});
test('parameter-only edits keep the tag and references',()=>{
    for(const mode of ['sync','replace']) {
        const edited={...node(oldTag),settings:{domainStrategy:'UseIPv6'}};
        const next=plain(refs.prepare(fixture(),1,edited,mode));
        assert.deepEqual(next.outbounds[1],edited);assert.equal(next.routing.rules[11].outboundTag,oldTag);
    }
});
test('all direct references update, unrelated tag fields remain intact',()=>{
    const before=fixture();
    before.routing.rules.push({type:'field',network:'tcp',outboundTag:oldTag});
    before.routing.balancers=[{tag:oldTag,selector:[oldTag,'other'],fallbackTag:oldTag}];
    before.observatory={subjectSelector:[oldTag]};before.burstObservatory={subjectSelector:[oldTag]};
    before.outbounds[2].proxySettings={tag:oldTag};
    before.outbounds[2].streamSettings={sockopt:{dialerProxy:oldTag},xhttpSettings:{extra:{downloadSettings:{sockopt:{dialerProxy:oldTag}}}}};
    before.inbounds=[{tag:oldTag,streamSettings:{sockopt:{dialerProxy:oldTag}}}];
    const next=plain(refs.prepare(before,1,node(newTag),'sync'));
    assert.equal(next.routing.balancers[0].tag,oldTag);assert.equal(next.routing.balancers[0].fallbackTag,newTag);
    assert.deepEqual(next.routing.balancers[0].selector,[newTag,'other']);
    assert.deepEqual(next.observatory.subjectSelector,[newTag]);assert.deepEqual(next.burstObservatory.subjectSelector,[newTag]);
    assert.equal(next.inbounds[0].tag,oldTag);assert.equal(next.inbounds[0].streamSettings.sockopt.dialerProxy,newTag);
    assert.equal(next.outbounds[2].proxySettings.tag,newTag);
    assert.equal(next.outbounds[2].streamSettings.xhttpSettings.extra.downloadSettings.sockopt.dialerProxy,newTag);
    assert.equal(refs.inspect(next,oldTag).total,0);
});
test('prefix group retains other nodes when a matching member is renamed',()=>{
    const before=fixture();before.outbounds.push(node(oldTag+'-2'));
    before.routing.balancers=[{tag:'group',selector:[oldTag]}];
    const next=plain(refs.prepare(before,1,node(newTag),'sync'));
    assert.deepEqual(next.routing.balancers[0].selector,[oldTag,newTag]);
});
test('shorter prefixes are preserved, matching membership follows renamed node',()=>{
    const before=fixture();before.routing.balancers=[{tag:'group',selector:['HK-']}];
    let next=plain(refs.prepare(before,1,node(newTag),'sync'));
    assert.deepEqual(next.routing.balancers[0].selector,['HK-']);
    next=plain(refs.prepare(before,1,node('JP-NEW'),'sync'));
    assert.deepEqual(next.routing.balancers[0].selector,['HK-','JP-NEW']);
});
test('prefix expansion cannot pull an unrelated node into a group',()=>{
    const before=fixture();before.outbounds.push(node(newTag+'-unrelated'));
    before.routing.balancers=[{tag:'group',selector:[oldTag]}];const snapshot=plain(before);
    assert.throws(()=>refs.prepare(before,1,node(newTag),'sync'),/前缀匹配/);assert.deepEqual(before,snapshot);
});
test('rename cannot silently join a different prefix group',()=>{
    const before=fixture();before.routing.balancers=[{tag:'group',selector:['JP-']}];
    assert.throws(()=>refs.prepare(before,1,node('JP-NEW'),'sync'),/前缀匹配/);
});
test('duplicate, blank, reserved, missing outbound and unsupported modes reject atomically',()=>{
    const before=fixture(), snapshot=plain(before);
    for(const tag of ['direct',' direct ','','  ','__dui_blacklist_fake']) assert.throws(()=>refs.prepare(before,1,node(tag),'sync'));
    assert.throws(()=>refs.prepare(before,99,node(newTag),'sync'));
    assert.throws(()=>refs.prepare(before,1,node(newTag),'invalid'));
    assert.deepEqual(before,snapshot);
});
test('optional sections and unreferenced nodes are supported',()=>{
    const before={outbounds:[node(oldTag)]};
    assert.equal(refs.prepare(before,0,node(newTag),'sync').outbounds[0].tag,newTag);
    assert.equal(refs.inspect(null,oldTag).total,0);
});

// Exercise the real page edit callback, grouped-rule fingerprints and modal confirmation path.
const page=read('web/html/xray.html'), rule=read('web/html/modals/xray_rule_modal.html');
vm.runInContext(page.slice(page.indexOf('  function duiStableValue('),page.indexOf('  const app = new Vue(')),context);
const splitStart=rule.indexOf('  function ruleAdvancedSplit('), splitEnd=rule.indexOf('\n  function ',splitStart+12);
assert(splitStart>=0&&splitEnd>splitStart);
vm.runInContext(rule.slice(splitStart,splitEnd)+rule.slice(rule.indexOf('  function duiBlacklistHash('),rule.indexOf('  function duiEnsureBlacklistOutbound(')),context);
const methods=vm.runInContext('({'+page.slice(page.indexOf('      editOutbound(index) {'),page.indexOf('      deleteOutbound(index) {'))+'})',context);
const outboundMethods=vm.runInContext('({'+page.slice(page.indexOf('      deleteOutbound(index) {'),page.indexOf('      addReverse() {'))+'})',context);
const computed=vm.runInContext('({'+page.slice(page.indexOf('      templateSettings: {'),page.indexOf('      inboundSettings: {'))+page.slice(page.indexOf('      outboundSettings: {'),page.indexOf('      reverseData: {'))+'})',context);
const draftWatcher=vm.runInContext('({'+page.slice(page.indexOf('      xraySetting(value) {'),page.indexOf('    computed: {')).replace(/    },\s*$/, '')+'})',context);
const modal=read('web/html/modals/xray_outbound_modal.html');
context.Outbound=class {static fromJson(value){return Object.assign(new this(),plain(value));}toJson(){return plain({...this});}};
context.ObjectUtil={execute:(fn,...args)=>fn(...args)};
vm.runInContext(modal.slice(modal.indexOf('    const outModal = '),modal.indexOf('    new Vue('))+'\nthis.modal = outModal;',context);
function appFixture() {
    const before=fixture();before.outbounds.unshift({tag:'__dui_blacklist_deny',protocol:'blackhole'});
    before.routing.rules=[{type:'field',domain:['domain:example.test'],outboundTag:oldTag},{type:'field',ip:['192.0.2.0/24'],outboundTag:oldTag}];
    const merged=context.duiBuildRoutingGroups(before.routing.rules,before.outbounds)[0].merged;
    const oldKey=context.duiRuleSourceFingerprint(merged);
    const app={templateSettings:before,outboundData:[{_physicalIndex:0},{_physicalIndex:2}],routeSourceMap:{[oldKey]:['list-1']},warnings:[]};
    app.xraySetting=JSON.stringify(before);
    for (const key of ['templateSettings','outboundSettings','outboundData']) Object.defineProperty(app,key,{...computed[key],configurable:true});
    app.$message={warning:message=>app.warnings.push(message)};
    return {app,oldKey};
}
test('real modal sync preserves hidden outbound positions and grouped rule source metadata',()=>{
    const {app,oldKey}=appFixture();methods.editOutbound.call(app,1);
    context.modal.outbound.tag=newTag;context.modal.ok('sync');
    assert.equal(app.warnings.length,0);assert.equal(app.templateSettings.outbounds[0].tag,'__dui_blacklist_deny');
    assert.equal(app.templateSettings.outbounds[2].tag,newTag);
    assert(app.templateSettings.routing.rules.every(r=>r.outboundTag===newTag));
    const merged=context.duiBuildRoutingGroups(app.templateSettings.routing.rules,app.templateSettings.outbounds)[0].merged;
    const key=context.duiRuleSourceFingerprint(merged);
    assert.notEqual(key,oldKey);assert.deepEqual(plain(app.routeSourceMap[key]),['list-1']);assert.equal(app.routeSourceMap[oldKey],undefined);
    assert.equal(context.modal.visible,false);
});
test('real ordinary confirm leaves routes and source metadata unchanged',()=>{
    const {app,oldKey}=appFixture();methods.editOutbound.call(app,1);context.modal.outbound.tag=newTag;context.modal.ok();
    assert.equal(app.templateSettings.outbounds[2].tag,newTag);assert(app.templateSettings.routing.rules.every(r=>r.outboundTag===oldTag));
    assert.deepEqual(plain(app.routeSourceMap[oldKey]),['list-1']);
});
test('cancel and invalid duplicate cannot modify draft configuration',()=>{
    const {app}=appFixture();const before=plain(app.templateSettings);methods.editOutbound.call(app,1);
    context.modal.outbound.tag='direct';context.modal.ok('sync');assert.equal(context.modal.isValid,false);
    assert.deepEqual(plain(app.templateSettings),before);context.modal.close();assert.deepEqual(plain(app.templateSettings),before);
});
test('stale edit is rejected instead of overwriting a changed outbound',()=>{
    const {app}=appFixture();methods.editOutbound.call(app,1);context.modal.outbound.tag=newTag;
    const changed=plain(app.templateSettings);changed.outbounds[2].settings.domainStrategy='UseIPv6';app.templateSettings=changed;
    const before=plain(app.templateSettings);context.modal.ok('sync');
    assert.deepEqual(plain(app.templateSettings),before);assert.equal(app.warnings.length,1);assert.equal(context.modal.visible,true);
});
test('ordinary rename can be reopened and synchronized without another rename',()=>{
    const {app,oldKey}=appFixture();
    methods.editOutbound.call(app,1);context.modal.outbound.tag=newTag;context.modal.ok();
    methods.editOutbound.call(app,1);
    assert.equal(context.modal.references.routes.length,2,'old-tag routes must remain available for synchronization');
    context.modal.ok('sync');
    assert(app.templateSettings.routing.rules.every(r=>r.outboundTag===newTag));
    const merged=context.duiBuildRoutingGroups(app.templateSettings.routing.rules,app.templateSettings.outbounds)[0].merged;
    assert.deepEqual(plain(app.routeSourceMap[context.duiRuleSourceFingerprint(merged)]),['list-1']);
    assert.equal(app.routeSourceMap[oldKey],undefined);
    assert.deepEqual(plain(app.pendingOutboundRenames),[]);
});
test('repeated ordinary renames retain original and intermediate references',()=>{
    const {app}=appFixture();
    methods.editOutbound.call(app,1);context.modal.outbound.tag=newTag;context.modal.ok();
    let settings=app.templateSettings;settings.routing.rules.push({type:'field',domain:['domain:intermediate.test'],outboundTag:newTag});app.templateSettings=settings;
    methods.editOutbound.call(app,1);context.modal.outbound.tag='THIRD';context.modal.ok();
    methods.editOutbound.call(app,1);assert.equal(context.modal.references.routes.length,3);
    context.modal.outbound.tag='FINAL';context.modal.ok('sync');
    assert(app.templateSettings.routing.rules.every(r=>r.outboundTag==='FINAL'));assert.equal(app.warnings.length,0);
    assert.deepEqual(plain(app.pendingOutboundRenames),[]);
});
test('parameter-only ordinary edit and cancel preserve pending synchronization',()=>{
    const {app}=appFixture();methods.editOutbound.call(app,1);context.modal.outbound.tag=newTag;context.modal.ok();
    const history=plain(app.pendingOutboundRenames);
    methods.editOutbound.call(app,1);context.modal.outbound.tag='CANCELLED';context.modal.close();
    assert.deepEqual(plain(app.pendingOutboundRenames),history);
    methods.editOutbound.call(app,1);context.modal.outbound.settings.domainStrategy='UseIPv6';context.modal.ok();
    methods.editOutbound.call(app,1);assert.equal(context.modal.references.routes.length,2);context.modal.ok('sync');
    assert.equal(app.templateSettings.outbounds[2].settings.domainStrategy,'UseIPv6');
    assert(app.templateSettings.routing.rules.every(r=>r.outboundTag===newTag));
});
test('pending synchronization follows outbound through default-order changes',()=>{
    const {app}=appFixture();methods.editOutbound.call(app,1);context.modal.outbound.tag=newTag;context.modal.ok();
    outboundMethods.setFirstOutbound.call(app,1);
    assert.equal(app.templateSettings.outbounds[0].tag,newTag);
    methods.editOutbound.call(app,0);assert.equal(context.modal.references.routes.length,2);context.modal.ok('sync');
    assert(app.templateSettings.routing.rules.every(r=>r.outboundTag===newTag));assert.equal(app.warnings.length,0);
});
test('deleting the renamed outbound does not transfer history to a replacement',()=>{
    const {app}=appFixture();methods.editOutbound.call(app,1);context.modal.outbound.tag=newTag;context.modal.ok();
    outboundMethods.deleteOutbound.call(app,1);assert.deepEqual(plain(app.pendingOutboundRenames),[]);
    const settings=app.templateSettings;settings.outbounds.push(node(newTag));app.templateSettings=settings;
    methods.editOutbound.call(app,2);assert.equal(context.modal.references.total,0);
});
test('reusing an old tag discards its history and never steals that outbound references',()=>{
    const {app}=appFixture();methods.editOutbound.call(app,1);context.modal.outbound.tag=newTag;context.modal.ok();
    const settings=app.templateSettings;settings.outbounds.push(node(oldTag));app.templateSettings=settings;
    assert.deepEqual(plain(app.pendingOutboundRenames),[]);
    methods.editOutbound.call(app,1);assert.equal(context.modal.references.total,0);context.modal.ok('sync');
    assert(app.templateSettings.routing.rules.every(r=>r.outboundTag===oldTag));
    outboundMethods.deleteOutbound.call(app,3);
    methods.editOutbound.call(app,1);assert.equal(context.modal.references.total,0);
});
test('manual route correction clears pending history and preserves unrelated references',()=>{
    const {app}=appFixture();methods.editOutbound.call(app,1);context.modal.outbound.tag=newTag;context.modal.ok();
    const settings=app.templateSettings;settings.routing.rules.forEach(r=>r.outboundTag='other');app.templateSettings=settings;
    assert.deepEqual(plain(app.pendingOutboundRenames),[]);
    methods.editOutbound.call(app,1);assert.equal(context.modal.references.total,0);context.modal.ok('sync');
    assert(app.templateSettings.routing.rules.every(r=>r.outboundTag==='other'));
});
test('deferred synchronization migrates chain and selector references without double counting',()=>{
    const before=fixture();before.outbounds.push(node(oldTag+'-2'));
    before.routing.balancers=[{tag:'group',selector:[oldTag]}];before.observatory={subjectSelector:['HK-']};
    before.outbounds[2].proxySettings={tag:oldTag};
    const renamed=refs.prepare(before,1,node('JP-NEW'),'replace');
    const pending=refs.rememberRename([],before,1,renamed,'replace');
    const aliases=refs.pendingTags(pending,renamed,1);
    const info=refs.inspect(renamed,['JP-NEW',...aliases]);
    assert.equal(info.balancers,1);assert.equal(info.observatories,1);assert.equal(info.chains,1);
    const synced=plain(refs.prepare(renamed,1,node('JP-NEW'),'sync',aliases));
    assert.deepEqual(synced.routing.balancers[0].selector,[oldTag,'JP-NEW']);
    assert.deepEqual(synced.observatory.subjectSelector,['HK-','JP-NEW']);
    assert.equal(synced.outbounds[2].proxySettings.tag,'JP-NEW');
});
test('deferred synchronization still rejects unsafe prefix expansion atomically',()=>{
    const before=fixture();before.outbounds.push(node(newTag+'-unrelated'));
    before.routing.balancers=[{tag:'group',selector:[oldTag]}];
    const renamed=refs.prepare(before,1,node(newTag),'replace'), snapshot=plain(renamed);
    const pending=refs.rememberRename([],before,1,renamed,'replace');
    assert.throws(()=>refs.prepare(renamed,1,node(newTag),'sync',refs.pendingTags(pending,renamed,1)),/前缀匹配/);
    assert.deepEqual(plain(renamed),snapshot);
});
test('multiple pending outbound renames remain independent',()=>{
    let before=fixture();before.routing.rules.push({type:'field',domain:['domain:second.test'],outboundTag:'other'});
    let next=refs.prepare(before,1,node(newTag),'replace');
    let pending=refs.rememberRename([],before,1,next,'replace');before=next;
    next=refs.prepare(before,2,node('OTHER-NEW'),'replace');pending=refs.rememberRename(pending,before,2,next,'replace');before=next;
    next=refs.prepare(before,1,node(newTag),'sync',refs.pendingTags(pending,before,1));pending=refs.rememberRename(pending,before,1,next,'sync');
    assert.equal(next.routing.rules[11].outboundTag,newTag);assert.equal(next.routing.rules[12].outboundTag,'other');
    assert.equal(pending.length,1);assert.equal(pending[0].tag,'OTHER-NEW');
    assert(!JSON.stringify(next).includes('previousTags'));
});
test('advanced JSON changes prune stale history without losing it during incomplete input',()=>{
    const {app}=appFixture();methods.editOutbound.call(app,1);context.modal.outbound.tag=newTag;context.modal.ok();
    const history=plain(app.pendingOutboundRenames);draftWatcher.xraySetting.call(app,'{');
    assert.deepEqual(plain(app.pendingOutboundRenames),history);
    const settings=app.templateSettings;settings.outbounds=settings.outbounds.filter(o=>o.tag!==newTag);
    draftWatcher.xraySetting.call(app,JSON.stringify(settings));assert.deepEqual(plain(app.pendingOutboundRenames),[]);
});
test('renaming back to the original tag removes obsolete pending history',()=>{
    const {app}=appFixture();methods.editOutbound.call(app,1);context.modal.outbound.tag=newTag;context.modal.ok();
    methods.editOutbound.call(app,1);context.modal.outbound.tag=oldTag;context.modal.ok();
    assert.deepEqual(plain(app.pendingOutboundRenames),[]);
    methods.editOutbound.call(app,1);assert.equal(context.modal.references.routes.length,2);
});
const persistence=vm.runInContext('({'+
    page.slice(page.indexOf('      async getXraySetting() {'),page.indexOf('      async loadRouteSourceMap() {'))+
    page.slice(page.indexOf('      async updateXraySetting() {'),page.indexOf('      async restartXray() {'))+
    page.slice(page.indexOf('      async resetXrayConfigToDefault() {'),page.indexOf('      async loadManagedBlacklist() {'))+'})',context);
(async()=>{
    const {app}=appFixture();
    methods.editOutbound.call(app,1);context.modal.outbound.tag=newTag;context.modal.ok();
    app.loading=()=>{};app.loadingStates={fetched:true};app.persistRouteSourceMap=async()=>{};app.getXrayResult=async()=>{};
    app.getXraySetting=()=>persistence.getXraySetting.call(app);
    context.PromiseUtil={sleep:async()=>{}};
    context.HttpUtil={post:async()=>({success:false})};
    const history=plain(app.pendingOutboundRenames);
    await persistence.updateXraySetting.call(app);
    assert.deepEqual(plain(app.pendingOutboundRenames),history);
    methods.editOutbound.call(app,1);assert.equal(context.modal.references.routes.length,2);context.modal.ok('sync');cases++;
    methods.editOutbound.call(app,1);context.modal.outbound.tag='NEXT';context.modal.ok();
    let posted;
    context.HttpUtil={post:async(url,body)=>{
        if(url==='/dui/xray/update'){posted=JSON.parse(body.xraySetting);return {success:true};}
        return {success:true,obj:JSON.stringify({xraySetting:fixture(),inboundTags:[]})};
    }};
    await persistence.updateXraySetting.call(app);
    assert.deepEqual(plain(app.pendingOutboundRenames),[]);assert(!JSON.stringify(posted).includes('previousTags'));cases++;
    methods.editOutbound.call(app,1);context.modal.outbound.tag=newTag;context.modal.ok();
    assert(app.pendingOutboundRenames.length>0);
    context.HttpUtil={get:async()=>({success:true,obj:fixture()})};
    await persistence.resetXrayConfigToDefault.call(app);
    assert.deepEqual(plain(app.pendingOutboundRenames),[]);cases++;
    console.log(`Outbound reference synchronization: ${cases} cases passed`);
})().catch(error=>{console.error(error);process.exit(1)});
