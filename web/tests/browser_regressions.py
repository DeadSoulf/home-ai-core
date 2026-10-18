"""Regression checks of the actual Vite production bundle against isolated API fixtures.
No production host, Docker engine, user data or Figma document is contacted.
Run after npm run build: python3 web/tests/browser_regressions.py
"""
import asyncio, base64, copy, json, mimetypes, os, re, sys
from pathlib import Path
from types import SimpleNamespace
from urllib.parse import urlparse
from playwright.async_api import async_playwright, expect

DIST = Path(__file__).resolve().parents[1] / 'dist'
OUT = Path(os.environ.get('UI_EVIDENCE_DIR', '/mnt/data/home-ai-ui-fixed'))
ORIGIN = 'https://home-ai.test'
PERMISSIONS = ['security.self.read','system.read','modules.read','modules.manage','jobs.read','jobs.cancel','security.users.read']
ACTOR = dict(type='user',id='usr-owner',username='owner',display_name='Home Owner',roles=['owner'],permissions=PERMISSIONS)
MANIFEST = dict(schema_version=2,id='demo.nvr',name='Камеры',description='Тестовый модуль видеонаблюдения',version='1.2.0',core='>=0.1.0 <1.0.0',runtime=dict(type='docker',docker=dict(image='example.com/camera@sha256:'+'a'*64)),capabilities=dict(requires=['host.docker']),lifecycle=['install','start','stop','restart','remove'],ui=dict(navigation=[dict(id='overview',title='Камеры',route='/modules/demo.nvr')]))
SYSTEM = dict(version='0.1.173-dev',schema_version=27,system=dict(node_id='node-test',hostname='home-test-server',os='linux',architecture='amd64',kernel='test',cpu=dict(model='Test CPU',logical_cpus=4,usage_percent=12.5),memory=dict(total_bytes=8589934592,available_bytes=4294967296),uptime_seconds=12345,block_tree=[],network_interfaces=[],gpus=[]))
RUNTIME = dict(available=True,active=True,engine_ready=True,network_ready=True,version='29.0-test',checked_at='2026-10-14T00:00:00Z')

def module(status='enabled',version='1.2.0'):
    manifest = copy.deepcopy(MANIFEST); manifest['version'] = version
    return dict(manifest=manifest,status=status)
def ai_module(status='enabled'):
    manifest=copy.deepcopy(MANIFEST)
    manifest.update(id='ai.agent', name='AI Agent')
    manifest['ui']={'navigation':[dict(id='ai.agent',title='AI Agent',route='/modules/ai.agent')]}
    return dict(manifest=manifest,status=status)

def job(status='running',typ='module.install',id='job-test'):
    return dict(id=id,node_id='node-test',type=typ,status=status,progress=1500,created_at='2026-10-14T00:00:00Z',message='pulling and creating module container',input=dict(module_id='demo.nvr'))

class Harness:
    def __init__(self, browser, **kw):
        self.browser=browser; self.actor=copy.deepcopy(ACTOR); self.modules=[]; self.catalog=[copy.deepcopy(MANIFEST)]
        self.jobs=[]; self.runtime=copy.deepcopy(RUNTIME); self.initialized=True; self.auth=True
        self.setup_status=200; self.me_status=200; self.logout_status=204; self.login_status=200
        self.system_delay=0; self.system_status=200; self.catalog_status=200; self.runtime_status=200; self.nav_status=200
        self.requests=[]; self.errors=[]; self.system_active=0; self.system_max=0; self.calls=0
        self.ai_providers=[]; self.ai_conversations=[]; self.ai_approvals=[]; self.ai_messages={}
        self.ai_version='0.1.0';self.ai_gpu_package_ready=False
        self.ai_gpus=[]; self.ai_assigned=[]; self.ai_installed=['qwen3:0.6b']; self.ai_download_status='ready'
        self.overrides={}; self.unhandled=[]; self.offline=os.environ.get('UI_OFFLINE')=='1'
        for k,v in kw.items(): setattr(self,k,v)
    async def start(self,path='/modules',width=1440):
        self.context=await self.browser.new_context(viewport=dict(width=width,height=1000 if width>768 else 844),locale='ru-RU')
        await self.context.add_init_script("localStorage.setItem('home-ai-core.locale','ru'); localStorage.setItem('home-ai-core.csrf','test-csrf');")
        await self.context.route('**/*',self.route)
        self.page=await self.context.new_page(); self.page.on('pageerror',lambda error:self.errors.append(str(error)))
        self.page.set_default_timeout(8000)
        if self.offline: await self.render_offline(path)
        else: await self.page.goto(ORIGIN+path)
        return self
    async def render_offline(self,path):
        # A restricted local Chromium may forbid all navigation. Render the actual
        # bundle without network in that environment; CI still exercises real URLs.
        await self.page.set_content('<!doctype html><html lang="ru"><head></head><body><div id="root"></div></body></html>')
        async def api_binding(source, payload):
            class Route:
                request=SimpleNamespace(url=ORIGIN+payload['path'],method=payload['method'],post_data=payload.get('body'))
                request.post_data_json=json.loads(payload['body']) if payload.get('body') else None
                result=None
                async def fulfill(self,**kwargs):self.result=kwargs
                async def abort(self):self.result=dict(status=500,body='')
            route=Route(); await self.route(route); return route.result
        if not getattr(self,'bound',False):
            await self.page.expose_binding('__fixture',api_binding);self.bound=True
        await self.page.evaluate("""(start)=>{
          const storage={};const local={getItem:k=>storage[k]??null,setItem:(k,v)=>storage[k]=String(v),removeItem:k=>delete storage[k]};
          Object.defineProperty(window,'localStorage',{configurable:true,value:local});
          local.setItem('home-ai-core.locale','ru');local.setItem('home-ai-core.csrf','test-csrf');
          window.__location=new URL(start);window.__history=[start];window.__index=0;
          history.pushState=(_,__,url)=>{window.__location=new URL(url,window.__location);window.__history.splice(++window.__index);window.__history.push(window.__location.href)};
          history.replaceState=(_,__,url)=>{window.__location=new URL(url,window.__location);window.__history[window.__index]=window.__location.href};
          window.__back=()=>{window.__location=new URL(window.__history[--window.__index]);dispatchEvent(new PopStateEvent('popstate'))};
          window.__forward=()=>{window.__location=new URL(window.__history[++window.__index]);dispatchEvent(new PopStateEvent('popstate'))};
          window.fetch=async (path,init={})=>{const pending=window.__fixture({path:String(path),method:init.method||'GET',body:init.body});
            const result=await pending;if(init.signal?.aborted)throw new DOMException('Aborted','AbortError');
            return new Response(result.status===204?null:result.body,{status:result.status,headers:{'Content-Type':result.content_type||'application/json'}})};
        }""",ORIGIN+path)
        for css in DIST.glob('assets/*.css'):await self.page.add_style_tag(content=css.read_text())
        script=next(DIST.glob('assets/index-*.js')).read_text().replace('window.location','window.__location')
        await self.page.add_script_tag(content=script,type='module')
    async def url(self):return await self.page.evaluate('window.__location.href') if self.offline else self.page.url
    async def reload(self):
        if self.offline:
            path=urlparse(await self.url());await self.render_offline(path.path+('#'+path.fragment if path.fragment else ''))
        else:await self.page.reload()
    async def back(self):
        if self.offline:await self.page.evaluate('window.__back()')
        else:await self.page.go_back()
    async def forward(self):
        if self.offline:await self.page.evaluate('window.__forward()')
        else:await self.page.go_forward()
    async def route(self,route):
        request=route.request; path=urlparse(request.url).path; method=request.method
        if method not in ('GET','HEAD'):
            try: data=request.post_data_json
            except Exception: data=request.post_data
            self.requests.append((method,path,data))
        async def respond(body=None,status=200):
            if status==204: await route.fulfill(status=204,body=''); return
            await route.fulfill(status=status,content_type='application/json',body=json.dumps(body if body is not None else {'error':dict(code='request_failed',message='fixture failure')},ensure_ascii=False))
        if path in self.overrides:
            await self.overrides[path](route); return
        if path.startswith('/api/'):
            if path=='/api/v1/security/setup-status': await respond(dict(initialized=self.initialized),self.setup_status)
            elif path=='/api/v1/auth/me': await respond(dict(actor=self.actor,csrf_valid=True),self.me_status if self.auth else 401)
            elif path=='/api/v1/auth/logout':
                if self.logout_status==204:self.auth=False
                await respond(status=self.logout_status)
            elif path=='/api/v1/auth/login':
                if self.login_status==200:self.auth=True
                await respond(dict(actor=self.actor,csrf_token='new-csrf') if self.login_status==200 else dict(error=dict(code='invalid_credentials',message='invalid username or password',request_id='request-test')), self.login_status)
            elif path=='/api/v1/security/bootstrap': self.initialized=True; self.auth=True; await respond(dict(actor=self.actor,csrf_token='new-csrf'),201)
            elif path=='/api/v1/system':
                self.system_active+=1; self.system_max=max(self.system_max,self.system_active); self.calls+=1
                try: await asyncio.sleep(self.system_delay); await respond(SYSTEM,self.system_status)
                finally:self.system_active-=1
            elif path.startswith('/api/v1/ai/'):
                sub=path[len('/api/v1/ai/'):]
                presets=[
                    dict(name='qwen3:0.6b',size_gb=0.6,minimum_ram_gb=2,purpose='Базовый агент, CPU',recommended=True),
                    dict(name='qwen3:1.7b',size_gb=1.5,minimum_ram_gb=4,purpose='Улучшенный чат',recommended=True),
                    dict(name='qwen3:4b',size_gb=2.8,minimum_ram_gb=8,purpose='Сложные задачи',recommended=False),
                ]
                if sub=='hardware' and method=='GET':
                    await respond(dict(
                        checked_at='2026-10-17T22:00:00Z',policy='cpu_first_gpu_requires_explicit_approval',
                        cpu=dict(model='Test CPU',logical_cpus=8,usage_percent=18),
                        memory=dict(total_bytes=17179869184,available_bytes=8589934592),
                        gpus=self.ai_gpus,
                    ))
                elif sub=='gpu/runtime' and method=='GET':
                    await respond(dict(ready=self.ai_gpu_package_ready))
                elif sub=='gpu/runtime/prepare' and method=='POST':
                    self.jobs=[job('running','module.gpu.prepare','job-optional-gpu')]
                    self.ai_gpu_package_ready=True
                    await respond(dict(job=self.jobs[0]),202)
                elif sub=='gpu/assignment' and method=='GET':
                    await respond(dict(assigned_gpu_pci=self.ai_assigned,mode='nvidia' if self.ai_assigned else 'cpu'))
                elif sub=='gpu/assignment' and method=='POST':
                    self.ai_assigned=request.post_data_json['pci_addresses']
                    await respond(dict(assigned_gpu_pci=self.ai_assigned,mode='nvidia' if self.ai_assigned else 'cpu'))
                elif sub=='local/status' and method=='GET':
                    await respond(dict(enabled=True,engine_state='ready',model_state=self.ai_download_status,
                                       active_model='qwen3:0.6b',cpu_first=True,ram_bytes=17179869184,
                                       recommended_models=presets))
                elif sub=='local/models' and method=='GET':
                    await respond(dict(enabled=True,installed=self.ai_installed,recommended_models=presets))
                elif sub=='local/pull' and method=='POST':
                    name=request.post_data_json['name']
                    if name not in self.ai_installed:self.ai_installed.append(name)
                    await respond(dict(queued=True,name=name),202)
                elif sub=='status' and method=='GET':
                    await respond(dict(version=self.ai_version,providers=len(self.ai_providers),
                                       approvals_pending=len([x for x in self.ai_approvals if x['status']=='pending']),
                                       core_actions_enabled=False))
                elif sub=='providers' and method=='GET':await respond(dict(providers=self.ai_providers))
                elif sub=='providers' and method=='POST':
                    p=request.post_data_json.copy();p['id']=format(len(self.ai_providers)+1,'032x');p.pop('api_key',None);self.ai_providers.append(p)
                    await respond(dict(id=p['id']),201)
                elif sub=='conversations' and method=='GET':await respond(dict(conversations=self.ai_conversations))
                elif sub=='approvals' and method=='GET':await respond(dict(approvals=self.ai_approvals))
                elif sub=='chat' and method=='POST':
                    cid=(request.post_data_json or {}).get('conversation_id') or 'b'*32
                    if not any(c['id']==cid for c in self.ai_conversations):
                        self.ai_conversations.append(dict(id=cid,title='Тестовый диалог',updated_at=1,created_at=1))
                    self.ai_messages.setdefault(cid,[]).extend([
                        dict(role='user',content=request.post_data_json['message']),
                        dict(role='assistant',content='Ответ локального агента'),
                    ])
                    await respond(dict(conversation_id=cid,reply='Ответ локального агента',approval_ids=[]))
                elif sub.endswith('/messages') and method=='GET':
                    cid=sub.split('/')[1];await respond(dict(messages=self.ai_messages.get(cid,[])))
                elif sub.startswith('approvals/') and sub.endswith('/decision') and method=='POST':
                    target=sub.split('/')[1]
                    for item in self.ai_approvals:
                        if item['id']==target:item['status']='approved_not_executed' if request.post_data_json['decision']=='approve' else 'rejected'
                    await respond(dict(status='approved_not_executed',executed=False))
                elif sub.startswith('providers/') and method=='DELETE':
                    self.ai_providers=[p for p in self.ai_providers if p['id']!=sub.split('/')[1]]
                    await respond(dict(deleted=True))
                else:self.unhandled.append((method,path));await respond(None,404)
            elif path=='/api/v1/modules': await respond(dict(modules=self.modules))
            elif path=='/api/v1/modules/navigation': await respond(dict(modules=[dict(module_id=m['manifest']['id'],status=m['status'],items=m['manifest'].get('ui',{}).get('navigation',[])) for m in self.modules]),self.nav_status)
            elif path=='/api/v1/modules/capabilities': await respond(dict(capabilities=['host.linux']+(['host.docker'] if self.runtime.get('available') else [])))
            elif path=='/api/v1/modules/runtime': await respond(dict(runtime=self.runtime),self.runtime_status)
            elif path=='/api/v1/modules/catalog': await respond(dict(catalog=dict(id='official-test',modules=self.catalog)),self.catalog_status)
            elif path=='/api/v1/modules/operations':
                active=[j for j in self.jobs if j['status'] in ('queued','running','cancel_requested') and j['type'].startswith('module.')]
                terminal=[j for j in reversed(self.jobs) if j['status'] in ('succeeded','failed','cancelled') and j['type'].startswith('module.')][:10]
                await respond(dict(jobs=active+terminal))
            elif path.startswith('/api/v1/modules/operations/') or (path.startswith('/api/v1/jobs/') and not path.endswith('/cancel')):
                id=path.rsplit('/',1)[-1]; found=next((j for j in self.jobs if j['id']==id),None)
                await respond(dict(job=found) if found else None,200 if found else 404)
            elif path=='/api/v1/jobs' and method=='GET':await respond(dict(jobs=self.jobs))
            elif path.endswith('/cancel'):
                for j in self.jobs:
                    if j['id']==path.split('/')[-2]:j['status']='cancel_requested'
                await respond(dict(job=self.jobs[0]))
            elif path=='/api/v1/jobs' and method=='DELETE': self.jobs=[j for j in self.jobs if j['status'] in ('running','queued','cancel_requested')]; await respond(dict(deleted=1))
            elif path.startswith('/api/v1/modules/') and method in ('POST','DELETE'):
                typ='module.remove' if method=='DELETE' else 'module.control' if path.endswith('/control') else 'module.install'
                self.jobs=[job('queued',typ)]; await respond(dict(job=self.jobs[0]),202)
            elif path=='/api/v1/security/users': await respond(dict(users=[dict(id='usr-owner',username='owner',display_name='Home Owner',roles=['owner'],permissions=PERMISSIONS,profile='owner',created_at='2026-10-14T00:00:00Z')]))
            else:self.unhandled.append((method,path)); await respond(None,404)
            return
        target=(DIST/path.lstrip('/')).resolve()
        if not target.is_relative_to(DIST.resolve()):await route.abort(); return
        if not target.is_file():target=DIST/'index.html'
        await route.fulfill(status=200,body=target.read_bytes(),content_type=mimetypes.guess_type(target.name)[0] or 'application/octet-stream')
    def posts(self, suffix):return [req for req in self.requests if req[1].endswith(suffix)]
    async def ready(self): await expect(self.page.locator('.modules-page')).to_be_visible()
    async def close(self):
        assert not self.errors, self.errors
        await self.context.close()
    async def screenshot(self,name):
        if self.offline:
            images={"/"+str(f.relative_to(DIST)):"data:"+(mimetypes.guess_type(f.name)[0] or 'application/octet-stream')+";base64,"+base64.b64encode(f.read_bytes()).decode() for f in (DIST/'brand').rglob('*') if f.is_file()}
            await self.page.evaluate("(images)=>{for(const el of document.querySelectorAll('img')){const src=el.getAttribute('src');if(images[src])el.src=images[src]}}",images)
            await self.page.wait_for_function("Array.from(document.images).every(img=>img.complete)")
        await self.page.screenshot(path=str(OUT/(name+'.png')),full_page=True)

RESULTS=[]
async def check(name,fn):
    try:await fn(); RESULTS.append(dict(name=name,status='passed')); print('PASS',name,flush=True)
    except Exception as error:RESULTS.append(dict(name=name,status='failed',error=str(error))); print('FAIL',name,str(error),flush=True); raise

async def suite(browser):
    async def unavailable():
        h=await Harness(browser,setup_status=503).start('/')
        await expect(h.page.get_by_role('heading',name='Сервер недоступен')).to_be_visible()
        assert await h.page.locator('input[name=password]').count()==0
        h.setup_status=200; await h.page.get_by_role('button',name='Повторить',exact=True).click()
        await expect(h.page.get_by_role('heading',name='Главная',exact=True)).to_be_visible()
        await h.close()
        h=await Harness(browser,me_status=503).start('/')
        await expect(h.page.get_by_role('heading',name='Сервер недоступен')).to_be_visible(); await h.close()
    await check('UI-10: setup and session 503 are unavailable, retry recovers',unavailable)

    async def logout():
        h=await Harness(browser,logout_status=503).start('/')
        await expect(h.page.get_by_role('heading',name='Главная',exact=True)).to_be_visible()
        await h.page.get_by_role('button',name='Выйти',exact=True).click()
        await expect(h.page.get_by_role('alert')).to_contain_text('Выход не подтверждён')
        assert await h.page.locator('input[name=password]').count()==0
        assert await h.page.evaluate("localStorage.getItem('home-ai-core.csrf')")=='test-csrf'
        h.logout_status=204; await h.page.get_by_role('button',name='Выйти',exact=True).click()
        await expect(h.page.locator('input[name=password]')).to_be_visible(); assert not await h.page.evaluate("localStorage.getItem('home-ai-core.csrf')")
        await h.close()
    await check('UI-06: failed logout preserves UI and CSRF; retry confirms revoke',logout)

    async def login():
        h=await Harness(browser,auth=False,login_status=401).start('/')
        await expect(h.page.locator('input[name=password]')).to_be_visible()
        await h.page.locator('button[type=submit]').click()
        assert not h.posts('/login'); await expect(h.page.locator('input[name=password]')).to_have_attribute('aria-invalid','true')
        await h.page.locator('input[name=password]').fill('valid-password')
        await h.page.locator('button[type=submit]').click()
        await expect(h.page.get_by_role('alert')).to_contain_text('Неверное имя пользователя или пароль')
        await expect(h.page.locator('input[name=password]')).to_have_value('valid-password')
        assert 'request-test' in await h.page.locator('body').text_content()
        await h.screenshot('01-login-localized-error'); await h.close()
    await check('UI-11/12/13: login validation, localized errors, accessible fields',login)

    async def setup():
        h=await Harness(browser,initialized=False).start('/',390)
        await expect(h.page.locator('input[name=bootstrapToken]')).to_be_visible()
        await h.page.locator('input[name=password]').fill('secure-password-123')
        await h.page.locator('input[name=confirmation]').fill('does-not-match')
        await h.page.locator('button[type=submit]').click()
        assert not h.posts('/bootstrap'); await expect(h.page.locator('input[name=bootstrapToken]')).to_be_focused()
        await h.page.locator('input[name=bootstrapToken]').fill('test-out-of-band-code')
        await h.page.locator('button[type=submit]').click()
        await expect(h.page.locator('input[name=confirmation]')).to_have_attribute('aria-invalid','true')
        await h.page.get_by_role('button',name='Показать пароль',exact=True).click()
        await expect(h.page.locator('input[name=password]')).to_have_attribute('type','text')
        await h.screenshot('02-setup-mobile'); await h.close()
    await check('UI-12/13: owner code, password confirmation and show password',setup)

    async def slow():
        h=await Harness(browser,system_delay=2.6).start('/system')
        await expect(h.page.get_by_text('home-test-server',exact=True)).to_be_visible(timeout=7000)
        await asyncio.sleep(5.0)
        assert h.calls>=2 and h.system_max==1,(h.calls,h.system_max)
        h.system_status=503
        await expect(h.page.get_by_role('alert')).to_be_visible(timeout=7000)
        await expect(h.page.get_by_text('home-test-server',exact=True)).to_be_visible()
        await h.screenshot('03-system-slow-retains-data'); await h.close()
    await check('UI-01: 2.6s System responses render, no overlap, stale data retained',slow)

    async def no_docker():
        runtime=copy.deepcopy(RUNTIME); runtime.update(available=False,active=False,engine_ready=False,network_ready=False)
        h=await Harness(browser,runtime=runtime).start('/modules/catalog'); await h.ready()
        button=h.page.locator('[data-catalog-id=\"demo.nvr\"] button')
        await expect(button).to_be_enabled(); await expect(button).to_contain_text('Подготовить Docker')
        await button.click()
        await expect(h.page.get_by_role('progressbar')).to_be_visible()
        assert (await h.url()).endswith('/modules/operations')
        await expect(h.page.get_by_role('button',name='Открыть операцию')).to_be_visible()
        assert len(h.posts('/install'))==1
        h.jobs[0].update(status='succeeded',progress=10000); h.modules=[module()]
        await h.page.get_by_role('button',name='Установленные модули',exact=True).click()
        await expect(h.page.locator('[data-module-id="demo.nvr"]')).to_be_visible(timeout=6500)
        await h.page.get_by_role('button',name='Операции модулей',exact=True).click()
        await h.page.get_by_role('button',name='Открыть операцию').click()
        await expect(h.page.locator('#job-job-test')).to_be_focused(); assert (await h.url()).endswith('/jobs#job=job-test')
        await h.screenshot('04-completed-job-linked'); await h.close()
    await check('UI-02/07: install prepares Docker, tracks job without websocket, prevents double submit',no_docker)

    async def versions():
        for candidate,label in [('1.1.0','Установлена более новая версия'),('1.3.0','безопасное обновление модулей пока не реализовано')]:
            manifest=copy.deepcopy(MANIFEST);manifest['version']=candidate
            h=await Harness(browser,modules=[module()],catalog=[manifest]).start('/modules/catalog'); await h.ready()
            card=h.page.locator('[data-catalog-id=\"demo.nvr\"]')
            await expect(card).to_contain_text(label); await expect(card.locator('button')).to_be_disabled()
            assert not h.requests
            await h.screenshot('05-catalog-'+candidate);await h.close()
        manifest=copy.deepcopy(MANIFEST);manifest['runtime']['docker']['image']='example.com/camera@sha256:'+'b'*64
        h=await Harness(browser,modules=[module()],catalog=[manifest]).start('/modules/catalog');await h.ready()
        await expect(h.page.locator('[data-catalog-id=\"demo.nvr\"]')).to_contain_text('Манифест каталога отличается')
        await expect(h.page.locator('[data-catalog-id=\"demo.nvr\"] button')).to_be_disabled();await h.close()
    await check('UI-03: older/newer/same-version different digest do not offer unsafe update',versions)

    async def retry():
        manifest=copy.deepcopy(MANIFEST);manifest['version']='9.0.0';manifest['runtime']['docker']['image']='example.com/camera@sha256:'+'b'*64
        h=await Harness(browser,modules=[module('registered')],catalog=[manifest]).start('/modules');await h.ready()
        card=h.page.locator('[data-module-id=\"demo.nvr\"]')
        await expect(card.get_by_role('button',name='Повторить установку')).to_be_enabled()
        assert await card.get_by_role('button',name='Остановить',exact=True).count()==0
        await card.get_by_role('button',name='Повторить установку').click()
        assert h.posts('/install')[0][2]==MANIFEST,h.posts('/install')
        await h.close()
    await check('UI-04: registered retry uses exact stored manifest, not newer catalog',retry)

    async def routes():
        h=await Harness(browser,modules=[module()]).start('/modules');await h.ready()
        await h.page.get_by_role('button',name='Камеры',exact=True).click()
        await expect(h.page.locator('main h1')).to_have_text('Камеры')
        await expect(h.page.get_by_text('Интерфейс модуля ещё не подключён',exact=True)).to_be_visible()
        await h.back();await expect(h.page.locator('.modules-page')).to_be_visible()
        await h.forward();await expect(h.page.locator('main h1')).to_have_text('Камеры')
        await h.screenshot('06-module-route-honest');await h.close()
        h=await Harness(browser,modules=[module()]).start('/modules/demo.nvr');await expect(h.page.locator('main h1')).to_have_text('Камеры');await h.close()
    await check('UI-05: module menu, direct reload and browser history show correct route',routes)

    async def remove():
        h=await Harness(browser,modules=[module()]).start('/modules');await h.ready()
        button=h.page.locator('[data-module-id=\"demo.nvr\"] button.danger');await expect(button).to_be_enabled();await button.click()
        dialog=h.page.get_by_role('dialog');await expect(dialog).to_be_visible();await expect(dialog).to_contain_text('Постоянные данные сохранятся')
        assert not h.posts('/demo.nvr');await h.page.keyboard.press('Escape');await expect(dialog).to_have_count(0);await expect(button).to_be_focused()
        await button.click();await h.page.get_by_role('dialog').get_by_role('button',name='Удалить модуль',exact=True).click()
        assert len([r for r in h.requests if r[0]=='DELETE'])==1
        await h.close()
    await check('UI-08/13: remove confirmation, safe Escape, focus restoration, one DELETE',remove)

    async def cancel():
        h=await Harness(browser,jobs=[job()]).start('/jobs#job=job-test')
        await expect(h.page.locator('#job-job-test')).to_be_focused()
        await expect(h.page.get_by_text('Установка модуля',exact=True)).to_be_visible()
        await h.page.get_by_role('button',name='Отменить операцию',exact=True).click()
        await expect(h.page.get_by_role('dialog')).to_contain_text('не откатывает')
        await h.page.get_by_role('dialog').get_by_role('button',name='Отменить операцию').click()
        await expect(h.page.get_by_role('button',name='Отменить операцию',exact=True)).to_have_count(0)
        assert len(h.posts('/cancel'))==1
        await h.screenshot('07-cancel-requested');await h.close()
    await check('UI-09/11: specific job name, progress, cancellation and correct pending state',cancel)

    async def styles_permissions():
        h=await Harness(browser,modules=[module()]).start('/modules/settings');await h.ready()
        await h.page.get_by_text('Расширенные настройки: установка Manifest вручную',exact=True).click()
        textarea=h.page.get_by_role('textbox',name='JSON-манифест модуля',exact=True)
        await expect(textarea).to_be_visible();await textarea.fill('not json')
        await h.page.locator('.developer-install').get_by_role('button',name='Установить модуль',exact=True).click()
        await expect(textarea).to_have_attribute('aria-invalid','true');await expect(textarea).to_be_focused()
        assert await h.page.locator('#manifest-error[role=alert]').count()==1
        for width in [320,360,390,768,1440]:
            await h.page.set_viewport_size(dict(width=width,height=1000 if width>768 else 844));await asyncio.sleep(.1)
            dims=await textarea.evaluate('(el)=>({width:el.offsetWidth,parent:el.parentElement.clientWidth,bg:getComputedStyle(el).backgroundColor,doc:document.documentElement.scrollWidth,win:innerWidth})')
            assert dims['width']>=dims['parent']*.85,dims
            assert dims['doc']<=width+1,dims
            assert dims['bg']!='rgb(255, 255, 255)',dims
            assert await h.page.locator('.button-row').first.evaluate('(el)=>parseFloat(getComputedStyle(el).gap)')>=8
            if width in [390,1440]:await h.screenshot('08-modules-'+str(width))
        await h.close()
        actor=copy.deepcopy(ACTOR);actor['permissions']=['security.self.read','modules.read']
        h=await Harness(browser,actor=actor,modules=[module()]).start('/modules');await h.ready()
        assert await h.page.locator('button.danger').count()==0
        assert await h.page.locator('.developer-install').count()==0
        assert not h.requests;await h.close()
        actor=copy.deepcopy(ACTOR);actor['permissions']=['security.self.read','modules.read','modules.manage']
        h=await Harness(browser,actor=actor).start('/modules/catalog');await h.ready()
        await expect(h.page.locator('[data-catalog-id] button')).to_be_enabled();await h.page.locator('[data-catalog-id] button').click()
        await expect(h.page.get_by_role('progressbar')).to_be_visible();assert await h.page.get_by_role('button',name='Открыть операцию').count()==0
        assert not h.unhandled,h.unhandled;await h.close()
    await check('UI-13/15/16: labeled themed full-width JSON, button gaps, 5 widths and least privilege',styles_permissions)

    async def runtime():
        runtime=copy.deepcopy(RUNTIME);runtime.update(engine_ready=False,network_ready=False,version='',error='socket exists but engine is down')
        h=await Harness(browser,runtime=runtime,catalog_status=503,modules=[module()]).start('/modules/settings')
        await expect(h.page.get_by_text('Движок Docker не отвечает.',exact=True)).to_be_visible()
        await h.page.get_by_role('button',name='Установленные модули',exact=True).click()
        await expect(h.page.locator('[data-module-id]')).to_be_visible()
        await expect(h.page.locator('[data-module-id] button.danger')).to_be_enabled()
        assert await h.page.get_by_text('Движок Docker и сеть модулей доступны.',exact=True).count()==0
        await h.screenshot('09-runtime-catalog-degraded');await h.close()
    await check('UI-14: host.docker is not readiness; catalog failure retains installed controls',runtime)

    async def degraded_monitor():
        h=await Harness(browser,jobs=[job()]).start('/modules/catalog');await h.ready()
        await expect(h.page.locator('[data-catalog-id] button')).to_be_disabled()
        await h.page.get_by_role('button',name='Операции модулей',exact=True).click()
        await expect(h.page.get_by_role('progressbar')).to_be_visible()
        await h.page.get_by_role('button',name='Каталог модулей',exact=True).click()
        await expect(h.page.locator('[data-catalog-id] button')).to_be_disabled()
        h.jobs=[]
        await expect(h.page.locator('[data-catalog-id] button')).to_be_enabled(timeout=6000)
        async def offline_status(route):await route.fulfill(status=503,content_type='application/json',body='{"error":{"code":"jobs_unavailable"}}')
        h.overrides['/api/v1/modules/operations']=offline_status
        await expect(h.page.get_by_role('alert')).to_contain_text('Не удалось обновить статус операции',timeout=6000)
        await expect(h.page.locator('[data-catalog-id] button')).to_be_disabled()
        await h.close()
    await check('UI-07: page reopen restores pending jobs; monitor failure fails closed',degraded_monitor)

    async def menu_and_routes():
        h=await Harness(browser,modules=[module()]).start('/modules',390);await h.ready()
        await h.page.get_by_role('button',name='Меню',exact=True).click()
        await expect(h.page.locator('main')).to_have_attribute('inert','')
        await expect(h.page.locator('#main-sidebar nav button[aria-current=page]')).to_be_focused()
        await h.page.keyboard.press('Escape')
        assert await h.page.locator('main[inert]').count()==0
        await expect(h.page.get_by_role('button',name='Меню',exact=True)).to_be_focused();await h.close()
        h=await Harness(browser,modules=[module()],nav_status=503).start('/modules/demo.nvr')
        await expect(h.page.get_by_role('alert')).to_be_visible()
        h.nav_status=200;await h.page.get_by_role('button',name='Повторить',exact=True).click()
        await expect(h.page.locator('main h1')).to_have_text('Камеры');await h.close()
    await check('UI-05/13: preserved mobile focus/Escape and module-navigation retry',menu_and_routes)

    async def module_sections():
        h=await Harness(browser,modules=[module()]).start('/modules/catalog');await h.ready()
        assert (await h.url()).endswith('/modules/catalog')
        await expect(h.page.locator('[data-catalog-id]')).to_have_count(1)
        assert await h.page.locator('[data-module-id]').count()==0
        await h.page.get_by_role('button',name='Установленные модули',exact=True).click()
        assert (await h.url()).endswith('/modules/installed')
        await expect(h.page.locator('[data-module-id]')).to_have_count(1)
        assert await h.page.locator('[data-catalog-id]').count()==0
        await h.page.get_by_role('button',name='Операции модулей',exact=True).click()
        assert (await h.url()).endswith('/modules/operations')
        await expect(h.page.get_by_text('Активных операций с модулями нет.',exact=False)).to_be_visible()
        assert await h.page.locator('[data-catalog-id], [data-module-id]').count()==0
        await h.page.get_by_role('button',name='Настройки модулей',exact=True).click()
        assert (await h.url()).endswith('/modules/settings')
        await expect(h.page.get_by_text('Движок Docker и сеть модулей доступны.',exact=True)).to_be_visible()
        await h.back()
        assert (await h.url()).endswith('/modules/operations')
        await h.close()
    await check('UI-17: separate module pages, direct routes, isolated content and history',module_sections)

    async def native_ai_agent():
        h=await Harness(browser,modules=[ai_module()]).start('/modules/ai.agent')
        await expect(h.page.locator('main h1')).to_have_text('AI Agent')
        await expect(h.page.get_by_text('Интерфейс модуля ещё не подключён')).to_have_count(0)
        await expect(h.page.get_by_text('Провайдер не подключён.',exact=False)).to_be_visible()
        await h.page.get_by_role('button',name='AI-провайдеры',exact=True).click()
        section=h.page.locator('section[aria-label="Настройки AI-провайдеров"]')
        # Regression: formerly the shared .text-input min-height:12rem made every
        # one-line input/select ~192 CSS pixels tall, obscuring the entire form.
        for width in [320, 390, 768, 1440]:
            await h.page.set_viewport_size(dict(width=width,height=900))
            metrics=await section.locator('.ai-provider-form .text-input').evaluate_all("""els => els.map(el => ({
                tag:el.tagName, height:el.getBoundingClientRect().height,
                minHeight:getComputedStyle(el).minHeight, resize:getComputedStyle(el).resize
            }))""")
            assert len(metrics)==5, metrics
            assert all(40 <= item['height'] <= 52 for item in metrics), (width,metrics)
            assert all(item['resize']=='none' for item in metrics), (width,metrics)
            assert await h.page.evaluate('document.documentElement.scrollWidth <= innerWidth + 1'), width
        await section.get_by_role('textbox',name='Название').fill('Локальная модель')
        await section.get_by_role('textbox',name='URL API').fill('http://192.168.1.20:11434')
        await section.get_by_role('textbox',name='Модель').fill('qwen3:8b')
        await section.get_by_role('button',name='Сохранить провайдера').click()
        chat=h.page.locator('section[aria-label="Чат с AI"]')
        for el in await chat.locator('.ai-controls select.text-input').all():
            assert 40 <= await el.evaluate('(el)=>el.getBoundingClientRect().height') <= 52
        compose_height=await chat.locator('textarea.text-input').evaluate('(el)=>el.getBoundingClientRect().height')
        assert 80 <= compose_height <= 220, compose_height
        await chat.get_by_role('textbox',name='Сообщение агенту').fill('Как дела?')
        await h.page.get_by_role('button',name='Отправить',exact=True).click()
        await expect(h.page.get_by_text('Ответ локального агента')).to_be_visible()
        assert any(req[1]=='/api/v1/ai/chat' for req in h.requests)
        await h.page.get_by_role('button',name='Закрыть чат',exact=True).click()
        await expect(h.page.get_by_text('Встроенный Ollama',exact=True)).to_be_visible()
        await expect(h.page.get_by_role('button',name='Закрыть чат',exact=True)).to_have_count(0)
        await h.page.get_by_role('button',name='Чат',exact=True).click()
        await expect(h.page.get_by_text('Ответ локального агента')).to_be_visible()
        await h.close()

        reader=copy.deepcopy(ACTOR);reader['permissions']=['security.self.read','modules.read']
        h=await Harness(browser,actor=reader,modules=[ai_module()]).start('/modules')
        await h.ready()
        assert await h.page.get_by_role('button',name='AI Agent',exact=True).count()==0
        await h.close()
    await check('UI-18: native AI chat, provider setup and manager-only navigation',native_ai_agent)

    async def ai_timeout_shows_model_error_not_server_outage():
        provider=dict(id='a'*32,name='Встроенный Ollama',model='qwen3:8b',kind='ollama',
                      base_url='http://127.0.0.1:11434',has_api_key=False)
        async def chat_failure(route):
            await route.fulfill(status=502,content_type='application/json',
                body=json.dumps(dict(error=dict(code='ai_upstream_error',
                                                message='AI model could not generate a reply'))))
        h=await Harness(browser,modules=[ai_module()],
                        ai_providers=[provider],overrides={'/api/v1/ai/chat':chat_failure}).start('/modules/ai.agent')
        await h.page.get_by_role('textbox',name='Сообщение агенту').fill('Привет')
        await h.page.get_by_role('button',name='Отправить',exact=True).click()
        await expect(h.page.get_by_role('alert')).to_contain_text('Модель Ollama не смогла сформировать ответ')
        await expect(h.page.get_by_text('Сервер недоступен',exact=True)).to_have_count(0)
        await h.close()
    await check('UI-24: AI model error is not misreported as server outage',ai_timeout_shows_model_error_not_server_outage)

    async def ai_multiple_installed_models():
        local_url='http://127.0.0.1:11434'
        providers=[
            dict(id='a'*32,name='Встроенный Ollama',kind='ollama',model='qwen3:0.6b',
                 base_url=local_url,has_api_key=False),
        ]
        models=['qwen3:0.6b','qwen3:8b','deepseek-r1:7b']
        h=await Harness(browser,modules=[ai_module()],ai_version='0.3.2',
                        ai_installed=models,ai_providers=providers).start('/modules/ai.agent')
        await expect(h.page.locator('.ai-status')).to_contain_text('Установлено локальных моделей: 3')
        await expect(h.page.locator('.ai-status')).to_contain_text('Подключено AI-провайдеров: 1')
        await h.page.get_by_role('button',name='Локальный AI',exact=True).click()
        await expect(h.page.get_by_text('Установлено:',exact=False)).to_contain_text('3')
        item=h.page.locator('.ai-model-entry').filter(has_text='deepseek-r1:7b')
        await expect(item).to_be_visible()
        await item.get_by_role('button',name='Выбрать для чата').click()
        await expect(h.page.locator('section[aria-label="Чат с AI"] select').first).to_have_value(format(2,'032x'))
        assert any(req[1]=='/api/v1/ai/providers' and req[0]=='POST' and req[2].get('model')=='deepseek-r1:7b' for req in h.requests)
        await expect(h.page.locator('.ai-status')).to_contain_text('Установлено локальных моделей: 3')
        await expect(h.page.locator('.ai-status')).to_contain_text('Подключено AI-провайдеров: 2')
        await h.close()
    await check('UI-25: count real Ollama installations and select discovered non-catalog model',ai_multiple_installed_models)

    async def local_ai_hardware():
        h=await Harness(browser,modules=[ai_module()]).start('/modules/ai.agent')
        await h.page.get_by_role('button',name='Локальный AI',exact=True).click()
        await expect(h.page.get_by_text('Встроенный Ollama',exact=True)).to_be_visible()
        await expect(h.page.get_by_text('qwen3:0.6b',exact=True).first).to_be_visible()
        await expect(h.page.get_by_text('GPU не обнаружены операционной системой.',exact=False)).to_be_visible()
        h.ai_gpus=[
            dict(model='GPU A',vendor='NVIDIA',pci_address='0000:01:00.0',driver='nvidia'),
            dict(model='GPU B',vendor='NVIDIA',pci_address='0000:02:00.0',driver='nvidia'),
        ]
        await h.page.get_by_role('button',name='Проверить оборудование').click()
        await expect(h.page.get_by_text('GPU A',exact=True)).to_be_visible()
        await expect(h.page.get_by_text('GPU B',exact=True)).to_be_visible()
        await expect(h.page.get_by_text('Новое оборудование:',exact=False)).to_be_visible()
        await h.page.locator('.ai-gpu-device input').first.check()
        await h.page.locator('.ai-gpu-device input').last.check()
        await expect(h.page.locator('.ai-gpu-choice')).to_contain_text('Выбрано: 2 из 2')
        assert await h.page.evaluate('document.documentElement.scrollWidth<=innerWidth+1')
        assert h.ai_assigned==[], 'GPU must not auto-allocate after hot plug'
        h.page.once('dialog',lambda d: asyncio.create_task(d.accept()))
        await h.page.get_by_role('button',name='Применить выбранные GPU').click()
        await expect(h.page.get_by_text('Текущий режим:',exact=False)).to_contain_text('NVIDIA GPU')
        assert len(h.ai_assigned)==2, h.ai_assigned
        await h.page.locator('.ai-gpu-device input').first.uncheck()
        await h.page.locator('.ai-gpu-device input').last.uncheck()
        h.page.once('dialog',lambda d: asyncio.create_task(d.accept()))
        await h.page.get_by_role('button',name='Переключить на CPU').click()
        await expect(h.page.get_by_text('Текущий режим:',exact=False)).to_contain_text('CPU')
        assert h.ai_assigned==[]
        h.page.once('dialog',lambda d: asyncio.create_task(d.accept()))
        await h.page.locator('.ai-model-entry').filter(has_text='qwen3:1.7b').get_by_role('button',name='Загрузить').click()
        assert any(p=='/api/v1/ai/local/pull' for _,p,_ in h.requests)
        await expect(h.page.locator('.ai-model-entry').filter(has_text='qwen3:1.7b')).to_contain_text('Установлена')
        await h.close()
    await check('UI-19: bundled Ollama model selection and live multi-GPU hotplug',local_ai_hardware)

    async def live_module_install_progress():
        installing=job('running','module.install','job-install-pull')
        installing['message']='module.phase.download|known:12300000/45600000 aabbccddeeff Downloading 12.3MB/45.6MB'
        installing['started_at']='2026-10-17T20:00:00Z'
        h=await Harness(browser,jobs=[installing]).start('/modules/operations')
        await expect(h.page.get_by_text('Скачивание Docker-образа')).to_be_visible()
        await expect(h.page.get_by_text('Слой образа:',exact=False)).to_contain_text('12.3MB/45.6MB')
        bar=h.page.get_by_role('progressbar',name='Ход установки модуля')
        await expect(bar).to_be_visible()
        assert await bar.get_attribute('aria-valuenow') is None
        await expect(h.page.get_by_text('Получено по известным слоям:',exact=False)).to_be_visible()
        await expect(h.page.get_by_text('Размер всего образа может быть больше:',exact=False)).to_be_visible()
        await expect(h.page.get_by_text('Прошло:',exact=False)).to_be_visible()
        assert await h.page.evaluate('document.documentElement.scrollWidth <= innerWidth + 1')
        installing['status']='succeeded'
        installing['progress']=10000
        await h.reload()
        await expect(h.page.locator('[data-job-id="job-install-pull"]')).to_be_visible()
        await expect(h.page.locator('[data-job-id="job-install-pull"]').get_by_text('100%',exact=False)).to_be_visible()
        await h.close()
    await check('UI-19: module install shows real phase and per-layer bytes without fake total percent',live_module_install_progress)

    async def optional_gpu_runtime_download():
        gpu=dict(model='GPU A',vendor='NVIDIA',pci_address='0000:01:00.0',driver='nvidia')
        h=await Harness(browser,modules=[ai_module()],ai_version='0.3.0',ai_gpus=[gpu]).start('/modules/ai.agent')
        await h.page.get_by_role('button',name='Локальный AI',exact=True).click()
        await expect(h.page.get_by_text('GPU-компоненты:',exact=False)).to_contain_text('не загружены')
        await h.page.locator('.ai-gpu-device input').first.check()
        await expect(h.page.get_by_role('button',name='Применить выбранные GPU')).to_be_disabled()
        assert not h.ai_assigned
        h.page.once('dialog',lambda d: asyncio.create_task(d.accept()))
        await h.page.get_by_role('button',name='Загрузить GPU-компоненты',exact=True).click()
        assert any(req[1]=='/api/v1/ai/gpu/runtime/prepare' for req in h.requests)
        assert h.jobs[0]['type']=='module.gpu.prepare'
        assert not h.ai_assigned, 'download must not allocate hardware'
        await h.page.get_by_role('button',name='Проверить оборудование').click()
        await expect(h.page.get_by_text('GPU-компоненты:',exact=False)).to_contain_text('загружены')
        await expect(h.page.get_by_role('button',name='Применить выбранные GPU')).to_be_enabled()
        h.page.once('dialog',lambda d: asyncio.create_task(d.accept()))
        await h.page.get_by_role('button',name='Применить выбранные GPU').click()
        assert h.ai_assigned==['0000:01:00.0']
        await h.close()
    await check('UI-23: optional GPU download happens before separately approved device assignment',optional_gpu_runtime_download)

    async def responsive_pages():
        for path,kw in [('/users',{}),('/jobs',dict(jobs=[job()])),('/',dict(initialized=False))]:
            h=await Harness(browser,**kw).start(path)
            await expect(h.page.locator('h1').first).to_be_visible()
            for width in [320,360,390,768,1440]:
                await h.page.set_viewport_size(dict(width=width,height=1000));await asyncio.sleep(.1)
                assert await h.page.evaluate('document.documentElement.scrollWidth<=innerWidth+1'),(path,width)
            await h.close()
    await check('UI-12/16: Auth, Users and Jobs fit 320–1440px without document overflow',responsive_pages)

async def main():
    OUT.mkdir(parents=True,exist_ok=True)
    async with async_playwright() as p:
        args=dict(headless=True,args=['--no-sandbox'])
        binary=os.environ.get('CHROMIUM_PATH')
        if binary:args['executable_path']=binary
        browser=await p.chromium.launch(**args)
        try:await suite(browser)
        finally:
            (OUT/'results.json').write_text(json.dumps(dict(environment='production bundle with mocked API; no live deployment',offline=os.environ.get('UI_OFFLINE')=='1',history=('virtual history: actual origin/history must pass CI' if os.environ.get('UI_OFFLINE')=='1' else 'real browser URL/history'),checks=RESULTS),ensure_ascii=False,indent=2))
            await browser.close()
    print(json.dumps(dict(passed=len(RESULTS)),ensure_ascii=False))
if __name__=='__main__':asyncio.run(main())
