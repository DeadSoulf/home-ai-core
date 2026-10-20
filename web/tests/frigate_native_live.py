"""Actual upstream UI, real Core TLS/cookie auth, synthetic camera only.
Run using scripts/test-frigate-native-ui.sh; no production server is contacted.
"""
import asyncio
import json
import os
import time
from pathlib import Path
from urllib.parse import quote
from playwright.async_api import async_playwright, expect

READY = Path(os.environ['FRIGATE_NATIVE_READY_FILE'])
OUT = Path(os.environ.get('UI_EVIDENCE_DIR', str(READY.parent / 'native-ui-evidence')))

async def main():
    OUT.mkdir(parents=True, exist_ok=True)
    info = json.loads(READY.read_text())
    core, ui = info['core'], info['ui']
    assert core.startswith('https://127.0.0.1:') and ui.startswith('https://127.0.0.1:') and core != ui
    checks = []
    def passed(value):
        checks.append(value)
        print('PASS:', value, flush=True)
    async with async_playwright() as p:
        browser = await p.chromium.launch(headless=True, args=['--no-sandbox'])
        context = await browser.new_context(ignore_https_errors=True, locale='en-US', viewport={'width': 1440, 'height': 1000})
        await context.add_init_script("localStorage.setItem('home-ai-core.locale','ru')")
        page = await context.new_page()
        errors = []
        page.on('pageerror', lambda error: errors.append(str(error)))
        try:
            for path in ['/', '/assets/private.js', '/api/config', '/exports/private.mp4']:
                result = await context.request.get(ui + path)
                assert result.status == 401, (path, result.status)
            passed('anonymous users cannot access native HTML, assets, APIs or recordings')

            await page.goto(core + '/modules/frigate.module')
            await page.locator('input[name=username]').fill('owner')
            await page.locator('input[name=password]').fill('owner correct password')
            await page.locator('button[type=submit]').click()
            iframe = page.locator('iframe.frigate-official-frame')
            await expect(iframe).to_have_attribute('src', ui + '/', timeout=20000)
            frame = await (await iframe.element_handle()).content_frame()
            await expect(frame.locator('#pageRoot')).to_be_visible(timeout=20000)
            assert await frame.evaluate('location.origin') == ui
            assert await frame.evaluate("(() => { try { parent.localStorage.getItem('home-ai-core.csrf'); return false; } catch { return true; } })()")
            sandbox = await iframe.get_attribute('sandbox')
            assert 'allow-top-navigation' not in sandbox and 'allow-scripts' in sandbox
            passed('original upstream application loads inside Core on an isolated origin; parent credentials are inaccessible')

            # Force the original full-quality player (not its smart-stream snapshot).
            await frame.evaluate("location.hash='synthetic'")
            await frame.wait_for_function("[...document.querySelectorAll('video')].some(v=>v.readyState>=3 && v.videoWidth>=320 && v.currentTime>0)", timeout=30000)
            before = await frame.locator('video').evaluate_all('els=>Math.max(...els.map(v=>v.currentTime))')
            await asyncio.sleep(2)
            after = await frame.locator('video').evaluate_all('els=>Math.max(...els.map(v=>v.currentTime))')
            assert after > before + 0.5, (before, after)
            await page.screenshot(path=str(OUT / 'native-live-inside-core.png'), full_page=True)
            passed('original MSE/WebSocket player decodes the real synthetic stream and playback time advances')

            await frame.evaluate("location.href='/settings'")
            await expect(frame.get_by_text('Global configuration', exact=True)).to_be_visible(timeout=20000)
            for label in ['Camera configuration', 'Enrichments', 'System', 'Users', 'Notifications', 'Maintenance']:
                await expect(frame.get_by_text(label, exact=True)).to_be_visible()
            await page.screenshot(path=str(OUT / 'native-settings-inside-core.png'), full_page=True)
            passed('full official settings are available, including global/camera configuration, enrichments, users and maintenance')

            native = await context.new_page()
            native.on('pageerror', lambda error: errors.append(str(error)))
            for route in ['/review', '/explore', '/export', '/system', '/logs', '/config']:
                await native.goto(ui + route)
                await expect(native.locator('#pageRoot')).to_be_visible(timeout=20000)
                if route == '/config':
                    await expect(native.locator('.monaco-editor').first).to_be_visible(timeout=20000)
            passed('original review, search, export, system, logs and Monaco configuration pages load directly')

            for path in ['/api/config', '/api/stats', '/api/review?limit=5', '/api/config/schema.json', '/api/go2rtc/streams', '/api/exports']:
                response = await context.request.get(ui + path)
                assert response.status == 200, (path, response.status)
                await response.json()
            passed('original APIs and configuration schema remain available without a reduced endpoint allowlist')

            recordings = []
            for _ in range(90):
                response = await context.request.get(ui + f'/api/synthetic/recordings?after={time.time()-3600}&before={time.time()}')
                assert response.status == 200
                recordings = [item for item in await response.json() if item['end_time'] - item['start_time'] > 5]
                if recordings:
                    break
                await asyncio.sleep(1)
            assert recordings, 'Frigate did not create real recording segments'
            segment = recordings[0]
            start, end = int(segment['start_time']) + 1, int(segment['start_time']) + 4
            response = await context.request.get(ui + f'/api/vod/synthetic/start/{start}/end/{end}')
            assert response.status == 200, ('recording mapping', response.status, await response.text())
            response = await context.request.get(ui + f'/vod/synthetic/start/{start}/end/{end}/index.m3u8')
            assert response.status == 200 and '#EXTM3U' in await response.text(), ('HLS', response.status, await response.text())
            passed('Frigate writes real recordings; archive mapping and native HLS playlist pass through the gateway')

            response = await context.request.post(ui + f'/api/export/synthetic/start/{start}/end/{end}',
                data={'source': 'recordings', 'name': 'HomeAiCore native UI acceptance'}, headers={'Origin': ui})
            assert response.status in (200, 202), ('export', response.status, await response.text())
            export_id = (await response.json())['export_id']
            exported = None
            export_diagnostics = []
            for attempt in range(90):
                response = await context.request.get(ui + '/api/exports', headers={'X-Cache-Bypass': '1'})
                assert response.status == 200
                exports = await response.json()
                if attempt in (0, 5, 30, 60):
                    print('SYNTHETIC_EXPORT_STATE', export_id, exports, flush=True)
                for entry in exports:
                    # The official UI waits for completion before downloading.
                    # Probing a queued filename can populate nginx's negative
                    # open-file cache even after the export has finished.
                    if entry.get('id') == export_id and entry.get('video_path') and not entry.get('in_progress', True):
                        video_path = entry['video_path']
                        assert video_path.startswith('/media/frigate/exports/')
                        download = await context.request.get(ui + '/exports/' + quote(video_path.split('/media/frigate/exports/', 1)[1]), headers={'Range': 'bytes=0-255'})
                        if attempt in (0, 5, 30, 60):
                            print('SYNTHETIC_EXPORT_DOWNLOAD', download.status, download.headers.get('content-range'), flush=True)
                        export_diagnostics.append(download.status)
                        if download.status == 206:
                            assert 'ftyp' in (await download.body()).decode('latin-1')
                            exported = entry
                            break
                if exported:
                    break
                await asyncio.sleep(1)
            assert exported, ('real recording export was not downloadable', export_id, export_diagnostics[-5:])
            passed('native export produces an actual MP4; authenticated byte-range download works')

            response = await context.request.get(ui + '/api/config/raw')
            assert response.status == 200
            raw = await response.text()
            try:
                decoded = json.loads(raw)
                if isinstance(decoded, str):
                    raw = decoded
            except ValueError:
                pass
            denied = await context.request.post(ui + '/api/config/save?save_option=saveonly', data=raw,
                headers={'Origin': 'https://untrusted.test', 'Content-Type': 'text/plain'})
            assert denied.status == 403
            response = await context.request.post(ui + '/api/config/save?save_option=saveonly', data=raw,
                headers={'Origin': ui, 'Content-Type': 'text/plain'})
            assert response.status == 200 and (await response.json()).get('success') is True, await response.text()
            passed('native YAML validation/save works; cross-origin writes are rejected without changing configuration')

            csrf = await page.evaluate("localStorage.getItem('home-ai-core.csrf')")
            response = await context.request.post(core + '/api/v1/auth/logout', headers={'X-CSRF-Token': csrf, 'Origin': core})
            assert response.status == 204
            for path in ['/', '/api/config', '/exports/' + quote(exported['video_path'].split('/media/frigate/exports/', 1)[1])]:
                assert (await context.request.get(ui + path)).status == 401
            passed('real Core logout immediately removes access to native UI, APIs and exported recordings')
            assert not errors, errors
        finally:
            (OUT / 'results.json').write_text(json.dumps({'environment': 'real Core TLS/session gateway, official Frigate 0.18.2, synthetic camera only', 'checks': checks, 'javascript_errors': errors}, ensure_ascii=False, indent=2), encoding='utf-8')
            await browser.close()
    print(json.dumps({'native_checks_passed': len(checks)}), flush=True)

if __name__ == '__main__':
    asyncio.run(main())
