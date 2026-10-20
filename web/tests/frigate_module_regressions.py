"""Frigate UI lifecycle against the built Web bundle and isolated API fixtures.
No real cameras, credentials or production Docker host are contacted.
"""
import asyncio, copy, json, re
from browser_regressions import ACTOR, Harness, MANIFEST, OUT
from playwright.async_api import async_playwright, expect


def manifest():
    value = copy.deepcopy(MANIFEST)
    value.update(id='frigate.module', name='Frigate', version='0.18.2', core='>=0.1.211 <1.0.0')
    value['runtime']['docker']['image'] = 'ghcr.io/blakeblackshear/frigate@sha256:' + 'a' * 64
    value['lifecycle'].append('upgrade')
    value['ui'] = {}  # Legacy/empty navigation must still have a working sidebar entry.
    return value


def overrides():
    def reply(payload):
        async def respond(route):
            await route.fulfill(status=200, content_type='application/json', body=json.dumps(payload))
        return respond
    return {
        '/api/v1/frigate/api/version': reply({'version': '0.18.2'}),
        '/api/v1/frigate/api/config': reply({'cameras': {}}),
        '/api/v1/frigate/api/stats': reply({'cameras': {}}),
        '/api/v1/frigate/api/events': reply([]),
    }


async def main():
    OUT.mkdir(parents=True, exist_ok=True)
    async with async_playwright() as playwright:
        browser = await playwright.chromium.launch(headless=True, args=['--no-sandbox'])
        h = await Harness(browser, catalog=[manifest()], overrides=overrides()).start('/modules/catalog')
        try:
            await h.ready()
            nav = h.page.locator('#main-sidebar').get_by_role('button', name='Frigate', exact=True)
            await expect(nav).to_have_count(0)  # A catalog offer is not an installed module.
            await h.page.locator('[data-catalog-id="frigate.module"]').get_by_role('button', name=re.compile(r'^Установить(?: модуль)?$')).click()
            await expect(h.page.get_by_role('dialog')).to_contain_text('root внутри контейнера')
            await h.page.get_by_role('dialog').get_by_role('button', name='Установить', exact=True).click()
            await expect(h.page.get_by_role('progressbar')).to_be_visible()
            approved = h.posts('/install')[-1][2]
            assert approved == {'expected_version': '0.18.2', 'expected_image': manifest()['runtime']['docker']['image']}
            h.modules = [dict(manifest=manifest(), status='enabled')]
            h.jobs[0].update(status='succeeded', progress=10000)
            await expect(nav).to_have_count(1, timeout=12000)  # No page reload or manual navigation configuration.
            await nav.click()
            await expect(h.page.locator('.frigate-page')).to_be_visible()
            await expect(h.page.locator('main h1')).to_have_text('Frigate')
            await h.reload()
            await expect(nav).to_have_count(1)
            await expect(h.page.locator('.frigate-page')).to_be_visible()
            print('PASS: install approval, automatic sidebar appearance, empty navigation and reload', flush=True)

            await h.page.get_by_role('button', name='Модули и обновления', exact=True).click()
            await h.page.locator('[data-module-id="frigate.module"]').get_by_role('button', name='Остановить', exact=True).click()
            await expect(h.page.get_by_role('progressbar')).to_be_visible()
            h.modules[0]['status'] = 'disabled'
            h.jobs[0].update(status='succeeded', progress=10000)
            await expect(nav).to_have_count(1)
            await expect(nav).to_have_attribute('data-module-status', 'disabled', timeout=12000)
            await nav.click()
            await expect(h.page.get_by_text('Модуль установлен и остановлен.', exact=False)).to_be_visible(timeout=12000)
            await h.page.get_by_role('button', name='Запустить', exact=True).click()
            await expect(h.page.get_by_role('progressbar')).to_be_visible()
            h.modules[0]['status'] = 'enabled'
            h.jobs[0].update(status='succeeded', progress=10000)
            await expect(nav).to_have_attribute('data-module-status', 'enabled', timeout=12000)
            await nav.click()
            await expect(h.page.locator('.frigate-page')).to_be_visible(timeout=12000)
            print('PASS: stop retains sidebar entry; management page starts the module again', flush=True)

            await h.page.get_by_role('button', name='Выйти', exact=True).click()
            await h.page.locator('input[name=password]').fill('fixture-password-123')
            await h.page.locator('button[type=submit]').click()
            await expect(nav).to_have_count(1, timeout=12000)
            print('PASS: fresh login re-reads installed navigation', flush=True)

            # The registry refresh also preserves explicit navigation without duplicates.
            h.modules[0]['manifest']['ui'] = {'navigation': [
                dict(id='frigate', title='Frigate', route='/modules/frigate.module'),
                dict(id='duplicate', title='Duplicate Frigate', route='/modules/frigate.module'),
            ]}
            await h.page.evaluate("dispatchEvent(new CustomEvent('home-ai-core:modules-changed'))")
            await expect(nav).to_have_count(1)
            await expect(h.page.locator('#main-sidebar').get_by_role('button', name='Duplicate Frigate', exact=True)).to_have_count(0)

            # Exercise deletion through the real confirmation UI, then completed job refresh.
            await h.page.locator('#main-sidebar').get_by_role('button', name='Модули', exact=True).click()
            await h.page.locator('[data-module-id="frigate.module"] button.danger').click()
            await h.page.get_by_role('dialog').get_by_role('button', name='Удалить модуль', exact=True).click()
            await expect(h.page.get_by_role('progressbar')).to_be_visible()
            h.modules = []
            h.jobs[0].update(status='succeeded', progress=10000)
            await expect(nav).to_have_count(0, timeout=12000)
            print('PASS: removal automatically removes the sidebar entry', flush=True)
            await h.close()

            reader = copy.deepcopy(ACTOR)
            reader['permissions'] = ['security.self.read', 'modules.read']
            h = await Harness(browser, actor=reader, modules=[dict(manifest=manifest(), status='enabled')], overrides=overrides()).start('/modules/frigate.module')
            await expect(h.page.locator('main h1')).to_be_visible()
            await expect(h.page.locator('.frigate-page')).to_have_count(0)
            await expect(h.page.locator('#main-sidebar').get_by_role('button', name='Frigate', exact=True)).to_have_count(0)
            print('PASS: read-only actor cannot open the administrative Frigate UI', flush=True)
            await h.close()
        finally:
            await browser.close()


if __name__ == '__main__':
    asyncio.run(main())
