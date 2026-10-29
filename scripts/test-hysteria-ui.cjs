const fs = require('node:fs');
const assert = require('node:assert/strict');
const {chromium} = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const base = process.env.DUI_FIXTURE_BASE;
const out = process.env.DUI_PROBE_TEST_OUTPUT || '/tmp';
(async () => {
    const browser = await chromium.launch({headless: true, executablePath: process.env.CHROMIUM_PATH, args: ['--no-sandbox']});
    const cases = [], errors = [];
    try {
        for (const [index, [width, theme]] of [[390, 'dark'], [390, 'light'], [1440, 'dark'], [1440, 'light']].entries()) {
            const context = await browser.newContext({viewport: {width, height: 1000}});
            const response = await context.request.post(base + '/login', {form: {
                username: process.env.DUI_TEST_USERNAME, password: process.env.DUI_TEST_PASSWORD,
            }});
            assert.equal((await response.json()).success, true);
            await context.addCookies([{name: 'lang', value: 'zh-CN', url: base + '/'}]);
            await context.addInitScript(theme => {
                localStorage.setItem('dark-mode', String(theme === 'dark'));
                localStorage.setItem('isRefreshEnabled', 'false');
            }, theme);
            const page = await context.newPage();
            page.setDefaultTimeout(15000);
            page.on('pageerror', e => errors.push(e.message));
            await page.goto(base + '/dui/inbounds');
            await page.waitForFunction(() => typeof app !== 'undefined' && app.loadingStates.fetched);
            await page.evaluate(() => app.openAddInbound());
            const modal = page.locator('.ant-modal:visible');
            await modal.waitFor();
            await modal.locator('.ant-select').first().click();
            await page.locator('.ant-select-dropdown:visible li').filter({hasText: /^hysteria$/}).click();
            const port = JSON.parse(process.env.DUI_TEST_PORTS)[index];
            await page.evaluate(({port, cert, key, index}) => {
                inModal.dbInbound.remark = 'HY2 browser test ' + index;
                inModal.inbound.listen = '127.0.0.1';
                inModal.inbound.port = port;
                inModal.inbound.clients[0].email = 'hy2-browser-' + index;
                inModal.inbound.stream.tls.certs[0].certFile = cert;
                inModal.inbound.stream.tls.certs[0].keyFile = key;
            }, {port, cert: process.env.DUI_TEST_CERT, key: process.env.DUI_TEST_KEY, index});
            assert.match(await modal.innerText(), /TLS 1.3/);
            assert.match(await modal.innerText(), /h3/);
            const bounds = await modal.boundingBox();
            assert(bounds.x >= 0 && bounds.x + bounds.width <= width + 1, 'HY2 modal exceeds viewport');
            assert.equal(await modal.evaluate(el => el.scrollWidth <= el.clientWidth + 2), true, 'HY2 modal content overflows');
            await page.screenshot({path: out + '/hy2-create-' + width + '-' + theme + '.png'});
            const addedResponse = page.waitForResponse(r => r.url().endsWith('/dui/api/inbounds/add') && r.request().method() === 'POST');
            await modal.locator('.ant-modal-footer button.ant-btn-primary').click();
            const added = await (await addedResponse).json();
            assert.equal(added.success, true, added.msg);
            const id = added.obj.id;
            const first = JSON.parse(added.obj.streamSettings);
            assert.equal(first.network, 'hysteria');
            assert.equal(first.security, 'tls');
            assert.equal(first.hysteriaSettings.version, 2);
            assert.deepEqual(first.tlsSettings.alpn, ['h3']);
            await page.waitForFunction(id => !inModal.visible && app.dbInbounds.some(r => r.id === id), id);
            await page.evaluate(id => app.openEditInbound(id), id);
            await modal.waitFor();
            await page.evaluate(() => {inModal.inbound.stream.hysteria.udpIdleTimeout = 123;});
            const updatedResponse = page.waitForResponse(r => r.url().endsWith('/dui/api/inbounds/update/' + id) && r.request().method() === 'POST');
            await modal.locator('.ant-modal-footer button.ant-btn-primary').click();
            const updated = await (await updatedResponse).json();
            assert.equal(updated.success, true, updated.msg);
            await page.waitForFunction(() => !inModal.visible);
            await page.reload();
            await page.waitForFunction(id => typeof app !== 'undefined' && app.dbInbounds.some(r => r.id === id), id);
            const saved = await page.evaluate(id => app.dbInbounds.find(r => r.id === id).streamSettings, id);
            assert.equal(JSON.parse(saved).hysteriaSettings.udpIdleTimeout, 123);
            assert.deepEqual(JSON.parse(saved).tlsSettings, first.tlsSettings);
            const deleted = await context.request.post(base + '/dui/api/inbounds/del/' + id);
            assert.equal((await deleted.json()).success, true);
            cases.push({width, theme, create: true, edit: true, reload: true, cleanup: true});
            await context.close();
        }
        assert.deepEqual(errors, []);
        fs.writeFileSync(out + '/hy2-browser.json', JSON.stringify({passed: true, cases, errors}, null, 2));
        console.log(JSON.stringify({HY2_real_browser_create_edit_reload: true, cases: cases.length, errors}));
    } finally {await browser.close();}
})().catch(error => {console.error(error); process.exitCode = 1;});
