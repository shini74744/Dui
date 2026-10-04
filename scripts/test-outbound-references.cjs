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
    assert.deepEqual(app.templateSettings,before);context.modal.close();assert.deepEqual(app.templateSettings,before);
});
test('stale edit is rejected instead of overwriting a changed outbound',()=>{
    const {app}=appFixture();methods.editOutbound.call(app,1);context.modal.outbound.tag=newTag;
    app.templateSettings=plain(app.templateSettings);app.templateSettings.outbounds[2].settings.domainStrategy='UseIPv6';
    const before=plain(app.templateSettings);context.modal.ok('sync');
    assert.deepEqual(app.templateSettings,before);assert.equal(app.warnings.length,1);assert.equal(context.modal.visible,true);
});
console.log(`Outbound reference synchronization: ${cases} cases passed`);
