// Run after test-web-ui: node tests/test_localization.cjs [path-to-playwright]
const fs = require('fs');
const path = require('path');
const assert = require('assert/strict');
const {chromium} = require(process.argv[2] || 'playwright');
const root = path.resolve(__dirname, '..');
const script = '(() => { const catalog = ' + JSON.stringify(fs.readFileSync(path.join(root,'web/ui/translations.tsv'),'utf8')) + ';\n' + fs.readFileSync(path.join(root,'web/ui/localization.js'),'utf8') + '\n})();';
(async () => {
    const browser = await chromium.launch({channel:process.env.HOMEAI_BROWSER_CHANNEL || 'msedge', headless:true});
    const page = await browser.newPage();
    const errors=[];
    page.on('pageerror', e => errors.push(e.message));
    let gpuSelected='', devices=[{pci_address:'0000:01:00.0',vendor:'NVIDIA',vendor_id:'0x10de',device_id:'0x1234',driver:''}];
    await page.route('http://homeai.test/**', async route => {
        const url=new URL(route.request().url());
        if(url.pathname==='/assets/i18n.js') return route.fulfill({contentType:'application/javascript',body:script});
        if(url.pathname==='/api/admin/accelerator') {gpuSelected=new URLSearchParams(route.request().postData()).get('pci_address');return route.fulfill({json:{success:true}});}
        if(url.pathname==='/api/admin/gpus') return route.fulfill({json:{available:true,selected:gpuSelected,selected_present:devices.some(d=>d.pci_address===gpuSelected),devices}});
        if(url.pathname==='/api/update/status') return route.fulfill({json:{branch:'develop',local_sha:'0123456789abcdef',remote_sha:'0123456789abcdef',state:'ready_to_restart',message:'Обновление установлено и протестировано. Требуется перезапуск.',restart_required:true,last_output:'Original compiler output'}});
        if(url.pathname==='/api/modules') return route.fulfill({json:{modules:[{name:'security',state:'running',health:'healthy',message:'Authentication and sessions are ready.',dependencies:[]}]}});
        if(url.pathname==='/api/network/vpn') return route.fulfill({json:{available:true,profiles:[{name:'test-profile',active:false}]}});
        if(url.pathname==='/api/storage') return route.fulfill({json:{volumes:[]}});
        if(url.pathname==='/api/storage/devices') return route.fulfill({json:{devices:[],helper_installed:false}});
        if(url.pathname.startsWith('/api/')) return route.fulfill({status:503,json:{error:'test_unavailable'}});
        const name=url.pathname==='/'?'home':url.pathname.slice(1);
        return route.fulfill({contentType:'text/html',body:fs.readFileSync(path.join(root,'build/ui-fixtures',name+'.html'),'utf8')});
    });
    try {
        await page.goto('http://homeai.test/');
        assert.equal(await page.locator('html').getAttribute('lang'),'ru');
        assert.equal(await page.locator('a[href="/network"] span').last().innerText(),'Сеть');
        await page.selectOption('#language-selector','en');
        await page.waitForFunction(()=>document.documentElement.lang==='en');
        for(const file of fs.readdirSync(path.join(root,'build/ui-fixtures'))) {
            await page.goto('http://homeai.test/'+(file==='home.html'?'':file.replace('.html','')));
            await page.waitForFunction(()=>document.querySelector('#language-selector')?.value==='en');
            const text=await page.locator('body').innerText();
            // The language option intentionally retains its native name.
            const remaining=text.replaceAll('Русский','').split('\n').filter(x=>/[А-Яа-яЁё]/.test(x));
            assert.deepEqual(remaining,[],file+' has untranslated text');
            assert(text.includes('Copyright © TexNik'));
            assert(!(await page.title()).includes('<span'));
        }
        await page.goto('http://homeai.test/admin');
        await page.getByRole('button',{name:'Assign to AI',exact:true}).click();
        await page.waitForFunction(()=>document.querySelector('#gpu-action').textContent==='GPU assignment saved.');
        assert.equal(gpuSelected,'0000:01:00.0');
        devices=[];
        await page.evaluate(()=>updateGpus());
        assert((await page.locator('#gpu-notice').innerText()).includes('Selected GPU is missing'));
        await page.getByRole('button',{name:'Clear GPU assignment',exact:true}).click();
        await page.waitForFunction(()=>document.querySelector('#gpu-selection').textContent.includes('Not assigned'));
        await page.selectOption('#language-selector','ru');
        await page.waitForFunction(()=>document.documentElement.lang==='ru');
        await page.goto('http://homeai.test/system');
        assert.equal(await page.locator('html').getAttribute('lang'),'ru');
        assert.equal(await page.evaluate(()=>tr('RUNNING')),'Работает');
        assert.equal(await page.evaluate(()=>tr('Invalid username or password')),'Неверное имя пользователя или пароль');
        assert.equal(await page.evaluate(()=>tr('/mnt/user-files')), '/mnt/user-files');
        assert.equal(await page.evaluate(()=>tr('Устройство: /mnt/admin/Сеть')), 'Устройство: /mnt/admin/Сеть');
        assert((await page.locator('#module-list').innerText()).includes('Аутентификация и сессии готовы.'));
        assert((await page.locator('#update-output').innerText()).includes('Original compiler output'));
        assert.deepEqual(errors,[]);
        console.log('Browser tests passed: all pages, persistence, dynamic messages, GPU selection/removal/clear.');
    } finally { await browser.close(); }
})().catch(error=>{console.error(error);process.exitCode=1;});
