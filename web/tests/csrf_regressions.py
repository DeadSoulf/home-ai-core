"""Real Chromium/production bundle; only synthetic API sessions and WebSockets.
No real login credentials, server or shell is contacted.
"""
import asyncio
import json
from urllib.parse import urlparse
from playwright.async_api import async_playwright, expect
from browser_regressions import Harness, OUT, ORIGIN

class SessionHarness(Harness):
    def __init__(self, browser, **kw):
        super().__init__(browser, **kw)
        self.actor['permissions'].append('system.console')
        self.wanted='test-csrf'; self.initial_token=kw.get('initial_token','test-csrf')
        self.block_storage=kw.get('block_storage',False)
        self.ticket_delay=kw.get('ticket_delay',0)
        self.console_requests=0; self.ws_opened=0; self.commands=0

    async def start(self,path='/system#console'):
        self.context=await self.browser.new_context(viewport=dict(width=1280,height=900),locale='ru-RU')
        initial="localStorage.setItem('home-ai-core.locale','ru');"
        if self.initial_token:
            initial+="localStorage.setItem('home-ai-core.csrf',"+json.dumps(self.initial_token)+");"
        if self.block_storage:
            initial+="for(const k of ['getItem','setItem','removeItem'])Storage.prototype[k]=()=>{throw new DOMException('blocked','SecurityError')};"
        await self.context.add_init_script(initial)
        await self.context.route('**/*',self.route)
        self.page=await self.context.new_page()
        self.page.on('pageerror',lambda error:self.errors.append(str(error)))
        self.page.set_default_timeout(8000)
        await self.page.route_web_socket('**/api/v1/system/console?*',self.socket)
        await self.page.goto(ORIGIN+path)
        return self

    def socket(self,ws):
        self.ws_opened+=1
        ws.send(json.dumps(dict(type='ready',data='Fixture console ready\n')))
        def receive(message):
            self.commands+=1
            ws.send(json.dumps(dict(type='output',data='Fixture command received\n')))
        ws.on_message(receive)

    async def route(self,route):
        path=urlparse(route.request.url).path
        headers=route.request.headers
        async def reply(body,status=200):
            await route.fulfill(status=status,content_type='application/json',body=json.dumps(body))
        if path=='/api/v1/auth/me':
            if not self.auth:
                return await reply(dict(error=dict(code='authentication_required')),401)
            return await reply(dict(actor=self.actor,csrf_valid=bool(self.wanted) and headers.get('x-csrf-token')==self.wanted))
        if path=='/api/v1/auth/login':
            self.wanted='new-csrf'
        if path=='/api/v1/auth/logout' and headers.get('x-csrf-token')!=self.wanted:
            return await reply(dict(error=dict(code='csrf_required')),403)
        if path=='/api/v1/system/console/session':
            self.console_requests+=1
            if headers.get('x-csrf-token')!=self.wanted:
                return await reply(dict(error=dict(code='csrf_required')),403)
            await asyncio.sleep(self.ticket_delay)
            return await reply(dict(ticket='synthetic-fixture-ticket',expires_at='2099-01-01T00:00:00Z'),201)
        return await super().route(route)

    async def login(self):
        await expect(self.page.locator('input[name=password]')).to_be_visible()
        await self.page.locator('input[name=username]').fill('owner')
        await self.page.locator('input[name=password]').fill('synthetic-test-password')
        await self.page.locator('button[type=submit]').click()

    async def connected(self):
        await expect(self.page.locator('.server-console-output')).to_contain_text('Fixture console ready')
        await expect(self.page.locator('.server-console-input textarea')).to_be_enabled()
        assert self.ws_opened==1

RESULTS=[]
async def check(name,fn):
    try:
        await fn(); RESULTS.append(dict(name=name,status='passed')); print('PASS',name,flush=True)
    except Exception as error:
        RESULTS.append(dict(name=name,status='failed',error=str(error))); raise

async def suite(browser):
    async def migration():
        for token in ['', 'stale-other-origin-token']:
            h=await SessionHarness(browser,initial_token=token).start()
            await expect(h.page.get_by_role('alert')).to_contain_text('Сессия браузера рассинхронизирована')
            assert h.console_requests==0
            await h.page.screenshot(path=str(OUT/'csrf-reauth.png'))
            await h.login(); await h.connected()
            assert h.console_requests==1
            await h.page.locator('.server-console-input textarea').fill('fixture-command')
            await h.page.locator('.server-console-input button[type=submit]').click()
            await expect(h.page.locator('.server-console-output')).to_contain_text('Fixture command received')
            assert h.commands==1
            await h.page.screenshot(path=str(OUT/'csrf-console-connected.png'))
            await h.close()
    await check('CSRF-01: missing/stale token forces explicit login before console; verified pair connects',migration)

    async def fresh():
        for blocked in (False,True):
            h=await SessionHarness(browser,auth=False,initial_token='',block_storage=blocked).start()
            await h.login(); await h.connected()
            assert h.console_requests==1
            await h.close()
    await check('CSRF-02: fresh private session connects, including blocked browser storage',fresh)

    async def expired_during_console():
        h=await SessionHarness(browser).start('/modules'); await h.ready()
        h.wanted='other-browser-session-token'
        await h.page.evaluate("history.pushState({},'', '/system#console');dispatchEvent(new PopStateEvent('popstate'))")
        await expect(h.page.get_by_role('alert')).to_contain_text('Сессия браузера рассинхронизирована')
        await asyncio.sleep(.2)
        assert h.console_requests==1 and h.ws_opened==0
        await h.login(); await h.connected()
        assert h.console_requests==2
        await h.close()
    await check('CSRF-03: rejected ticket yields reauthentication without automatically retrying action',expired_during_console)

    async def logout():
        h=await SessionHarness(browser).start('/')
        await expect(h.page.get_by_role('button',name='Выйти',exact=True)).to_be_visible()
        h.wanted='other-session-token'
        await h.page.get_by_role('button',name='Выйти',exact=True).click()
        await expect(h.page.get_by_role('alert')).to_contain_text('не отозвана')
        assert h.auth is True
        await h.login()
        await expect(h.page.get_by_role('button',name='Выйти',exact=True)).to_be_visible()
        await h.page.get_by_role('button',name='Выйти',exact=True).click()
        await expect(h.page.locator('input[name=password]')).to_be_visible()
        assert h.auth is False
        await h.close()
    await check('CSRF-04: invalid-token logout does not claim revoke; re-login repairs it',logout)

    async def cancelled_ticket():
        h=await SessionHarness(browser,ticket_delay=1.2).start()
        for _ in range(80):
            if h.console_requests:break
            await asyncio.sleep(.05)
        assert h.console_requests==1
        await h.page.evaluate("history.pushState({},'', '/modules');dispatchEvent(new PopStateEvent('popstate'))")
        await h.ready(); await asyncio.sleep(1.5)
        assert h.ws_opened==0
        await h.close()
    await check('CSRF-05: leaving console while ticket pending never opens a background shell',cancelled_ticket)

async def main():
    OUT.mkdir(parents=True,exist_ok=True)
    async with async_playwright() as p:
        browser=await p.chromium.launch(headless=True,args=['--no-sandbox'])
        try:await suite(browser)
        finally:
            (OUT/'csrf-results.json').write_text(json.dumps(dict(environment='real Chromium; synthetic sessions and websocket; no live server',checks=RESULTS),indent=2))
            await browser.close()
    print(json.dumps(dict(csrf_groups_passed=len(RESULTS))))

if __name__=='__main__':asyncio.run(main())
