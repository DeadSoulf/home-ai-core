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
    let state = 'shutoff';
    let autostart = false;
    let allowed = [];
    let posts = [];
    let deny = false;
    let broken = false;
    let host = {libvirt_available: true, libvirt_connected: true, kvm_accessible: true, qemu_available: true};
    let statusMessage = '';
    let releaseAction;
    const createPreviews = [];
    const creates = [];

    const refreshAllowed = () => {
        const auto = autostart ? 'autostart-off' : 'autostart-on';
        if (state === 'shutoff') allowed = ['start', auto];
        else if (state === 'paused') allowed = ['resume', 'force-off', auto];
        else if (state === 'running') allowed = ['shutdown', 'reboot', 'pause', 'force-off', auto];
        else allowed = [];
    };
    refreshAllowed();

    await page.route('http://homeai.test/**', async route => {
        const url = new URL(route.request().url());

        if (url.pathname === '/assets/i18n.js') {
            const script = '(() => { const catalog = ' +
                JSON.stringify(fs.readFileSync('web/ui/translations.tsv', 'utf8')) + ';\n' +
                fs.readFileSync('web/ui/localization.js', 'utf8') + '\n})();';
            return route.fulfill({contentType: 'application/javascript', body: script});
        }

        if (url.pathname === '/api/hypervisor/create/preview') {
            const form = new URLSearchParams(route.request().postData());
            createPreviews.push(form);
            assert.equal(route.request().headers()['x-homeai-request'], '1');
            return route.fulfill({status: 200, json: {
                success: true, code: 'preview_ready',
                message: 'Validated VM definition preview.',
                xml: "<domain type='kvm'>\n  <name>" + form.get('name') + "</name>\n</domain>\n"
            }});
        }

        if (url.pathname === '/api/hypervisor/create') {
            const form = new URLSearchParams(route.request().postData());
            creates.push(form);
            assert.equal(route.request().headers()['x-homeai-request'], '1');
            return route.fulfill({status: 201, json: {
                success: true, code: 'created',
                message: 'Persistent VM definition created.',
                name: form.get('name'),
                uuid: 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee',
                state: 'shutoff'
            }});
        }

        if (url.pathname === '/api/hypervisor/action') {
            posts.push(new URLSearchParams(route.request().postData()));
            assert.equal(route.request().headers()['x-homeai-request'], '1');

            if (releaseAction) {
                await new Promise(resolve => {
                    releaseAction = resolve;
                });
            }

            if (deny) {
                return route.fulfill({
                    status: 409,
                    json: {success: false, code: 'state_changed', message: 'VM state changed'}
                });
            }

            const action = posts.at(-1).get('action');
            if (action === 'start') state = 'running';
            if (action === 'pause') state = 'paused';
            if (action === 'resume') state = 'running';
            if (action === 'autostart-on') autostart = true;
            if (action === 'autostart-off') autostart = false;
            if (action === 'force-off') state = 'shutoff';
            refreshAllowed();

            return route.fulfill({
                status: 202,
                json: {success: true, code: 'accepted', state}
            });
        }

        if (url.pathname === '/api/hypervisor') {
            return route.fulfill({json: {
                success: !broken,
                message: statusMessage || (broken ? 'Inventory failed' : ''),
                host,
                capabilities: {create_preview: true, create: true},
                machines: [{
                    uuid,
                    name: '<VM & test>',
                    state,
                    active: state !== 'shutoff',
                    autostart,
                    allowed_actions: allowed
                }]
            }});
        }

        if (url.pathname.startsWith('/api/')) {
            return route.fulfill({status: 503, json: {success: false}});
        }

        return route.fulfill({
            contentType: 'text/html',
            body: fs.readFileSync(path.join('build/ui-fixtures/hypervisor.html'), 'utf8')
        });
    });

    try {
        await page.goto('http://homeai.test/hypervisor');
        await page.waitForFunction(() => document.querySelectorAll('#hypervisor-vm-list button').length === 2);
        assert.equal(await page.getByRole('button', {name: 'Запустить VM'}).isEnabled(), true);
        assert.equal(await page.getByRole('button', {name: 'Включить автозапуск VM'}).isEnabled(), true);
        assert.equal(await page.locator('#hypervisor-vm-list strong').innerText(), '<VM & test>');
        await page.fill('#hypervisor-create-preview-form input[name="name"]', 'preview-vm');
        await page.getByRole('button', {name: 'Проверить конфигурацию'}).click();
        await page.waitForFunction(() => document.querySelector('#hypervisor-create-preview-message').textContent.includes('Конфигурация VM валидна'));
        assert.equal(createPreviews.length, 1);
        assert.equal(createPreviews[0].get('name'), 'preview-vm');
        assert((await page.locator('#hypervisor-create-preview-xml').innerText()).includes('<name>preview-vm</name>'));
        assert.equal(await page.getByRole('button', {name: 'Создать VM'}).isVisible(), true);

        page.once('dialog', dialog => dialog.dismiss());
        await page.getByRole('button', {name: 'Создать VM'}).click();
        assert.equal(creates.length, 0);

        page.once('dialog', dialog => dialog.accept('preview-vm'));
        await page.getByRole('button', {name: 'Создать VM'}).click();
        await page.waitForFunction(() =>
            document.querySelector('#hypervisor-create-preview-message').textContent.includes('VM создана как постоянная конфигурация'));
        assert.equal(creates.length, 1);
        assert.equal(creates[0].get('confirmation'), 'preview-vm');
        assert.equal(creates[0].get('name'), 'preview-vm');

        page.once('dialog', dialog => dialog.dismiss());
        await page.getByRole('button', {name: 'Запустить VM'}).click();
        assert.equal(posts.length, 0);

        releaseAction = true;
        page.once('dialog', dialog => dialog.accept());
        await page.getByRole('button', {name: 'Запустить VM'}).click();
        await page.waitForFunction(() => document.querySelector('#hypervisor-vm-list button').disabled);
        assert.equal(await page.locator('#hypervisor-vm-list button:enabled').count(), 0);
        while (typeof releaseAction !== 'function') {
            await new Promise(resolve => setTimeout(resolve, 10));
        }
        const release = releaseAction;
        releaseAction = null;
        release();

        await page.waitForFunction(() =>
            [...document.querySelectorAll('#hypervisor-vm-list button')]
                .some(button => button.textContent.includes('Приостановить VM')));
        assert.equal(posts[0].get('confirmation'), uuid);
        assert.equal(posts[0].get('expected_state'), 'shutoff');

        page.once('dialog', dialog => dialog.accept('wrong UUID'));
        await page.getByRole('button', {name: 'Принудительно выключить VM'}).click();
        assert.equal(posts.length, 1);

        page.once('dialog', dialog => dialog.accept());
        await page.getByRole('button', {name: 'Приостановить VM'}).click();
        await page.waitForFunction(() =>
            document.querySelector('#hypervisor-action-message').textContent.includes('Состояние VM обновлено'));
        assert.equal(state, 'paused');
        assert.equal(await page.getByRole('button', {name: 'Продолжить VM'}).isEnabled(), true);

        page.once('dialog', dialog => dialog.accept());
        await page.getByRole('button', {name: 'Продолжить VM'}).click();
        await page.waitForFunction(() =>
            [...document.querySelectorAll('#hypervisor-vm-list button')]
                .some(button => button.textContent.includes('Приостановить VM')));
        assert.equal(state, 'running');

        page.once('dialog', dialog => dialog.accept());
        await page.getByRole('button', {name: 'Включить автозапуск VM'}).click();
        await page.waitForFunction(() =>
            document.querySelector('#hypervisor-action-message').textContent.includes('Настройка автозапуска VM обновлена'));
        assert.equal(autostart, true);
        assert.equal(await page.getByRole('button', {name: 'Отключить автозапуск VM'}).isEnabled(), true);

        page.once('dialog', dialog => dialog.accept());
        await page.getByRole('button', {name: 'Завершить работу VM'}).click();
        await page.waitForFunction(() =>
            document.querySelector('#hypervisor-action-message').textContent.includes('Команда принята'));
        assert.equal(state, 'running');

        deny = true;
        page.once('dialog', dialog => dialog.accept());
        await page.getByRole('button', {name: 'Перезагрузить VM'}).click();
        await page.waitForFunction(() =>
            document.querySelector('#hypervisor-action-message').textContent.includes('VM state changed'));
        await page.evaluate(() => updateHypervisor());
        assert((await page.locator('#hypervisor-action-message').innerText()).includes('VM state changed'));

        deny = false;
        page.once('dialog', dialog => dialog.accept(uuid));
        await page.getByRole('button', {name: 'Принудительно выключить VM'}).click();
        await page.waitForFunction(() =>
            [...document.querySelectorAll('#hypervisor-vm-list button')]
                .some(button => button.textContent.includes('Запустить VM')));
        assert.equal(state, 'shutoff');

        allowed = [];
        await page.evaluate(() => updateHypervisor());
        assert.equal(await page.locator('#hypervisor-vm-list button').count(), 0);

        broken = true;
        await page.evaluate(() => updateHypervisor());
        assert.equal(await page.locator('#hypervisor-vm-list button').count(), 0);

        broken = false;
        host = {libvirt_available: false, libvirt_connected: false};
        statusMessage = 'libvirt is not installed. Hypervisor Core is running in detection-only mode.';
        await page.evaluate(() => updateHypervisor());
        assert.equal(
            await page.locator('#hypervisor-message').innerText(),
            'libvirt не установлен. Ядро виртуализации работает в режиме обнаружения оборудования.'
        );
        assert((await page.locator('#hypervisor-setup code').innerText()).endsWith(' install'));

        await page.selectOption('#language-selector', 'en');
        await page.waitForFunction(() => document.documentElement.lang === 'en');
        await page.waitForFunction(() =>
            document.querySelector('#hypervisor-message')?.textContent.includes('detection-only'));
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
    } finally {
        await browser.close();
    }
})().catch(error => {
    console.error(error);
    process.exit(1);
});
