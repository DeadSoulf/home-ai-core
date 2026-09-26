// Run after CTest: node tests/test_hypervisor_ui.cjs [path-to-playwright]
const fs = require('fs');
const path = require('path');
const assert = require('assert/strict');
const {chromium} = require(process.argv[2] || 'playwright');
(async () => {
    const browser = await chromium.launch({headless: true, channel: process.argv[3]});
    const page = await browser.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    const uuid = '11111111-2222-3333-4444-555555555555';
    let state = 'shutoff', allowed = ['start'], posts = [], deny = false, broken = false;
    let host = {libvirt_available: true, libvirt_connected: true, kvm_accessible: true, qemu_available: true};
    let statusMessage = '';
    let releaseAction;
    await page.route('http://homeai.test/**', async route => {
        const url = new URL(route.request().url());
        if (url.pathname === '/assets/i18n.js') {
            const script = '(() => { const catalog = ' + JSON.stringify(fs.readFileSync('web/ui/translations.tsv', 'utf8')) + ';\n' + fs.readFileSync('web/ui/localization.js', 'utf8') + '\n})();';
            return route.fulfill({contentType: 'application/javascript', body: script});
        }
        if (url.pathname === '/api/hypervisor/action') {
            posts.push(new URLSearchParams(route.request().postData()));
            assert.equal(route.request().headers()['x-homeai-request'], '1');
            if (releaseAction) await new Promise(resolve => { releaseAction = resolve; });
            if (deny) return route.fulfill({status: 409, json: {success: false, code: 'state_changed', message: 'VM state changed'}});
            const action = posts.at(-1).get('action');
            if (action === 'start') {state = 'running'; allowed = ['shutdown', 'reboot', 'force-off'];}
            if (action === 'force-off') {state = 'shutoff'; allowed = ['start'];}
            return route.fulfill({status: 202, json: {success: true, code: 'accepted', state}});
        }
        if (url.pathname === '/api/hypervisor') return route.fulfill({json: {
            success: !broken, message: statusMessage || (broken ? 'Inventory failed' : ''), host,
            machines: [{uuid, name: '<VM & test>', state, active: state !== 'shutoff', allowed_actions: allowed}]
        }});
        if (url.pathname.startsWith('/api/')) return route.fulfill({status: 503, json: {success: false}});
        return route.fulfill({contentType: 'text/html', body: fs.readFileSync(path.join('build/ui-fixtures/hypervisor.html'), 'utf8')});
    });
    try {
        await page.goto('http://homeai.test/hypervisor');
        await page.waitForFunction(() => document.querySelectorAll('#hypervisor-vm-list button').length === 4);
        const buttons = page.locator('#hypervisor-vm-list button');
        assert.equal(await buttons.nth(0).isEnabled(), true);
        assert.equal(await buttons.nth(1).isDisabled(), true);
        assert.equal(await page.locator('#hypervisor-vm-list strong').innerText(), '<VM & test>');
        page.once('dialog', dialog => dialog.dismiss());
        await buttons.nth(0).click();
        assert.equal(posts.length, 0);
        releaseAction = true;
        page.once('dialog', dialog => dialog.accept());
        await buttons.nth(0).click();
        await page.waitForFunction(() => document.querySelector('#hypervisor-vm-list button').disabled);
        assert.equal(await page.locator('#hypervisor-vm-list button:enabled').count(), 0);
        while (typeof releaseAction !== 'function') await new Promise(resolve => setTimeout(resolve, 10));
        const release = releaseAction; releaseAction = null; release();
        await page.waitForFunction(() => !document.querySelectorAll('#hypervisor-vm-list button')[1].disabled);
        assert.equal(posts[0].get('confirmation'), uuid);
        assert.equal(posts[0].get('expected_state'), 'shutoff');
        page.once('dialog', dialog => dialog.accept('wrong UUID'));
        await buttons.nth(3).click();
        assert.equal(posts.length, 1);
        page.once('dialog', dialog => dialog.accept());
        await buttons.nth(1).click();
        await page.waitForFunction(() => document.querySelector('#hypervisor-action-message').textContent.includes('Команда принята'));
        assert.equal(state, 'running'); // Never pretend graceful shutdown completed.
        deny = true;
        page.once('dialog', dialog => dialog.accept());
        await buttons.nth(2).click();
        await page.waitForFunction(() => document.querySelector('#hypervisor-action-message').textContent.includes('VM state changed'));
        await page.evaluate(() => updateHypervisor());
        assert((await page.locator('#hypervisor-action-message').innerText()).includes('VM state changed'));
        deny = false;
        page.once('dialog', dialog => dialog.accept(uuid));
        await buttons.nth(3).click();
        await page.waitForFunction(() => !document.querySelector('#hypervisor-vm-list button').disabled);
        allowed = [];
        await page.evaluate(() => updateHypervisor());
        assert.equal(await page.locator('#hypervisor-vm-list button:enabled').count(), 0);
        broken = true;
        await page.evaluate(() => updateHypervisor());
        assert.equal(await page.locator('#hypervisor-vm-list button').count(), 0);
        broken = false;
        host = {libvirt_available: false, libvirt_connected: false};
        statusMessage = 'libvirt is not installed. Hypervisor Core is running in detection-only mode.';
        await page.evaluate(() => updateHypervisor());
        assert.equal(await page.locator('#hypervisor-message').innerText(), 'libvirt не установлен. Ядро виртуализации работает в режиме обнаружения оборудования.');
        assert((await page.locator('#hypervisor-setup code').innerText()).endsWith(' install'));
        await page.selectOption('#language-selector', 'en');
        await page.waitForFunction(() => document.documentElement.lang === 'en');
        await page.waitForFunction(() => document.querySelector('#hypervisor-message')?.textContent.includes('detection-only'));
        assert.equal(await page.locator('#hypervisor-message').innerText(), statusMessage);
        host = {libvirt_available: true, libvirt_connected: false, qemu_available: true};
        broken = true;
        await page.evaluate(() => updateHypervisor());
        assert((await page.locator('#hypervisor-setup').innerText()).includes('connection is unavailable'));
        assert((await page.locator('#hypervisor-setup code').innerText()).endsWith(' check'));
        broken = false;
        host.libvirt_connected = true;
        await page.evaluate(() => updateHypervisor());
        assert((await page.locator('#hypervisor-setup').innerText()).includes('nested virtualization'));
        host.kvm_accessible = true;
        await page.evaluate(() => updateHypervisor());
        assert(await page.locator('#hypervisor-setup').isHidden());
        assert.deepEqual(errors, []);
        console.log('Hypervisor browser tests passed');
    } finally { await browser.close(); }
})().catch(error => {console.error(error); process.exit(1);});
