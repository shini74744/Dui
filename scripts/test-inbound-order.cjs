const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const {merge} = require('../web/assets/js/util/inbound-order.js');
assert.deepEqual(merge([1,4,9,14,20],[4,14],[14,4]),[1,14,9,4,20]);
assert.deepEqual(merge([1,4,9,14,20],[9,14,20],[20,9,14]),[1,4,20,9,14]);
assert.deepEqual(merge([1,4,9],[1,4,9],[9,1,4]),[9,1,4]);
for (const [before,after] of [[[],[]],[[1,1],[1,1]],[[1,4],[4,99]],[[4,1],[1,4]],[[99],[99]],[[1,4],[1]]]) {
    assert.equal(merge([1,4,9],before,after),null);
}
const html = fs.readFileSync(__dirname+'/../web/html/inbounds.html','utf8');
const methods = html.slice(html.indexOf('            inboundOrderDragChanged(active) {'),html.indexOf('            clientRouteKey(')) +
    html.slice(html.indexOf('            async getDBInbounds() {'),html.indexOf('            async getOnlineUsers() {'));
const context = vm.createContext({DuiInboundOrder:{merge}, console});
vm.runInContext('var methods = ({'+methods+'});', context);
const state = {dbInbounds:[{id:1},{id:4},{id:9}],inboundListRequest:0,inboundOrderSaving:false,inboundOrderDragging:false,
    loadClientRouteReferences(){},setInbounds(rows){this.dbInbounds=rows},...context.methods};
const ids = () => Array.from(state.dbInbounds, r=>r.id);
const deferred = () => {let resolve;const promise=new Promise(r=>resolve=r);return {promise,resolve};};
(async()=>{
    let response=deferred(),calls=0;
    context.HttpUtil={get:()=>{calls++;return response.promise},post:async()=>({success:true,obj:[]})};
    const pending=state.getDBInbounds();
    state.inboundOrderDragChanged(true);
    await state.getDBInbounds();assert.equal(calls,1,'refresh must defer during drag');
    response.resolve({success:true,obj:[{id:9},{id:4},{id:1}]});await pending;
    assert.deepEqual(ids(),[1,4,9],'an in-flight response replaced a dragging list');
    state.inboundOrderDragChanged(false);
    // A newer refresh wins even if the old request finishes last.
    const first=deferred(),second=deferred();let index=0;
    context.HttpUtil.get=()=>[first,second][index++].promise;
    const old=state.getDBInbounds(),fresh=state.getDBInbounds();
    second.resolve({success:true,obj:[{id:4},{id:1},{id:9}]});await fresh;
    first.resolve({success:true,obj:[{id:1},{id:4},{id:9}]});await old;
    assert.deepEqual(ids(),[4,1,9]);
    // Failed saves retain the current order; successful saves are never concurrent.
    let refreshes=0;state.getDBInbounds=async()=>{refreshes++};
    const save=deferred();let writes=0;
    context.HttpUtil.post=()=>{writes++;return save.promise};
    const change={before:[4,1],after:[1,4]};
    const saving=state.saveInboundOrder(change);
    assert.equal(state.inboundOrderSaving,true);
    await state.saveInboundOrder(change);assert.equal(writes,1);
    save.resolve({success:false});await saving;
    assert.deepEqual(ids(),[4,1,9]);assert.equal(state.inboundOrderSaving,false);assert.equal(refreshes,1);
    context.HttpUtil.post=async(_url,body)=>{
        assert.deepEqual(JSON.parse(body.baseIds),[4,1,9]);
        assert.deepEqual(JSON.parse(body.ids),[1,4,9]);
        return {success:true};
    };
    await state.saveInboundOrder(change);assert.deepEqual(ids(),[1,4,9]);assert.equal(refreshes,2);
    console.log('Inbound ordering: visible slots, stale requests, drag refresh, failure recovery and single-writer saves passed');
})().catch(err=>{console.error(err);process.exitCode=1});
