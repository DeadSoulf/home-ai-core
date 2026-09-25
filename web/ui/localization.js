// The catalog is embedded at build time. No network translation or dependencies.
const normalize = text => text.replace(/\s+/g, ' ').trim();
const pairs = catalog.trimEnd().split('\n').map(line => line.replace(/\r$/, '').split('\t'));
let language = 'ru';
try { language = localStorage.getItem('homeai.language') === 'en' ? 'en' : 'ru'; } catch (_) {
    language = /(?:^|;\s*)homeai_language=en(?:;|$)/.test(document.cookie) ? 'en' : 'ru';
}
document.documentElement.lang = language;
const dictionary = new Map();
for (const [ru, en] of pairs) {
    if (ru && en) dictionary.set(normalize(language === 'en' ? ru : en), normalize(language === 'en' ? en : ru));
}
const escapePattern = text => text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
const pattern = new RegExp('(?<![\\p{L}\\p{N}_])(?:' + [...dictionary.keys()].sort((a,b) => b.length-a.length).map(escapePattern).join('|') + ')(?![\\p{L}\\p{N}_])', 'gu');
window.tr = function(text) {
    const value = String(text);
    const normalized = normalize(value);
    if (dictionary.has(normalized)) {
        const leading = value.match(/^\s*/)[0], trailing = value.match(/\s*$/)[0];
        return leading + dictionary.get(normalized) + trailing;
    }
    // Preserve paths and URLs embedded in status text.
    return value.split(/((?:https?:\/\/|\/)[^\s,;]+)/g).map((part, index) =>
        index % 2 ? part : part.replace(pattern, match => dictionary.get(match))).join('');
};
// Native dialogs must use the same language as the page.
for (const name of ['alert', 'confirm', 'prompt']) {
    const original = window[name].bind(window);
    window[name] = (message, ...args) => original(window.tr(message), ...args);
}
function translateNode(node) {
    if (node.nodeType === Node.TEXT_NODE) {
        if (!node.parentElement || node.parentElement.closest('script,style,code,pre,textarea,input,[data-i18n-skip]')) return;
        const translated = window.tr(node.nodeValue);
        if (translated !== node.nodeValue) node.nodeValue = translated;
    } else if (node.nodeType === Node.ELEMENT_NODE) {
        if (node.matches('script,style,code,pre,textarea,input,[data-i18n-skip]')) return;
        for (const child of [...node.childNodes]) translateNode(child);
    }
}
document.addEventListener('DOMContentLoaded', () => {
    const label = document.createElement('label');
    label.dataset.i18nSkip = '';
    label.style.cssText = 'display:inline-flex;gap:8px;align-items:center;padding:8px';
    label.textContent = language === 'ru' ? 'Язык' : 'Language';
    const select = document.createElement('select');
    select.id = 'language-selector';
    select.setAttribute('aria-label', label.textContent);
    for (const [value, text] of [['ru','Русский'], ['en','English']]) {
        const option = document.createElement('option');
        option.value = value; option.textContent = text; option.selected = value === language;
        select.appendChild(option);
    }
    select.addEventListener('change', () => {
        try { localStorage.setItem('homeai.language', select.value); } catch (_) {
            // A denied storage policy still permits switching for this page.
            document.cookie = 'homeai_language=' + select.value + '; Path=/; SameSite=Lax';
        }
        window.location.reload();
    });
    label.appendChild(select);
    (document.querySelector('.topbar') || document.body).prepend(label);
    translateNode(document.body);
    document.title = window.tr(document.title);
    const observer = new MutationObserver(records => {
        observer.disconnect();
        for (const record of records) {
            if (record.type === 'characterData') translateNode(record.target);
            else for (const node of record.addedNodes) translateNode(node);
        }
        observer.observe(document.body, {subtree:true, childList:true, characterData:true});
    });
    observer.observe(document.body, {subtree:true, childList:true, characterData:true});
});
