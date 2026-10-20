"""Installed-module sidebar regressions using the production Web bundle.
API fixtures only: never connects to a production host or Docker engine.
Run after npm run build: python web/tests/module_navigation_regressions.py
"""
import asyncio
import copy
import json
import os
import sys
from pathlib import Path
from urllib.parse import urlparse
from playwright.async_api import async_playwright, expect
from browser_regressions import ACTOR, Harness, MANIFEST, ORIGIN, OUT, job

HA_ID = 'homeassistant.module'
HA_PATH = '/modules/' + HA_ID
HA_MANIFEST = copy.deepcopy(MANIFEST)
HA_MANIFEST.update(id=HA_ID, name='Home Assistant', ui={}, version='2026.10.2')
HA_MANIFEST['lifecycle'].append('upgrade')
HA_MANIFEST['runtime']['docker']['image'] = 'ghcr.io/home-assistant/home-assistant@sha256:' + 'a' * 64


def installed(status='enabled'):
    return dict(manifest=copy.deepcopy(HA_MANIFEST), status=status)


class NavigationHarness(Harness):
    def __init__(self, browser, **kwargs):
        super().__init__(browser, **kwargs)
        self.catalog = [copy.deepcopy(HA_MANIFEST)]
        self.nav_calls = self.nav_active = self.nav_maximum = 0
        self.hold_next = False
        self.nav_started = asyncio.Event()
        self.release_nav = asyncio.Event()

    async def route(self, route):
        path = urlparse(route.request.url).path
        if path == '/api/v1/modules/navigation':
            self.nav_calls += 1
            self.nav_active += 1
            self.nav_maximum = max(self.nav_maximum, self.nav_active)
            # Legacy empty ui.navigation must work without updating/reinstalling a manifest.
            snapshot = [dict(module_id=m['manifest']['id'], name=m['manifest']['name'],
                             status=m['status'], items=m['manifest'].get('ui', {}).get('navigation', []))
                        for m in copy.deepcopy(self.modules)]
            permissions = self.actor['permissions']
            snapshot = [m for m in snapshot if
                        ('modules.manage' if m['module_id'] in ('ai.agent', 'frigate.module')
                         else 'modules.read') in permissions]
            try:
                if self.hold_next:
                    self.hold_next = False
                    self.nav_started.set()
                    await asyncio.wait_for(self.release_nav.wait(), timeout=15)
                await route.fulfill(status=self.nav_status, content_type='application/json',
                                    body=json.dumps(dict(modules=snapshot)))
            finally:
                self.nav_active -= 1
            return
        if path == '/api/v1/modules/catalog/' + HA_ID + '/install':
            self.requests.append((route.request.method, path, None))
            operation = job('queued', 'module.install', 'job-homeassistant-install')
            operation['input']['module_id'] = HA_ID
            self.jobs = [operation]
            await route.fulfill(status=202, content_type='application/json',
                                body=json.dumps(dict(job=operation)))
            return
        await super().route(route)

    def entry(self):
        return self.page.locator('#main-sidebar').get_by_role('button', name='Home Assistant', exact=True)

    async def changed(self):
        await self.page.evaluate("window.dispatchEvent(new CustomEvent('home-ai-core:modules-changed'))")


RESULTS = []


async def check(name, run):
    try:
        await run()
        RESULTS.append(dict(name=name, status='passed'))
        print('PASS', name, flush=True)
    except Exception as exc:
        RESULTS.append(dict(name=name, status='failed', error=str(exc)))
        print('FAIL', name, str(exc), flush=True)
        raise


async def suite(browser):
    async def pending_refresh():
        h = NavigationHarness(browser)
        h.hold_next = True
        await h.start('/account')
        try:
            await asyncio.wait_for(h.nav_started.wait(), timeout=8)
            h.modules = [installed()]
            await h.changed()
            await h.changed()  # A burst must coalesce, never race two responses.
            h.release_nav.set()
            await expect(h.entry()).to_be_visible(timeout=2000)
            assert h.nav_calls >= 2, 'in-flight stale snapshot swallowed the installation event'
            assert h.nav_maximum == 1, 'navigation requests ran concurrently'
        finally:
            h.release_nav.set()
            await h.close()
    await check('NAV-01: a change during an in-flight request is refreshed immediately', pending_refresh)

    async def catalog_install():
        h = await NavigationHarness(browser).start('/modules/catalog')
        try:
            await h.ready()
            await expect(h.entry()).to_have_count(0)
            card = h.page.locator('[data-catalog-id="homeassistant.module"]')
            await expect(card.locator('button').first).to_be_enabled()
            await card.locator('button').first.click()
            await h.page.get_by_role('button', name='Разрешить и установить', exact=True).click()
            await expect(h.page.locator('[data-job-id="job-homeassistant-install"]')).to_be_visible()
            h.modules = [installed()]
            h.jobs[0].update(status='succeeded', progress=10000)
            # No manually dispatched event: ModulesPage observes the completed job.
            await expect(h.entry()).to_be_visible(timeout=6000)
            await expect(h.entry()).to_have_count(1)
            await h.entry().click()
            await expect(h.page.get_by_role('heading', name='Home Assistant', exact=True)).to_be_visible()
            link = h.page.get_by_role('link', name='Открыть Home Assistant', exact=True)
            await expect(link).to_have_attribute('href', 'http://home-ai.test:8123/')
            await expect(link).to_have_attribute('rel', 'noopener noreferrer')
            await expect(h.page.get_by_text('routeUnavailable', exact=True)).to_have_count(0)
            await h.screenshot('module-navigation-installed-homeassistant')
        finally:
            await h.close()
    await check('NAV-02: catalog install adds a working Home Assistant page without reload', catalog_install)

    async def existing_lifecycle():
        h = await NavigationHarness(browser, modules=[installed()]).start(HA_PATH)
        try:
            await expect(h.entry()).to_be_visible()
            await expect(h.page.get_by_role('link', name='Открыть Home Assistant')).to_be_visible()
            for state in ('disabled', 'error', 'enabled'):
                h.modules[0]['status'] = state
                await h.changed()
                await expect(h.entry()).to_have_count(1)
                await expect(h.entry()).to_be_visible()
                if state != 'enabled':
                    await expect(h.page.get_by_role('button', name='Запустить', exact=True)).to_be_visible()
                    await expect(h.page.get_by_role('link', name='Открыть Home Assistant')).to_have_count(0)
                else:
                    await expect(h.page.get_by_role('link', name='Открыть Home Assistant')).to_be_visible()
            await h.reload()
            await expect(h.entry()).to_be_visible()
            await h.page.get_by_role('button', name='Выйти', exact=True).click()
            await h.page.locator('input[name="username"]').fill('owner')
            await h.page.locator('input[name="password"]').fill('fixture-only-password')
            await h.page.locator('form button[type=submit]').click()
            await expect(h.entry()).to_be_visible()
            h.modules = []
            await h.changed()
            await expect(h.entry()).to_have_count(0)
            await expect(h.page.get_by_role('link', name='Открыть Home Assistant')).to_have_count(0)
            await expect(h.page.get_by_role('button', name='Запустить', exact=True)).to_have_count(0)
        finally:
            await h.close()
    await check('NAV-03: old empty manifest survives stop/error/reload/login and disappears on removal', existing_lifecycle)

    async def access_and_recovery():
        reader = copy.deepcopy(ACTOR)
        reader['permissions'] = ['security.self.read']
        h = await NavigationHarness(browser, modules=[installed()], actor=reader).start(HA_PATH)
        try:
            await expect(h.page.locator('.app-shell')).to_be_visible()
            await expect(h.entry()).to_have_count(0)
            await expect(h.page.get_by_role('link', name='Открыть Home Assistant')).to_have_count(0)
            await expect(h.page.get_by_role('button', name='Запустить', exact=True)).to_have_count(0)
        finally:
            await h.close()
        h = await NavigationHarness(browser, modules=[installed()], nav_status=503).start(HA_PATH)
        try:
            await expect(h.page.locator('.app-shell')).to_be_visible()
            await expect(h.entry()).to_have_count(0)
            h.nav_status = 200
            await h.page.evaluate("window.dispatchEvent(new Event('online'))")
            await expect(h.entry()).to_be_visible(timeout=2000)
            # A disconnected tab catches up on focus, without a reload.
            h.modules = []
            await h.page.evaluate("window.dispatchEvent(new Event('focus'))")
            await expect(h.entry()).to_have_count(0, timeout=2000)
        finally:
            await h.close()
    await check('NAV-04: permissions remain enforced and reconnect/focus refresh registry state', access_and_recovery)


async def main():
    OUT.mkdir(parents=True, exist_ok=True)
    async with async_playwright() as playwright:
        options = dict(headless=True, args=['--no-sandbox'])
        if os.environ.get('CHROMIUM_PATH'):
            options['executable_path'] = os.environ['CHROMIUM_PATH']
        browser = await playwright.chromium.launch(**options)
        try:
            await suite(browser)
        finally:
            (OUT / 'module-navigation-results.json').write_text(json.dumps(dict(
                environment='production bundle, isolated API fixtures, no live deployment', checks=RESULTS),
                ensure_ascii=False, indent=2), encoding='utf-8')
            await browser.close()
    print(json.dumps(dict(passed=len(RESULTS))))


if __name__ == '__main__':
    asyncio.run(main())
