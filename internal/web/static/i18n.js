// Translation helpers and the language picker, shared by every page.
//
// The server renders each page in the visitor's language and hands this file
// the strings the scripts need as window.I18N (already in that language, with
// English filling any gap). Text with a number in it has one key per plural
// form, "x.one", "x.other" and so on, picked with Intl.PluralRules.

(function () {
    const strings = window.I18N || {};
    const lang = window.I18N_LANG || 'en';

    function fill(s, vars) {
        if (!vars) return s;
        return s.replace(/\{(\w+)\}/g, (whole, k) => (k in vars ? String(vars[k]) : whole));
    }

    // t('toast.saved') or t('toast.deleted', { ip: '10.0.0.4' })
    window.t = function (key, vars) {
        const s = strings[key];
        return fill(s === undefined ? key : s, vars);
    };

    // tn('devices.found', 3) fills {count} and picks the right plural form.
    window.tn = function (key, n, vars) {
        let form = 'other';
        try {
            form = new Intl.PluralRules(lang).select(n);
        } catch (e) { /* very old browser: "other" reads acceptably */ }
        const k = strings[key + '.' + form] !== undefined ? key + '.' + form : key + '.other';
        return window.t(k, Object.assign({ count: n }, vars || {}));
    };

    window.I18N_LANG = lang;

    // The choice lives in a cookie so the server can render the next page in
    // it. A year, same site only, readable by scripts because this file sets it.
    window.setLang = function (id) {
        document.cookie = 'orangutan_lang=' + encodeURIComponent(id) + '; path=/; max-age=31536000; SameSite=Lax';
        location.reload();
    };

    window.toggleLangMenu = function (btn) {
        const menu = btn.parentElement.querySelector('.lang-menu');
        const open = menu.hidden;
        document.querySelectorAll('.lang-menu').forEach(m => { m.hidden = true; });
        document.querySelectorAll('.langpicker > button').forEach(b => b.setAttribute('aria-expanded', 'false'));
        menu.hidden = !open;
        btn.setAttribute('aria-expanded', String(open));
    };

    document.addEventListener('mousedown', (e) => {
        if (!e.target.closest('.langpicker')) {
            document.querySelectorAll('.lang-menu').forEach(m => { m.hidden = true; });
        }
    });
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') document.querySelectorAll('.lang-menu').forEach(m => { m.hidden = true; });
    });
})();
