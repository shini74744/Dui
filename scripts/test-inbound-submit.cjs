const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const root = path.resolve(__dirname, '..');
const ctx = vm.createContext({
    console, URL, URLSearchParams,
    window: {btoa: value => Buffer.from(value, 'binary').toString('base64'), atob: value => Buffer.from(value, 'base64').toString('binary'), crypto: require('node:crypto').webcrypto, location: {protocol: 'https:'}},
    location: {hostname: 'test.example'},
    inModal: {dbInbound: null, remarkOnly: () => false},
});
for (const file of ['util/index.js', 'model/inbound.js', 'model/dbinbound.js']) {
    vm.runInContext(fs.readFileSync(path.join(root, 'web/assets/js', file), 'utf8'), ctx);
}
const html = fs.readFileSync(path.join(root, 'web/html/inbounds.html'), 'utf8');
const start = html.indexOf('            async addInbound(inbound, dbInbound) {');
const end = html.indexOf('            openAddClient(', start);
assert(start >= 0 && end > start, 'real submission handlers not found');
const app = vm.runInContext('({' + html.slice(start, end) + '})', ctx);
const db = vm.runInContext('new DBInbound({id:99, remark:"submit-test", enable:true})', ctx);
let submitted;
app.submit = async (url, data) => { submitted = {url, data}; };
async function check(inbound, expectedStream) {
    for (const method of ['addInbound', 'updateInbound']) {
        await app[method](inbound, db);
        assert.equal(submitted.data.protocol, inbound.protocol);
        assert.deepEqual(submitted.data.streamSettings === undefined ? undefined :
            JSON.parse(submitted.data.streamSettings), expectedStream, method + ' stream configuration');
        assert.deepEqual(JSON.parse(submitted.data.settings), JSON.parse(inbound.settings.toString()));
    }
}
(async () => {
    const hy = vm.runInContext(`(() => {
        const i = new Inbound(); i.protocol = Protocols.HYSTERIA;
        i.clients[0].auth = 'test-auth'; i.clients[0].email = 'test-user';
        i.stream.hysteria.udpIdleTimeout = 123;
        i.stream.tls.certs[0].certFile = '/tmp/test-cert.pem';
        i.stream.tls.certs[0].keyFile = '/tmp/test-key.pem';
        return i;
    })()`, ctx);
    assert.equal(hy.canEnableStream(), false, 'fixed HY2 transport stays hidden in the UI');
    const expected = JSON.parse(hy.stream.toString());
    assert.equal(expected.network, 'hysteria');
    assert.equal(expected.security, 'tls');
    assert.equal(expected.hysteriaSettings.version, 2);
    assert.equal(expected.hysteriaSettings.udpIdleTimeout, 123);
    assert.deepEqual(expected.tlsSettings.alpn, ['h3']);
    await check(hy, expected);
    const edited = vm.runInContext('Inbound.fromJson(' + JSON.stringify(hy.toJson()) + ')', ctx);
    await check(edited, expected);
    for (const protocol of ['VLESS', 'VMESS', 'TROJAN', 'SHADOWSOCKS']) {
        const inbound = vm.runInContext('(() => {const i=new Inbound(); i.protocol=Protocols.' + protocol + ';return i;})()', ctx);
        await check(inbound, JSON.parse(inbound.stream.toString()));
    }
    const mixed = vm.runInContext('(() => {const i=new Inbound();i.protocol=Protocols.MIXED;return i;})()', ctx);
    mixed.stream.sockopt = null;
    await check(mixed, undefined);
    mixed.stream.sockopt = {toJson: () => ({tcpFastOpen: true})};
    await check(mixed, {sockopt: {tcpFastOpen: true}});
    ctx.inModal.dbInbound = db;
    ctx.inModal.remarkOnly = () => true;
    await app.updateInbound(hy, db);
    assert.equal(submitted.url, '/dui/api/inbounds/updateRemark/99');
    assert.deepEqual(JSON.parse(JSON.stringify(submitted.data)), {remark: db.remark});
    console.log('PASS: real add/update handlers preserve HY2 transport/TLS/auth; other transports, sockopt and remark-only path unchanged');
})().catch(error => {console.error(error); process.exitCode = 1;});
