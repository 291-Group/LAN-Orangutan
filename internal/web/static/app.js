// Theme

// applyTheme switches the theme with transitions turned off for the switch, so
// every element changes at the same instant. Left on, some elements fade over
// their own transition while others (the page background) snap immediately,
// which reads as the page changing in pieces.
function applyTheme(theme) {
    const root = document.documentElement;
    // Do this synchronously with forced reflows rather than on an animation
    // frame: requestAnimationFrame does not fire in a background tab, which
    // would leave transitions disabled if the tab lost focus mid-switch.
    root.classList.add('theme-switching');
    void root.offsetWidth;                 // commit "transitions off"
    root.setAttribute('data-theme', theme);
    void root.offsetWidth;                 // commit the new theme instantly
    root.classList.remove('theme-switching'); // restore transitions for hover
    localStorage.setItem('theme', theme);
    syncThemeRadios(theme);
    updateThemeToggle(theme);
}

// Keep the Settings page's Light/Dark/Auto radios showing the theme that is
// actually applied. Without this they are checked from the server's config
// default, so a browser that pinned Light/Dark with the header button would
// still show "Auto" selected and leave the user unable to tell what is active
// (or to re-pick a value that already looks chosen). A no-op on pages without
// the radios (the dashboard).
function syncThemeRadios(theme) {
    document.querySelectorAll('input[name="theme"]').forEach(function (radio) {
        radio.checked = radio.value === theme;
    });
}

// Tooltips for the header theme button, one per state. The three icons (sun
// for light, moon for dark, monitor for "follow the system") are in the page
// markup, and the stylesheet shows the one matching data-theme, which the inline
// script in <head> sets before the first paint. Drawing the icon here instead
// made it change visibly on every page load.
const THEME_TOGGLE_TITLES = {
    light: 'app.theme.light',
    dark: 'app.theme.dark',
    auto: 'app.theme.auto'
};

// Keep the header button's tooltip describing the active state and what the
// next click does.
function updateThemeToggle(theme) {
    const state = THEME_TOGGLE_TITLES[theme] ? theme : 'auto';
    const title = t(THEME_TOGGLE_TITLES[state]);
    document.querySelectorAll('.theme-toggle').forEach(function (btn) {
        btn.setAttribute('title', title);
        btn.setAttribute('aria-label', title);
    });
}

// The header button cycles light -> dark -> auto -> light, matching the three
// choices on the settings page so the two controls can never disagree. The icon
// changes on every click, so landing on Auto is always a visible change even
// when Auto looks like the current system theme.
function toggleTheme() {
    const current = document.documentElement.getAttribute('data-theme');
    const next = current === 'light' ? 'dark' : current === 'dark' ? 'auto' : 'light';
    applyTheme(next);
}

// Initialize theme
(function() {
    const saved = localStorage.getItem('theme');
    if (saved) {
        document.documentElement.setAttribute('data-theme', saved);
        // Reflect the real (localStorage) choice in the Settings radios, which
        // the server rendered from the config default and would otherwise show
        // a value the browser is not actually using.
        syncThemeRadios(saved);
    }
    // Point the header button at whatever is actually active (the saved choice,
    // or the server's config default when nothing is saved).
    updateThemeToggle(document.documentElement.getAttribute('data-theme') || 'auto');
})();

// Accent colour. Blue is the default. Orange is the stylesheet's base and
// carries no attribute; every other accent (including blue) sets data-accent,
// which the stylesheet turns into the accent for both light and dark. The
// choice is stored so it applies on every page and every visit.
function applyAccent(name) {
    name = name || 'blue';
    if (name === 'orange') {
        document.documentElement.removeAttribute('data-accent');
    } else {
        document.documentElement.setAttribute('data-accent', name);
    }
    localStorage.setItem('accent', name);
}

// setAccent is called from the settings swatches: apply and mark the active one.
function setAccent(name) {
    applyAccent(name);
    document.querySelectorAll('.accent-swatch').forEach(function(sw) {
        sw.classList.toggle('active', sw.dataset.accent === name);
    });
}

(function() {
    applyAccent(localStorage.getItem('accent'));
    // Mark the active swatch on the settings page once it has rendered.
    document.addEventListener('DOMContentLoaded', function() {
        const current = localStorage.getItem('accent') || 'blue';
        document.querySelectorAll('.accent-swatch').forEach(function(sw) {
            sw.classList.toggle('active', sw.dataset.accent === current);
        });
    });
})();

// Toast notifications
function showToast(message, type = 'info', duration = 3000) {
    const toast = document.getElementById('toast');
    if (!toast) return;
    toast.textContent = message;
    toast.className = 'toast ' + type + ' show';
    setTimeout(() => toast.classList.remove('show'), duration);
}

// API helper
//
// A failed request throws an Error carrying the server's message, and, when
// the server identified the problem, its `code` and `args` so errorText can
// show it in the page's language.
async function api(action, params = {}, method = 'GET') {
    let url = `/api/${action}`;
    const options = { method };
    if (method === 'GET') {
        const queryParams = Object.keys(params).map(key => `${key}=${encodeURIComponent(params[key])}`).join('&');
        if (queryParams) url += `?${queryParams}`;
    } else {
        options.headers = { 'Content-Type': 'application/json' };
        options.body = JSON.stringify(params);
    }
    try {
        const response = await fetch(url, options);
        const text = await response.text();
        if (!response.ok) {
            // The body is JSON on every error the server itself produced. A
            // proxy or a crash in front of it can still answer with something
            // else, which is the only case the generic HTTP message is for.
            let body = null;
            try { body = JSON.parse(text); } catch (e) { body = null; }
            const fallback = response.status === 429 ? t('app.error.rateLimited') : t('app.error.http', { status: response.status });
            const err = apiError(body, fallback);
            err.status = response.status;
            err.rateLimited = response.status === 429;
            throw err;
        }
        if (!text) throw new Error(t('app.error.emptyResponse'));
        return JSON.parse(text);
    } catch (error) {
        console.error('API error:', error);
        throw error;
    }
}

// apiError turns an error response body ({error, code, args}) into an Error
// that errorText can translate. fallback is used when the body has no message.
function apiError(body, fallback) {
    const err = new Error((body && body.error) || fallback);
    if (body && body.code) {
        err.code = body.code;
        err.args = body.args || {};
    }
    return err;
}

// errorText is the message to show for an error from api(): the catalogue's
// translation when the server named the problem and this language has it,
// otherwise the server's own English text.
function errorText(err) {
    if (err && err.code) {
        const key = 'app.error.' + err.code;
        if (window.I18N && window.I18N[key] !== undefined) return t(key, err.args || {});
    }
    return (err && err.message) || String(err);
}

// Network scanning
//
// Scans run in the background on the server and the page polls for progress.
// A large network takes minutes, which is far too long to hold a request open.
const SCAN_POLL_MS = 1000;

function formatSeconds(total) {
    const s = Math.max(0, Math.round(total));
    return s < 60
        ? t('app.scan.seconds', { s })
        : t('app.scan.minutesSeconds', { m: Math.floor(s / 60), s: String(s % 60).padStart(2, '0') });
}

function showScanProgress(p) {
    const panel = document.getElementById('scan-progress');
    if (!panel) return;
    panel.style.display = 'flex';

    // Once every network is swept the job asks the network for IPv6
    // neighbours and mDNS names. That pass has no estimate, so it gets its own
    // label and an indeterminate bar rather than the last network's bar
    // sitting at 100% with "~0s left".
    const discovering = p.phase === 'discover';

    const title = document.getElementById('scan-title');
    if (title) {
        title.textContent = discovering ? t('app.scan.phaseDiscover')
            : p.current_network ? t('app.scan.scanningNetwork', { network: p.current_network })
            : t('app.scan.starting');
    }

    // percent is -1 when the network has never been scanned and there is no
    // timing history to estimate from. Show a sweeping bar rather than a
    // made-up number.
    const bar = document.getElementById('scan-bar');
    const fill = document.getElementById('scan-bar-fill');
    const known = !discovering && p.percent >= 0;
    if (bar) bar.classList.toggle('indeterminate', !known);
    if (fill) fill.style.width = known ? `${Math.min(100, p.percent)}%` : '';

    const detail = document.getElementById('scan-detail');
    if (detail) {
        const parts = [];
        if (!discovering && p.network_count > 1) parts.push(t('app.scan.networkOf', { index: p.network_index, total: p.network_count }));
        if (p.current_network) parts.push(p.current_network);

        detail.textContent = parts.join(' · ');
    }

    // The running total gets its own line. Appended to the line above it wrapped
    // mid-sentence once more than one network was involved.
    //
    // Only claim a count once there is one: nmap reports nothing until its
    // sweep finishes, so a permanent "0 devices found" reads as a failed scan
    // rather than an unfinished one.
    const count = document.getElementById('scan-count');
    if (count) {
        count.textContent = p.device_count > 0
            ? tn('app.scan.devicesFound', p.device_count)
            : '';
    }

    const eta = document.getElementById('scan-eta');
    if (eta) {
        eta.textContent = known && p.remaining != null
            ? t('app.scan.timeLeft', { time: formatSeconds(p.remaining), percent: Math.round(p.percent) })
            : t('app.scan.elapsed', { time: formatSeconds(p.elapsed) });
    }

    // Set expectations explicitly while there is nothing to report yet. An
    // IPv6 network is never swept address by address, so it gets the hint
    // that matches what its card says.
    const hint = document.getElementById('scan-hint');
    if (hint) {
        hint.textContent = p.device_count > 0 || discovering
            ? ''
            : (p.current_network || '').includes(':') ? t('app.scan.hintIPv6') : t('app.scan.hint');
    }
}

function hideScanProgress() {
    const panel = document.getElementById('scan-progress');
    if (panel) panel.style.display = 'none';
}

async function cancelScan() {
    try {
        await api('scan/cancel', {}, 'POST');
        showToast(t('app.scan.cancelled'), 'warning');
    } catch (e) {
        showToast(t('app.scan.cancelFailed', { error: errorText(e) }), 'error');
    }
}

// The supplemental passes report under these names in the per-network list.
// They are sources, not networks, so they do not count as one.
const SUPPLEMENTAL_SOURCES = new Set(['ARP table', 'IPv6 neighbors', 'mDNS']);

// Reports the outcome of a finished job. Networks that were rate limited or
// failed are surfaced rather than being hidden behind a success message.
function reportScanOutcome(p) {
    const all = p.networks || [];
    const scanned = all.filter(n => n.status === 'scanned');
    const real = all.filter(n => !SUPPLEMENTAL_SOURCES.has(n.network));
    const scannedReal = real.filter(n => n.status === 'scanned');
    const failed = real.filter(n => n.status === 'failed');
    if (p.status === 'cancelled') {
        showToast(t('app.scan.cancelled'), 'warning');
        if (scanned.length) setTimeout(refreshAfterScan, 1000);
        return;
    }
    if (scanned.length === 0) {
        const reason = all.find(n => n.error)?.error;
        showToast(reason ? t('app.scan.failed', { error: reason }) : t('app.scan.noNetworks'), 'warning');
        return;
    }
    const found = p.network_count > 1
        ? tn('app.scan.foundAcross', p.device_count, { networks: tn('app.scan.networkCount', scannedReal.length) })
        : tn('app.scan.found', p.device_count);
    // A network that failed is a warning, not a success, however well the
    // others went: the user should know part of the picture is missing.
    if (failed.length) {
        const list = failed.map(n => n.network).join(', ');
        showToast(found + ' · ' + tn('app.scan.failedNetworks', failed.length, { list }), 'warning', 6000);
    } else {
        showToast(found, 'success');
    }
    setTimeout(refreshAfterScan, 1000);
}

// refreshAfterScan brings in the newly discovered devices without throwing away
// the user's scroll position, search text or filters.
async function refreshAfterScan() {
    if (!(await refreshInPlace())) location.reload();
}

// Follows a scan that is already running until it stops. Scans live on the
// server, not in the page, so this is used both by the page that starts one and
// by any page loaded while one is already in progress.
async function followScan() {
    while (true) {
        await new Promise(r => setTimeout(r, SCAN_POLL_MS));
        const progress = (await api('scan/progress')).data;
        // Stop when the scan ends, or when the current job is an automatic
        // background scan: the user's scan has finished and the background
        // scanner has taken the single job slot, so there is nothing of the
        // user's left to follow. Automatic scans never own the overlay.
        if (progress.status !== 'running' || progress.automatic) {
            hideScanProgress();
            if (!progress.automatic) reportScanOutcome(progress);
            return;
        }
        showScanProgress(progress);
    }
}

async function runScan(target) {
    try {
        const started = await api(`scan/start?network=${encodeURIComponent(target)}`, {}, 'POST');
        showScanProgress(started.data);
        await followScan();
    } catch (e) {
        hideScanProgress();
        showToast(e.rateLimited ? errorText(e) : t('app.scan.failed', { error: errorText(e) }), e.rateLimited ? 'warning' : 'error');
    }
}

// Picks up a scan that was started before this page was loaded, for instance
// when the user starts a scan and then moves to the settings page. Without
// this, the scan carries on invisibly and the page never notices it finish.
async function resumeScanIfRunning() {
    try {
        const progress = (await api('scan/progress')).data;
        // Only resume a scan the user started. An automatic background scan
        // must not pop the progress overlay (with its Cancel button) on its
        // own; its results simply appear on the next refresh.
        if (progress && progress.status === 'running' && !progress.automatic) {
            showScanProgress(progress);
            await followScan();
        }
    } catch (e) {
        // No scan has ever run, or this page cannot ask. Nothing to show.
    }
}

resumeScanIfRunning();

async function scanNetwork(cidr) {
    await runScan(cidr);
}

async function scanAllNetworks() {
    await runScan('all');
}

// Device filtering
// Filter set by clicking a summary chip ("flagged" / "moved"), or null.
let statChipFilter = null;

// Toggle the chip filter: a second click on the same chip clears it.
function applyStatChipFilter(kind) {
    statChipFilter = statChipFilter === kind ? null : kind;
    filterDevices();
    if (statChipFilter) {
        document.querySelector('.table-container')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
}

function filterDevices() {
    // Reflect the active chip filter on the chips, which an auto-refresh may
    // have just swapped out from under us.
    document.querySelectorAll('.stat-chip-filter').forEach(chip =>
        chip.classList.toggle('active', !!statChipFilter && chip.dataset.filter === statChipFilter));

    const search = (document.getElementById('device-search')?.value || '').toLowerCase();
    const statusFilter = document.getElementById('device-filter')?.value || 'all';
    const groupFilter = document.getElementById('group-filter')?.value || 'all';
    const locationFilter = document.getElementById('location-filter')?.value || 'all';

    let visible = 0;
    document.querySelectorAll('.device-row').forEach(row => {
        const text = [row.dataset.ip, row.dataset.hostname, row.dataset.mac, row.dataset.vendor, row.dataset.label, row.dataset.type, row.dataset.location].join(' ').toLowerCase();
        const status = row.dataset.status;
        const group = row.dataset.group || '';

        const matchSearch = !search || text.includes(search);
        const matchStatus = statusFilter === 'all' ||
            (statusFilter === 'online' && (status === 'online' || status === 'recent')) ||
            (statusFilter === 'offline' && status === 'offline');
        const matchGroup = groupFilter === 'all' || group === groupFilter;
        const matchLocation = locationFilter === 'all' || (row.dataset.location || '') === locationFilter;
        const matchChip = !statChipFilter ||
            (statChipFilter === 'moved' && row.dataset.moved === '1') ||
            (statChipFilter === 'flagged' && row.dataset.flagged === '1') ||
            (statChipFilter === 'unexpected' && row.dataset.unexpected === '1') ||
            (statChipFilter === 'missing' && row.dataset.missing === '1');

        const show = matchSearch && matchStatus && matchGroup && matchLocation && matchChip;
        row.style.display = show ? '' : 'none';
        if (show) visible++;
    });

    const countEl = document.getElementById('device-count');
    if (countEl) countEl.textContent = tn('app.devices.showing', visible);
}

// Device editing
function editDevice(ip) {
    const modal = document.getElementById('edit-modal');
    const row = document.querySelector(`.device-row[data-ip="${CSS.escape(ip)}"]`);
    if (!modal || !row) return;
    document.getElementById('edit-ip').value = ip;
    document.getElementById('edit-ip-display').value = ip;
    document.getElementById('edit-label').value = row.dataset.labelOriginal || '';
    document.getElementById('edit-group').value = row.dataset.group || '';
    const locInput = document.getElementById('edit-location');
    if (locInput) locInput.value = row.dataset.location || '';
    document.getElementById('edit-notes').value = row.dataset.notes || '';
    const hostInput = document.getElementById('edit-hostname');
    if (hostInput) {
        const scanned = row.dataset.hostnameScanned || '';
        hostInput.value = row.dataset.customHostname || '';
        hostInput.placeholder = scanned || t('app.edit.noHostname');
        const hint = document.getElementById('edit-hostname-hint');
        if (hint) {
            hint.textContent = scanned
                ? t('app.edit.hostnameHintScanned', { hostname: scanned })
                : t('app.edit.hostnameHint');
        }
    }
    const exp = document.getElementById('edit-expected');
    if (exp) exp.checked = row.dataset.expected === '1';

    const currentType = row.dataset.typeOriginal || '';
    const isCustom = row.dataset.customType === '1';
    const typeSelect = document.getElementById('edit-type-select');
    if (typeSelect) {
        const initialVal = (isCustom && currentType) ? currentType : '';
        typeSelect.value = initialVal;
        typeSelect.dataset.initial = initialVal;
    }

    modal.style.display = 'flex';
}

function closeModal() {
    // Close whichever modal is open, so Escape and a backdrop click work for all
    // of them (edit device, snapshots).
    document.querySelectorAll('.modal').forEach(m => { m.style.display = 'none'; });
}

// --- Snapshots ---------------------------------------------------------------

function openSnapshots() {
    document.getElementById('snapshots-modal').style.display = 'flex';
    document.getElementById('compare-result').textContent = '';
    loadSnapshots();
}

function formatSnapDate(iso) {
    try { return new Date(iso).toLocaleString(); } catch (e) { return iso; }
}

async function loadSnapshots() {
    const res = await api('snapshots');
    const snaps = (res && res.success && Array.isArray(res.data)) ? res.data : [];
    const list = document.getElementById('snapshot-list');
    const fromSel = document.getElementById('compare-from');
    const toSel = document.getElementById('compare-to');
    list.textContent = '';
    fromSel.innerHTML = '';
    toSel.innerHTML = '';

    if (snaps.length === 0) {
        const p = document.createElement('p');
        p.className = 'form-help';
        p.style.margin = '0';
        p.textContent = t('app.snapshots.none');
        list.appendChild(p);
    } else {
        snaps.forEach(s => {
            const row = document.createElement('div');
            row.className = 'snapshot-row';
            const info = document.createElement('span');
            info.textContent = tn('app.snapshots.row', s.device_count, { name: s.name, date: formatSnapDate(s.created_at) });
            const del = document.createElement('button');
            del.className = 'btn btn-sm';
            del.textContent = t('app.snapshots.delete');
            del.onclick = () => deleteSnapshot(s.id);
            row.appendChild(info);
            row.appendChild(del);
            list.appendChild(row);
            fromSel.add(new Option(s.name, s.id));
            toSel.add(new Option(s.name, s.id));
        });
    }
    // "Current network" is always a valid comparison target.
    toSel.add(new Option(t('app.snapshots.currentNetwork'), 'current'));
    toSel.value = 'current';
}

async function saveSnapshot() {
    const name = document.getElementById('snapshot-name').value.trim();
    const status = document.getElementById('snapshot-status');
    const res = await api('snapshots', { name }, 'POST');
    if (res && res.success) {
        document.getElementById('snapshot-name').value = '';
        status.textContent = t('app.snapshots.saved');
        status.style.color = 'var(--success)';
        loadSnapshots();
    } else {
        status.textContent = res ? errorText(apiError(res, t('app.snapshots.saveFailed'))) : t('app.snapshots.saveFailed');
        status.style.color = 'var(--danger)';
    }
    setTimeout(() => { status.textContent = ''; }, 2500);
}

async function deleteSnapshot(id) {
    try {
        // DELETE takes the id as a query param and needs the CSRF header.
        await fetch(`/api/snapshots?id=${encodeURIComponent(id)}`, {
            method: 'DELETE',
            headers: { 'X-Requested-With': 'XMLHttpRequest' }
        });
    } catch (e) { /* ignore; the reload below reflects reality */ }
    document.getElementById('compare-result').textContent = '';
    loadSnapshots();
}

async function runCompare() {
    const from = document.getElementById('compare-from').value;
    const to = document.getElementById('compare-to').value;
    const result = document.getElementById('compare-result');
    if (!from || !to) { result.textContent = t('app.compare.pickTwo'); return; }
    if (from === to) { result.textContent = t('app.compare.pickDifferent'); return; }
    const res = await api('snapshots/compare', { from, to });
    if (!res || !res.success) {
        result.textContent = res ? errorText(apiError(res, t('app.compare.failed'))) : t('app.compare.failed');
        return;
    }
    renderDiff(res.data);
}

// Display names for the fields a snapshot comparison reports as changed. An
// unknown field is shown under its own name.
const COMPARE_FIELD_KEYS = {
    ip: 'app.compare.field.ip',
    hostname: 'app.compare.field.hostname',
    vendor: 'app.compare.field.vendor',
    type: 'app.compare.field.type',
    custom_hostname: 'app.compare.field.customHostname',
    label: 'app.compare.field.label',
    group: 'app.compare.field.group',
    location: 'app.compare.field.location',
    expected: 'app.compare.field.expected'
};

function renderDiff(d) {
    const result = document.getElementById('compare-result');
    result.textContent = '';
    const total = d.added.length + d.removed.length + d.changed.length;
    if (total === 0) {
        const p = document.createElement('p');
        p.className = 'form-help';
        p.style.margin = '0';
        p.textContent = t('app.compare.noDifferences');
        result.appendChild(p);
        return;
    }
    const groups = [
        { key: 'added', labelKey: 'app.compare.added', cls: 'diff-added', sign: '+' },
        { key: 'removed', labelKey: 'app.compare.removed', cls: 'diff-removed', sign: '−' },
        { key: 'changed', labelKey: 'app.compare.changed', cls: 'diff-changed', sign: '~' }
    ];
    groups.forEach(g => {
        const items = d[g.key];
        if (!items || items.length === 0) return;
        const head = document.createElement('div');
        head.className = 'diff-group-head ' + g.cls;
        head.textContent = t(g.labelKey, { count: items.length });
        result.appendChild(head);
        items.forEach(item => {
            const dev = g.key === 'changed' ? item.after : item;
            let text = `${g.sign} ${dev.ip}`;
            if (dev.hostname) text += ` · ${dev.hostname}`;
            else if (dev.vendor) text += ` · ${dev.vendor}`;
            if (g.key === 'changed') {
                text += '  (' + item.fields.map(f => t('app.compare.fieldChange', {
                    field: COMPARE_FIELD_KEYS[f] ? t(COMPARE_FIELD_KEYS[f]) : f,
                    before: item.before[f] || '-',
                    after: item.after[f] || '-'
                })).join(', ') + ')';
            }
            const line = document.createElement('div');
            line.className = 'diff-line';
            line.textContent = text;
            result.appendChild(line);
        });
    });
}

async function saveDevice() {
    const ip = document.getElementById('edit-ip').value;
    const label = document.getElementById('edit-label').value;
    const group = document.getElementById('edit-group').value;
    const notes = document.getElementById('edit-notes').value;
    const expected = document.getElementById('edit-expected') ? document.getElementById('edit-expected').checked : false;

    const payload = { ip, label, group, notes, expected };
    const locInput = document.getElementById('edit-location');
    if (locInput) payload.location = locInput.value.trim();
    const hostInput = document.getElementById('edit-hostname');
    if (hostInput) payload.custom_hostname = hostInput.value.trim();
    const typeSelect = document.getElementById('edit-type-select');
    if (typeSelect) {
        if (typeSelect.value !== '') {
            payload.type = typeSelect.value;
            payload.custom_type = true;
        } else if (typeSelect.dataset.initial && typeSelect.dataset.initial !== '') {
            payload.type = '';
            payload.custom_type = false;
        }
    }

    try {
        const result = await api('device', payload, 'POST');
        if (result.success) {
            showToast(t('app.toast.deviceUpdated'), 'success');
            closeModal();
            // Keep the user where they were; editing a device halfway down a
            // long list should not jump back to the top.
            if (!(await refreshInPlace())) location.reload();
        } else {
            showToast(errorText(apiError(result, t('app.toast.deviceUpdateFailed'))), 'error');
        }
    } catch (e) {
        showToast(errorText(e), 'error');
    }
}

// Mark every current device as expected, baselining the network so the
// "unexpected" and "missing" flags become meaningful afterwards.
async function markAllExpected() {
    try {
        const result = await api('devices/mark-expected', { expected: true }, 'POST');
        if (result && result.success) {
            showToast(t('app.toast.allMarkedExpected'), 'success');
            if (!(await refreshInPlace())) location.reload();
        } else {
            showToast(errorText(apiError(result, t('app.toast.devicesUpdateFailed'))), 'error');
        }
    } catch (e) {
        showToast(errorText(e), 'error');
    }
}

async function deleteDevice(ip) {
    if (!confirm(t('app.confirm.deleteDevice', { ip }))) return;
    try {
        const response = await fetch(`/api/device?ip=${encodeURIComponent(ip)}`, {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' }
        });
        const result = await response.json();
        if (result.success) {
            showToast(t('app.toast.deviceDeleted'), 'success');
            document.querySelector(`.device-row[data-ip="${CSS.escape(ip)}"]`)?.remove();
            filterDevices(); // Update count
        } else {
            showToast(errorText(apiError(result, t('app.toast.deleteFailed'))), 'error');
        }
    } catch (e) {
        showToast(errorText(e), 'error');
    }
}

async function updateDeviceGroup(select) {
    const ip = select.dataset.ip;
    const group = select.value;
    try {
        const result = await api('device', { ip, group }, 'POST');
        if (result.success) {
            showToast(t('app.toast.groupUpdated'), 'success');
            // Update data attribute, then re-apply the current filters so a row
            // moved out of the active group filter stops showing (and the
            // "Showing N devices" count stays honest) without waiting for the
            // next refresh.
            const row = select.closest('.device-row');
            if (row) row.dataset.group = group;
            filterDevices();
        } else {
            showToast(errorText(apiError(result, t('app.toast.updateFailed'))), 'error');
        }
    } catch (e) {
        showToast(errorText(e), 'error');
    }
}

// Copy to clipboard.
//
// The async Clipboard API (navigator.clipboard) is only exposed in a secure
// context: HTTPS, or localhost. On a phone opening the dashboard over plain
// http on the LAN (http://<pi-ip>:291) it is not available, so we fall back to
// the legacy execCommand path. That is how most people reach a self-hosted
// instance, so the fallback is the common case, not an edge one.
function copyToClipboard(text, event) {
    const ok = () => showCopied(event);
    const fail = () => showToast(t('app.copy.failed'), 'error');
    // Chromium over plain http cannot be written to at all (see
    // clipboardWriteBlocked). The one copy the browser still honours there is
    // the user's own keyboard shortcut, so select the value for them and say so,
    // instead of claiming a copy that did not happen.
    if (clipboardWriteBlocked()) {
        if (selectValueText(event, text)) {
            showCopied(event, t('app.copy.pressShortcut', { shortcut: copyShortcutName() }));
        } else {
            fail();
        }
        return;
    }
    // Non-secure fallbacks, in order: the copy-event route first (nothing to
    // tear down, so nothing to race), the hidden-field route last.
    const fallback = () => copyViaEvent(text) || legacyCopy(text);
    if (navigator.clipboard && window.isSecureContext) {
        navigator.clipboard.writeText(text).then(ok).catch(() => {
            if (fallback()) ok(); else fail();
        });
    } else if (fallback()) {
        ok();
    } else {
        fail();
    }
}

// The floating "Copied!" note at the tap/cursor point. A different message
// can be given for the case where the user has to finish the copy themselves.
function showCopied(event, message) {
    const feedback = document.createElement('div');
    feedback.className = 'copy-feedback';
    feedback.textContent = message || t('app.copy.copied');
    document.body.appendChild(feedback);
    // .copy-feedback is position: fixed, so it is placed in viewport
    // coordinates (clientX/clientY). pageX/pageY include the scroll offset,
    // which pushed the note far below the viewport once the page was scrolled
    // down (e.g. the IPv6 rows), so it never showed there. A synthetic event
    // may lack coordinates, so fall back to the centre of the screen.
    //
    // Centre the note on the tap, then clamp it inside the viewport keeping a
    // comfortable margin from every edge. On a phone the values are right
    // aligned in their cards, so the tap lands near the right edge; without a
    // real margin the note sits flush against (or off) the screen edge, which
    // is what looked wrong. Measured after append for its real size.
    const pad = 16;
    const w = feedback.offsetWidth;
    const x = event && event.clientX ? event.clientX : window.innerWidth / 2;
    const y = event && event.clientY ? event.clientY : 40;
    const left = Math.max(pad, Math.min(x - w / 2, window.innerWidth - w - pad));
    const top = Math.max(pad, y - 30);
    feedback.style.left = left + 'px';
    feedback.style.top = top + 'px';
    // "Copied!" is a flash; an instruction needs long enough to be read and
    // acted on.
    setTimeout(() => feedback.remove(), message ? 2500 : 800);
}

// Chromium (Chrome, Edge, Brave) no longer writes the clipboard from a page
// served over plain http, whichever route is used: execCommand reports success,
// the copy event fires, and nothing lands. Reproduced on Chromium 153 against
// this dashboard (GitHub #43: Brave and Edge fail, Firefox copies, all on the
// same http://<ip> instance). There is no page-side way around it and no way to
// detect it after the fact, since every call reports success, so it has to be
// decided up front from the two facts that define it: not a secure context, and
// a Chromium browser. window.chrome is present in Chrome, Edge and Brave on every
// page and absent in Firefox and Safari. (navigator.userAgentData would be the
// tidier signal, but it is itself secure-context-only, so it is undefined on
// precisely the pages this check exists for.) The user-agent test is a backstop
// for a Chromium build that hides window.chrome.
function clipboardWriteBlocked() {
    if (window.isSecureContext) return false;
    return !!window.chrome || /\b(Chrome|Chromium|Edg|Brave)\//.test(navigator.userAgent);
}

// Select the copied value's own text in the clicked cell, so a keyboard copy
// takes exactly that value and nothing else in the cell (an icon, the IPv6
// line under an IPv4 address). Returns whether a selection was made.
function selectValueText(event, text) {
    const cell = event && event.target && event.target.closest ? event.target.closest('[data-copy]') : null;
    if (!cell || !window.getSelection) return false;
    let node = null;
    for (const child of cell.childNodes) {
        if (child.nodeType === Node.TEXT_NODE && child.textContent.trim() === text) {
            node = child;
            break;
        }
    }
    const range = document.createRange();
    if (node) {
        const start = node.textContent.indexOf(text);
        range.setStart(node, start);
        range.setEnd(node, start + text.length);
    } else {
        range.selectNodeContents(cell);
    }
    const selection = window.getSelection();
    selection.removeAllRanges();
    selection.addRange(range);
    return true;
}

// The copy shortcut as the user knows it on their platform.
function copyShortcutName() {
    const p = navigator.platform || ((navigator.userAgentData && navigator.userAgentData.platform) || '');
    return /mac|iphone|ipad/i.test(p) ? '\u2318C' : 'Ctrl+C';
}

// Clipboard copy for non-secure contexts (plain-http LAN access, the common
// case for a self-hosted instance) with no element involved at all.
//
// execCommand('copy') fires a `copy` event; a listener that sets the event's
// clipboardData and prevents the default is what the browser then puts on the
// clipboard. Nothing is added to the page and nothing is removed afterwards.
// That matters because the hidden-field route below removes its field the
// instant execCommand returns, and on some systems the clipboard is served
// lazily by whichever element owns it, so tearing that element down at once
// can leave an empty clipboard while the copy still reported success (the
// "Copied!" note with nothing to paste that was reported against v3.3.8).
// Returns whether the browser accepted the copy.
function copyViaEvent(text) {
    if (typeof document.execCommand !== 'function') return false;
    let handled = false;
    const onCopy = (e) => {
        if (!e.clipboardData) return;
        e.clipboardData.setData('text/plain', text);
        e.preventDefault();
        handled = true;
    };
    document.addEventListener('copy', onCopy, { once: true });
    let ok = false;
    try {
        ok = document.execCommand('copy');
    } catch (e) {
        ok = false;
    }
    // { once } already removed it if the event fired; this covers the case
    // where execCommand did not dispatch a copy event at all.
    document.removeEventListener('copy', onCopy);
    return ok && handled;
}

// Last-resort clipboard copy: a hidden, selectable field and
// document.execCommand('copy'). Deprecated, but still the widest-supported
// path when copyViaEvent is refused. The Range-based selection is what makes
// it work on iOS Safari, where textarea.select() alone does not. Returns
// whether the copy succeeded, and restores any prior page selection so the
// user's own text selection is left untouched.
function legacyCopy(text) {
    let ok = false;
    const field = document.createElement('textarea');
    field.value = text;
    field.contentEditable = 'true';
    field.readOnly = false;
    // Keep the field in the viewport (top/left 0, 1px, transparent). Several
    // browsers refuse to copy from an element scrolled off-screen, which made the
    // copy work only sometimes on a plain-http instance where this path is the
    // only one available. Tiny and opacity 0 so it is never actually seen.
    field.style.position = 'fixed';
    field.style.top = '0';
    field.style.left = '0';
    field.style.width = '1px';
    field.style.height = '1px';
    field.style.padding = '0';
    field.style.border = 'none';
    field.style.opacity = '0';
    document.body.appendChild(field);
    const selection = window.getSelection();
    const previous = selection && selection.rangeCount > 0 ? selection.getRangeAt(0) : null;
    try {
        // Focusing the field first is what makes execCommand reliably copy from
        // it rather than from whatever was selected on the page.
        field.focus();
        const range = document.createRange();
        range.selectNodeContents(field);
        selection.removeAllRanges();
        selection.addRange(range);
        field.setSelectionRange(0, text.length);
        ok = document.execCommand('copy');
    } catch (e) {
        ok = false;
    }
    // Restore the user's selection now, but take the field itself down on the
    // next tick rather than synchronously: it is the clipboard's owner until
    // the handoff completes, and removing it in the same task has been seen to
    // leave the clipboard empty despite execCommand reporting success.
    if (selection) {
        selection.removeAllRanges();
        if (previous) selection.addRange(previous);
    }
    setTimeout(() => { if (field.parentNode) field.parentNode.removeChild(field); }, 0);
    return ok;
}

// Export devices
function exportDevices(format) {
    const rows = document.querySelectorAll('.device-row');
    const devices = [];

    rows.forEach(row => {
        if (row.style.display !== 'none') {
            devices.push({
                ip: row.dataset.ip,
                hostname: row.querySelector('.hostname-cell')?.textContent?.trim() || '',
                mac: row.dataset.mac?.toUpperCase() || '',
                vendor: row.querySelector('.vendor-cell')?.textContent?.trim() || '',
                label: row.dataset.labelOriginal || '',
                group: row.dataset.group || '',
                location: row.dataset.location || '',
                status: row.dataset.status || ''
            });
        }
    });

    let content, filename, type;

    if (format === 'csv') {
        const headers = ['IP', 'Hostname', 'MAC', 'Vendor', 'Label', 'Group', 'Location', 'Status'];
        const csvRows = [headers.join(',')];
        devices.forEach(d => {
            csvRows.push([d.ip, d.hostname, d.mac, d.vendor, d.label, d.group, d.location, d.status]
                .map(v => `"${(v || '').replace(/"/g, '""')}"`)
                .join(','));
        });
        content = csvRows.join('\n');
        filename = 'devices.csv';
        type = 'text/csv';
    } else {
        content = JSON.stringify(devices, null, 2);
        filename = 'devices.json';
        type = 'application/json';
    }

    const blob = new Blob([content], { type });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    a.click();
    URL.revokeObjectURL(url);

    toggleDropdown('export-menu');
    showToast(tn('app.export.done', devices.length), 'success');
}

// Dropdown toggle
function toggleDropdown(id) {
    const menu = document.getElementById(id);
    if (menu) {
        menu.classList.toggle('show');
        // Close on click outside
        if (menu.classList.contains('show')) {
            setTimeout(() => {
                document.addEventListener('click', function closeDropdown(e) {
                    if (!menu.contains(e.target)) {
                        menu.classList.remove('show');
                        document.removeEventListener('click', closeDropdown);
                    }
                });
            }, 0);
        }
    }
}

// Table sorting
let sortColumn = null;
let sortAsc = true;

// compareRows orders two device rows by the active column and direction. Split
// out so the sort can be re-applied after an auto-refresh without re-toggling
// the direction.
function compareRows(a, b, column) {
    let valA, valB;

    switch (column) {
        case 'ip':
            // Sort IP addresses numerically
            valA = a.dataset.ip.split('.').map(n => n.padStart(3, '0')).join('');
            valB = b.dataset.ip.split('.').map(n => n.padStart(3, '0')).join('');
            break;
        case 'hostname':
            valA = a.dataset.hostnameDisplay || '';
            valB = b.dataset.hostnameDisplay || '';
            break;
        case 'label':
        case 'group':
        case 'location':
            // The ones the user types themselves, so compare them the way the
            // user reads them: case-insensitively.
            valA = (a.dataset[column] || '').toLowerCase();
            valB = (b.dataset[column] || '').toLowerCase();
            break;
        case 'status':
            const order = { online: 0, recent: 1, offline: 2 };
            valA = order[a.dataset.status] ?? 3;
            valB = order[b.dataset.status] ?? 3;
            break;
        case 'lastseen':
            valA = parseInt(a.dataset.lastseen) || 0;
            valB = parseInt(b.dataset.lastseen) || 0;
            break;
        default:
            valA = a.dataset[column] || '';
            valB = b.dataset[column] || '';
    }

    // A device with no value for the column goes last whichever way the sort
    // runs: flipping the direction should reverse the devices that have a
    // value, not bring the blanks to the top.
    if (valA === '' && valB !== '') return 1;
    if (valB === '' && valA !== '') return -1;
    if (valA < valB) return sortAsc ? -1 : 1;
    if (valA > valB) return sortAsc ? 1 : -1;
    return 0;
}

// applyCurrentSort reorders the rows in the DOM to match the active sort. It
// does nothing when no column is sorted, so a freshly loaded (server-ordered)
// table is left as the server sent it.
function applyCurrentSort() {
    if (!sortColumn) return;
    const tbody = document.getElementById('devices-tbody');
    if (!tbody) return;
    const rows = Array.from(tbody.querySelectorAll('.device-row'));
    rows.sort((a, b) => compareRows(a, b, sortColumn));
    rows.forEach(row => tbody.appendChild(row));
}

// updateSortIndicator shows which column is sorted and in which direction. The
// arrow flips between ascending and descending; every other column shows the
// neutral glyph. The header row is not replaced on refresh, so this only needs
// to run when the sort itself changes.
function updateSortIndicator() {
    document.querySelectorAll('.table th[data-sort]').forEach(th => {
        const icon = th.querySelector('.sort-icon');
        if (th.dataset.sort === sortColumn) {
            th.classList.add('sorted');
            if (icon) icon.textContent = sortAsc ? '↑' : '↓';
        } else {
            th.classList.remove('sorted');
            if (icon) icon.textContent = '↕';
        }
    });
}

function sortTable(column) {
    if (sortColumn === column) {
        sortAsc = !sortAsc;
    } else {
        sortColumn = column;
        sortAsc = true;
    }
    applyCurrentSort();
    updateSortIndicator();
}

// Auto-refresh
//
// Refreshing must never disturb what the user is doing. Reloading the page
// would reset the scroll position, empty the search box, drop the status and
// group filters and undo any sorting, every thirty seconds. Instead the page is
// fetched in the background and only the parts that carry data are swapped in.
const AUTO_REFRESH_MS = 30000;
let autoRefreshInterval = null;

// refreshInPlace replaces the device table, the summary cards and the scan
// footer with fresh copies from the server.
//
// The markup comes from the server rather than being rebuilt here, so there is
// only ever one definition of how a device row looks.
//
// Returns false if the refresh could not be applied, which usually means the
// session has ended and the response was the login page.
async function refreshInPlace() {
    let doc;
    try {
        const response = await fetch(location.href, { credentials: 'same-origin' });
        if (!response.ok) return false;
        doc = new DOMParser().parseFromString(await response.text(), 'text/html');
    } catch (e) {
        return false;
    }

    const freshRows = doc.getElementById('devices-tbody');
    if (!freshRows) return false;

    document.getElementById('devices-tbody')?.replaceWith(freshRows);

    for (const selector of ['.stat-strip', '.table-footer', '#device-count', '#scan-time-top', '#location-options']) {
        const current = document.querySelector(selector);
        const replacement = doc.querySelector(selector);
        if (current && replacement) current.replaceWith(replacement);
    }

    // The location filter lists the places devices are in, so a place typed
    // into a device for the first time has to appear here too. The server
    // omits the dropdown when there are no locations at all, so it may need
    // adding or removing, not just refreshing. The user's current choice is
    // kept when it still exists.
    const freshLoc = doc.getElementById('location-filter');
    const curLoc = document.getElementById('location-filter');
    if (freshLoc) {
        const chosen = curLoc ? curLoc.value : 'all';
        if (curLoc) curLoc.replaceWith(freshLoc);
        else document.getElementById('group-filter')?.after(freshLoc);
        if (Array.from(freshLoc.options).some(o => o.value === chosen)) freshLoc.value = chosen;
    } else if (curLoc) {
        curLoc.remove();
    }

    // The search box, filter dropdowns and column sort are deliberately left
    // alone, so re-apply what they currently say to the rows that just arrived.
    // Without re-sorting, a live-refreshing table would jump back to server
    // order every interval, quietly undoing the user's chosen sort.
    updateRelativeTimes();
    filterDevices();
    applyCurrentSort();
    // Let the scroll-dock re-measure in case the new strip is a different height.
    window.dispatchEvent(new Event('devices-refreshed'));
    return true;
}

function startAutoRefresh() {
    if (autoRefreshInterval) return;
    autoRefreshInterval = setInterval(async () => {
        // If the refresh could not be applied the session has probably ended.
        // Reload so the user is taken to the login page rather than left
        // looking at a table that has quietly stopped updating.
        if (!(await refreshInPlace())) location.reload();
    }, AUTO_REFRESH_MS);
}

// Continuous scan is the single control for "live vs snapshot". When on, the
// server keeps scanning AND the browser auto-refreshes to show it AND the "Live"
// cue appears; when off, everything is a static snapshot. Binding the view to
// this one setting removes the confusing "scanning but frozen" combination.
// The initial state is rendered into the toggle by the server.
let continuousScan = !!(document.getElementById('continuous-scan-toggle') &&
    document.getElementById('continuous-scan-toggle').classList.contains('active'));

function stopAutoRefresh() {
    if (autoRefreshInterval) {
        clearInterval(autoRefreshInterval);
        autoRefreshInterval = null;
    }
}

// Bring the view (auto-refresh polling and the "Live" cue) in line with the
// current continuous-scan state.
function applyContinuousScanState() {
    document.documentElement.classList.toggle('live-view', continuousScan);
    if (continuousScan) startAutoRefresh();
    else stopAutoRefresh();
}

// toggleContinuousScan switches the server's background scanner on or off at
// runtime. The choice is persisted server-side, so it survives a restart and
// lets a user scan once and sit on the snapshot.
async function toggleContinuousScan() {
    const toggle = document.getElementById('continuous-scan-toggle');
    if (!toggle) return;
    const next = !continuousScan;
    try {
        const res = await api('scan/continuous', { enabled: next }, 'POST');
        if (!res || !res.success) throw apiError(res, t('app.error.requestFailed'));
    } catch (e) {
        showToast(t('app.continuous.changeFailed', { error: errorText(e) }), 'error');
        return;
    }
    continuousScan = next;
    toggle.classList.toggle('active', next);
    toggle.setAttribute('aria-checked', next ? 'true' : 'false');
    applyContinuousScanState();
    showToast(next
        ? t('app.continuous.on')
        : t('app.continuous.off'),
        'info', 5000);
}

// Start the view in whatever state the server reports.
applyContinuousScanState();

// Keyboard support for the toolbar switch (role="switch" tabindex="0").
document.addEventListener('keydown', e => {
    const sw = e.target.closest && e.target.closest('.toggle-switch[role="switch"]');
    if (sw && (e.key === 'Enter' || e.key === ' ')) {
        e.preventDefault();
        sw.click();
    }
});

// Keyboard shortcuts
document.addEventListener('keydown', e => {
    // Ignore if typing in input
    if (e.target.matches('input, textarea, select')) return;

    switch (e.key.toLowerCase()) {
        case '/':
            e.preventDefault();
            document.getElementById('device-search')?.focus();
            break;
        case 'r':
            refreshAfterScan();
            break;
        case 't':
            toggleTheme();
            break;
        // Escape is handled by a dedicated listener below so it also works
        // while focus is inside a modal input, where this handler bails out.
    }
});

// Close modal on backdrop click
document.addEventListener('click', e => {
    if (e.target.classList.contains('modal')) closeModal();
});

// Close modal on Escape
document.addEventListener('keydown', e => {
    if (e.key === 'Escape') closeModal();
});

// --- Relative times ---------------------------------------------------
//
// "3 min ago" is rendered once by the server. Left alone it freezes, so a page
// open for an hour still claims three minutes, which is exactly the false
// impression these timestamps exist to prevent. Anything carrying a
// data-relative-time attribute (a unix timestamp) is rewritten periodically.

function relativeTime(unixSeconds) {
    const diff = Math.floor(Date.now() / 1000) - unixSeconds;

    if (diff < 0) return t('app.time.justNow');
    if (diff < 60) return t('app.time.justNow');

    const minutes = Math.floor(diff / 60);
    if (minutes < 60) return tn('app.time.minAgo', minutes);

    const hours = Math.floor(diff / 3600);
    if (hours < 24) return tn('app.time.hrAgo', hours);

    const days = Math.floor(diff / 86400);
    if (days < 7) return tn('app.time.dayAgo', days);

    // Older than a week, show the date instead, matching the server format:
    // "Jan 2, 2006" in English, a numeric date in every other language.
    const date = new Date(unixSeconds * 1000);
    if ((window.I18N_LANG || 'en') !== 'en') {
        return date.getFullYear() + '-' + String(date.getMonth() + 1).padStart(2, '0') + '-' +
            String(date.getDate()).padStart(2, '0');
    }
    return date.toLocaleDateString('en-US', {
        month: 'short', day: 'numeric', year: 'numeric'
    });
}

function updateRelativeTimes() {
    document.querySelectorAll('[data-relative-time]').forEach(el => {
        const ts = parseInt(el.dataset.relativeTime, 10);
        if (!isNaN(ts) && ts > 0) {
            el.textContent = relativeTime(ts);
        }
    });
}

updateRelativeTimes();
setInterval(updateRelativeTimes, 30000);

// --- Tailscale control ------------------------------------------------

// connectTailscale brings the machine up on its tailnet.
//
// If the machine still needs to sign in, the server returns a login URL
// instead of connecting, which is shown as a link rather than trying to
// automate the browser sign-in.
async function connectTailscale() {
    const msg = document.getElementById('tailscale-message');
    if (msg) msg.textContent = t('app.tailscale.connecting');
    try {
        const result = await api('tailscale/connect', {}, 'POST');
        const data = result.data || {};

        if (data.login_url) {
            if (msg) {
                // Build the link with the DOM rather than innerHTML so the URL
                // cannot inject markup.
                // The sentence is translated whole; {link} marks where the
                // link goes, since its position differs between languages.
                const [before, after = ''] = t('app.tailscale.signInNeeded').split('{link}');
                msg.textContent = before;
                const a = document.createElement('a');
                a.href = data.login_url;
                a.target = '_blank';
                a.rel = 'noreferrer noopener';
                a.textContent = t('app.tailscale.signInLink');
                msg.appendChild(a);
                msg.appendChild(document.createTextNode(after));
            }
            return;
        }

        showToast(t('app.tailscale.connected'), 'success');
        setTimeout(() => location.reload(), 1000);
    } catch (e) {
        if (msg) msg.textContent = '';
        showToast(t('app.tailscale.connectFailed', { error: errorText(e) }), 'error');
    }
}

// disconnectTailscale takes the machine off its tailnet, after warning that
// this can cut the connection the user is reaching the dashboard through.
async function disconnectTailscale() {
    if (!confirm(t('app.tailscale.disconnectConfirm'))) return;

    try {
        await api('tailscale/disconnect', {}, 'POST');
        showToast(t('app.tailscale.disconnected'), 'warning');
        setTimeout(() => location.reload(), 1000);
    } catch (e) {
        showToast(t('app.tailscale.disconnectFailed', { error: errorText(e) }), 'error');
    }
}

// Summary strip scroll-dock: as the page scrolls and the strip's top reaches
// the header, add .strip-docked to <body>. CSS then lifts the strip to
// position: fixed in the header band (transparent + light). It snaps in the
// instant it touches, so the glass bar is never painted over the header.
//
// The whole thing is driven by a class on <body> and matched by CSS selector;
// the strip is never moved or held in a variable, so an auto-refresh that swaps
// the strip element out cannot break it. The holder reserves the strip's height
// (via --strip-h) so the page never jumps. No-ops on narrow screens and reduced
// motion.
(function initStatDock() {
    const holder = document.querySelector('.stat-strip-holder');
    const header = document.querySelector('.header');
    if (!holder || !header) return;

    const wide = window.matchMedia('(min-width: 1100px)');
    const reduce = window.matchMedia('(prefers-reduced-motion: reduce)');
    const brand = header.querySelector('.header-brand');
    const nav = header.querySelector('.header-nav');
    let touchAt = Infinity;

    // Bound the docked strip to the space between the logo and the nav, so it
    // can never slide under either. The nav widens when a "Sign out" link is
    // present (auth enabled) and shifts with the monitor width, so measure it
    // live rather than assuming a fixed layout. In a right-to-left page the
    // logo sits on the right and the nav on the left, so the gap is taken
    // between whichever of the two is nearer each edge.
    function measureDockBounds() {
        if (!brand || !nav) return;
        const gap = 20;
        const b = brand.getBoundingClientRect();
        const n = nav.getBoundingClientRect();
        const innerLeft = Math.min(b.right, n.right);
        const innerRight = Math.max(b.left, n.left);
        const left = Math.round(innerLeft + gap);
        const right = Math.round(window.innerWidth - innerRight + gap);
        document.documentElement.style.setProperty('--dock-left', Math.max(0, left) + 'px');
        document.documentElement.style.setProperty('--dock-right', Math.max(0, right) + 'px');
    }

    function measure() {
        // Reserve exactly the strip's height, and find the scroll offset at
        // which the holder's top reaches the header's bottom (the touch point).
        // Re-query the strip so a refreshed one is always measured.
        const strip = holder.querySelector('.stat-strip');
        if (strip) holder.style.setProperty('--strip-h', strip.offsetHeight + 'px');
        touchAt = Math.max(0, holder.getBoundingClientRect().top + window.scrollY - header.offsetHeight);
        measureDockBounds();
    }

    // Drop the least-critical pieces from the docked strip only as far as
    // needed to keep it on one line inside its band: the scanned/Live line
    // first, then the chips. Driven by the real overflow (scrollWidth beyond
    // the band's clientWidth), so it adapts to any monitor width and nav size
    // instead of guessing a breakpoint.
    function fitDock() {
        const strip = holder.querySelector('.stat-strip');
        if (!strip) return;
        document.body.classList.remove('dock-hide-scan', 'dock-hide-meta');
        if (strip.scrollWidth > strip.clientWidth + 1) document.body.classList.add('dock-hide-scan');
        if (strip.scrollWidth > strip.clientWidth + 1) document.body.classList.add('dock-hide-meta');
    }

    let wasDocked = false;
    function onScroll() {
        const docked = wide.matches && !reduce.matches && window.scrollY >= touchAt;
        document.body.classList.toggle('strip-docked', docked);
        if (docked && !wasDocked) fitDock();
        if (!docked) document.body.classList.remove('dock-hide-scan', 'dock-hide-meta');
        wasDocked = docked;
    }

    function remeasure() {
        // Only re-measure the reserve while undocked; a docked holder is a fixed
        // reserve. The dock bounds, though, must track the header at any width.
        if (!document.body.classList.contains('strip-docked')) measure();
        else measureDockBounds();
        onScroll();
        if (document.body.classList.contains('strip-docked')) fitDock();
    }

    measure();
    onScroll();
    window.addEventListener('scroll', onScroll, { passive: true });
    window.addEventListener('resize', remeasure);
    // An auto-refresh can swap in a strip of a different height (e.g. a flagged
    // chip appears), so re-measure the reserve and touch point after each one.
    window.addEventListener('devices-refreshed', remeasure);
})();

// Back-to-top button: a floating orangutan that appears once the page is
// scrolled past a threshold and smoothly returns to the top when clicked.
(function initScrollTop() {
    const btn = document.getElementById('scroll-top');
    if (!btn) return;

    const THRESHOLD = 300;
    const reduce = window.matchMedia('(prefers-reduced-motion: reduce)');

    // Sit the button in the bottom-right corner, always clear of the vertical
    // scrollbar. That scrollbar has real width on Linux/Windows (~15px) and
    // zero width on overlay-scrollbar systems like macOS, so measure it
    // (innerWidth includes it, clientWidth does not) and add it to the inset.
    // This keeps the button off the scrollbar on every platform and view.
    function place() {
        const scrollbar = Math.max(0, window.innerWidth - document.documentElement.clientWidth);
        btn.style.left = 'auto';
        btn.style.right = (18 + scrollbar) + 'px';
    }

    function onScroll() {
        btn.classList.toggle('visible', window.scrollY > THRESHOLD);
    }

    place();
    onScroll();
    window.addEventListener('scroll', onScroll, { passive: true });
    window.addEventListener('resize', place);
    window.addEventListener('devices-refreshed', place);

    btn.addEventListener('click', () => {
        window.scrollTo({ top: 0, behavior: reduce.matches ? 'auto' : 'smooth' });
    });
})();

// Column headers wrap at word boundaries only, and take at most two lines. A
// translated header that is a single long word ("ПРОИЗВОДИТЕЛЬ") cannot wrap,
// and hyphenating it mid-word read as broken, so instead the header's text is
// shrunk just enough for its longest word to fit the column, down to a floor.
// Below the floor the column borrows the width it still needs from a free-text
// column (Label first, then Hostname, then Group: all truncate with a tooltip
// anyway), and only if none can spare it is the word allowed to break. A
// header that still needs more than two lines is shrunk further. Re-run on
// resize and after fonts load, since both change widths.
(function initHeaderFit() {
    const table = document.getElementById('devices-table');
    if (!table) return;
    const MIN_PX = 9.5;
    const MAX_LINES = 2;
    const MIN_DONOR_PX = 48;
    const donors = ['label', 'hostname', 'group']
        .map(c => table.querySelector('thead th[data-sort="' + c + '"]')).filter(Boolean);

    // The header's own words, as the browser can wrap them: at spaces and
    // after a hyphen ("MAC-" / "Adresse").
    function words(th) {
        const text = th.firstChild && th.firstChild.nodeType === Node.TEXT_NODE ? th.firstChild.textContent : th.textContent;
        return text.trim().split(/\s+|(?<=-)/).filter(Boolean);
    }

    // Width of the longest word and height of the header wrapped at avail, at
    // its current font, measured on a hidden probe inside the header so it
    // inherits the transform and spacing. The sort arrow is a word of its own
    // that can wrap onto a line by itself, so a copy of it is part of the
    // wrapped text.
    function measure(th, avail) {
        const probe = document.createElement('span');
        probe.style.cssText = 'position:absolute;visibility:hidden;white-space:nowrap;font:inherit;letter-spacing:inherit;text-transform:inherit;line-height:inherit';
        th.appendChild(probe);
        let longest = 0;
        words(th).forEach(w => { probe.textContent = w; longest = Math.max(longest, probe.getBoundingClientRect().width); });
        probe.style.whiteSpace = 'normal';
        probe.style.display = 'block';
        probe.style.width = avail + 'px';
        probe.textContent = words(th).join(' ');
        const icon = th.querySelector('.sort-icon');
        if (icon) {
            probe.appendChild(document.createTextNode(' '));
            probe.appendChild(icon.cloneNode(true));
        }
        const height = probe.getBoundingClientRect().height;
        probe.remove();
        return { longest, height };
    }

    function padding(th) {
        const cs = getComputedStyle(th);
        return parseFloat(cs.paddingLeft) + parseFloat(cs.paddingRight);
    }

    // A fixed-layout table keeps its column widths when a header's width is
    // changed or cleared, so anything measured afterwards would still see the
    // old columns. Laying the table out again from scratch applies the change.
    function relayout() {
        table.style.display = 'none';
        void table.offsetHeight;
        table.style.display = '';
    }

    // Whether a header's text fits its column in at most MAX_LINES lines.
    function fits(th) {
        const avail = th.clientWidth - padding(th);
        const m = measure(th, avail);
        return m.longest <= avail && m.height <= MAX_LINES * parseFloat(getComputedStyle(th).fontSize) * 1.3 + 1;
    }

    // Widen th by need pixels at the expense of the first donor that can
    // spare them without its own header ceasing to fit. Both are set as
    // shares of the table, like the stylesheet's widths, so the fixed layout
    // keeps the table exactly as wide as it was. Returns whether anything
    // moved.
    function borrow(th, need) {
        const share = px => (px / table.clientWidth * 100).toFixed(2) + '%';
        for (const d of donors) {
            if (d === th || d.offsetWidth === 0 || d.clientWidth - need < MIN_DONOR_PX) continue;
            const before = [d.style.width, th.style.width];
            d.style.width = share(d.clientWidth - need);
            th.style.width = share(th.clientWidth + need);
            relayout();
            if (fits(d)) return true;
            d.style.width = before[0];
            th.style.width = before[1];
            relayout();
        }
        return false;
    }

    function fit() {
        const ths = Array.from(table.querySelectorAll('thead th'));
        ths.forEach(th => { th.style.fontSize = ''; th.style.width = ''; th.classList.remove('th-break'); });
        relayout();
        // Freeze the stylesheet's layout as explicit shares that sum to the
        // table's width, so that borrowing moves exactly the pixels asked for.
        // (The stylesheet's shares are for all eleven columns; once some are
        // hidden the browser scales the rest, which borrowing cannot predict.)
        const visible = ths.filter(th => th.offsetWidth > 0);
        const widths = visible.map(th => th.clientWidth);
        visible.forEach((th, i) => { th.style.width = (widths[i] / table.clientWidth * 100).toFixed(3) + '%'; });
        ths.forEach(th => {
            if (th.offsetWidth === 0) return; // hidden column
            const cs = getComputedStyle(th);
            const pad = parseFloat(cs.paddingLeft) + parseFloat(cs.paddingRight);
            let avail = th.clientWidth - pad;
            if (!words(th).length || avail <= 0) return;
            const base = parseFloat(cs.fontSize);
            let size = base;
            let m = measure(th, avail);
            const tooTall = () => m.height > MAX_LINES * size * 1.3 + 1;
            // First shrink: for the longest word, then for the line count.
            if (m.longest > avail) {
                size = Math.max(MIN_PX, Math.floor(base * avail / m.longest * 10) / 10);
                th.style.fontSize = size + 'px';
                m = measure(th, avail);
            }
            while (size > MIN_PX && tooTall()) {
                size = Math.max(MIN_PX, size - 0.5);
                th.style.fontSize = size + 'px';
                m = measure(th, avail);
            }
            // Then, at the floor, widen the column until it fits.
            for (let tries = 0; (m.longest > avail || tooTall()) && tries < 6; tries++) {
                const need = m.longest > avail ? Math.ceil(m.longest - avail) + 2 : 20;
                if (!borrow(th, need)) break;
                avail = th.clientWidth - pad;
                m = measure(th, avail);
            }
            if (m.longest > avail) th.classList.add('th-break');
        });
    }

    let pending = false;
    function schedule() {
        if (pending) return;
        pending = true;
        setTimeout(() => { pending = false; fit(); }, 0);
    }
    schedule();
    window.addEventListener('load', schedule);
    if (document.fonts && document.fonts.ready) document.fonts.ready.then(schedule);
    window.addEventListener('resize', schedule, { passive: true });
})();

// Instant tooltips for truncated table cells (hostname, vendor, label). The
// native `title` attribute waits ~1s before showing (and resets to that delay
// when "cold"), so those cells use this instead: one shared element at the body
// level, shown the moment the pointer enters a cell whose text is actually
// clipped. Positioned in fixed coordinates so the cell's overflow never clips it.
(function initCellTips() {
    const table = document.getElementById('devices-table');
    if (!table) return;

    const tip = document.createElement('div');
    tip.id = 'cell-tip';
    tip.setAttribute('role', 'tooltip');
    document.body.appendChild(tip);

    const SELECTOR = '.hostname-cell, .vendor-cell, .label-cell, .location-cell, .ip-cell, .mac-cell';

    function isClipped(cell) {
        return cell.scrollWidth > cell.clientWidth + 1;
    }

    function show(cell) {
        const text = (cell.textContent || '').trim();
        if (!text || !isClipped(cell)) return;

        // Match the cell's own text exactly so the reveal reads as the same text
        // un-truncating in place rather than a separate label.
        const cs = getComputedStyle(cell);
        const c = cell.getBoundingClientRect();
        tip.textContent = text;
        tip.style.fontFamily = cs.fontFamily;
        tip.style.fontSize = cs.fontSize;
        tip.style.fontWeight = cs.fontWeight;
        tip.style.fontStyle = cs.fontStyle;
        tip.style.letterSpacing = cs.letterSpacing;
        tip.style.color = cs.color;
        tip.style.paddingLeft = cs.paddingLeft;
        tip.style.paddingRight = cs.paddingRight;
        tip.style.height = Math.round(c.height) + 'px';
        tip.classList.add('show');

        // Overlay the cell and grow to the right; nudge left only if the full
        // value would run off the right edge of the window.
        const t = tip.getBoundingClientRect();
        let left = c.left;
        if (left + t.width > window.innerWidth - 6) {
            left = Math.max(6, window.innerWidth - t.width - 6);
        }
        tip.style.left = Math.round(left) + 'px';
        tip.style.top = Math.round(c.top) + 'px';
    }

    function hide() {
        tip.classList.remove('show');
    }

    // Delegated hover. mouseover/mouseout bubble, so they cover cells added by
    // an auto-refresh too. Only hide when the pointer truly leaves the cell.
    table.addEventListener('mouseover', function (e) {
        const cell = e.target.closest(SELECTOR);
        if (cell) show(cell);
    });
    table.addEventListener('mouseout', function (e) {
        const cell = e.target.closest(SELECTOR);
        if (cell && !cell.contains(e.relatedTarget)) hide();
    });
    // A stale tooltip must not linger over new content.
    window.addEventListener('scroll', hide, { passive: true });

    // Discoverability. The reveal existed but nothing said so: a clipped cell
    // looked identical to a normal one apart from the ellipsis, which reads as
    // "cut off", not "hover me" (GitHub #41). Two signals, both present only
    // when something is actually clipped, so a table whose values all fit shows
    // neither: the cell itself fades out at its right edge instead of ending in
    // an ellipsis (.is-clipped), and the footer legend gains a one-line hint.
    //
    // Re-run whenever widths can change: after load and font loading (metrics
    // shift once web fonts arrive), on resize, and after an auto-refresh swaps
    // the rows and the footer (which is why the hint is server-rendered and
    // toggled here rather than injected once).
    function markClipped() {
        let any = false;
        table.querySelectorAll(SELECTOR).forEach(cell => {
            const clipped = isClipped(cell);
            cell.classList.toggle('is-clipped', clipped);
            if (clipped) any = true;
        });
        const hint = document.getElementById('clip-hint');
        if (hint) hint.hidden = !any;
    }
    // Coalesced with a timer rather than requestAnimationFrame: a frame never
    // fires while the tab is hidden, so a dashboard opened in a background tab
    // would sit unmarked until something else nudged it. A timer runs either
    // way, and a zero delay still folds a burst of resize events into one pass.
    let markPending = false;
    function scheduleMark() {
        if (markPending) return;
        markPending = true;
        setTimeout(() => { markPending = false; markClipped(); }, 0);
    }
    scheduleMark();
    window.addEventListener('load', scheduleMark);
    if (document.fonts && document.fonts.ready) document.fonts.ready.then(scheduleMark);
    window.addEventListener('resize', scheduleMark, { passive: true });
    window.addEventListener('devices-refreshed', scheduleMark);

    // Touch has no hover. On a touch device wide enough to show the table (an
    // iPad in landscape), a tap on a clipped cell reveals it briefly. The tap
    // also copies the value, and the two do not conflict: the copy note sits at
    // the finger, the reveal overlays the cell.
    table.addEventListener('touchstart', function (e) {
        const cell = e.target.closest(SELECTOR);
        if (!cell || !isClipped(cell)) return;
        show(cell);
        setTimeout(hide, 1800);
    }, { passive: true });
})();

// Click-to-copy for value cells (IP, MAC, hostname, vendor). Driven by a
// data-copy attribute and event delegation rather than an inline onclick, so a
// value that contains quotes (a vendor name, say) can never break out of the
// handler, and rows added by an auto-refresh are covered automatically. Empty
// values (a placeholder like "Unknown") carry data-copy="" and are skipped.
//
// The target is the nearest element carrying data-copy, not just the cell, so a
// value nested inside a cell (the IPv6 address shown under the IPv4 one) copies
// itself rather than the cell's primary value.
(function initCopyCells() {
    const table = document.getElementById('devices-table');
    if (!table) return;
    table.addEventListener('click', function (e) {
        const cell = e.target.closest('[data-copy]');
        if (!cell || !table.contains(cell)) return;
        const value = cell.getAttribute('data-copy');
        if (value) copyToClipboard(value, e);
    });
})();

// Clickable summary chips: clicking "N flagged" or "N moved" filters the device
// list to just those devices (and a second click clears it). Delegated on the
// document so it keeps working after an auto-refresh swaps the strip, and while
// the strip is docked in the header.
(function initStatChipFilters() {
    document.addEventListener('click', function (e) {
        const chip = e.target.closest && e.target.closest('.stat-chip-filter[data-filter]');
        if (chip) applyStatChipFilter(chip.dataset.filter);
    });
    document.addEventListener('keydown', function (e) {
        if (e.key !== 'Enter' && e.key !== ' ') return;
        const chip = e.target.closest && e.target.closest('.stat-chip-filter[data-filter]');
        if (chip) { e.preventDefault(); applyStatChipFilter(chip.dataset.filter); }
    });
})();

// Instant tooltips for small indicators carrying a data-tip attribute (moved,
// risk, web-interface, notes). One shared bubble at the body level shown the
// moment the pointer enters the element, so there is no native `title` delay.
// Delegated on the document, so it also covers rows added by an auto-refresh.
(function initHintTips() {
    let tip = null;
    function ensure() {
        if (!tip) {
            tip = document.createElement('div');
            tip.id = 'hint-tip';
            tip.setAttribute('role', 'tooltip');
            document.body.appendChild(tip);
        }
        return tip;
    }
    function show(el) {
        const text = el.getAttribute('data-tip');
        if (!text) return;
        const t = ensure();
        t.textContent = text;
        t.classList.add('show');
        const r = el.getBoundingClientRect();
        const tr = t.getBoundingClientRect();
        let left = r.left + r.width / 2 - tr.width / 2;
        left = Math.max(6, Math.min(left, window.innerWidth - tr.width - 6));
        let top = r.top - tr.height - 8;      // above the icon by default
        if (top < 6) top = r.bottom + 8;       // drop below when no room above
        t.style.left = Math.round(left) + 'px';
        t.style.top = Math.round(top) + 'px';
    }
    function hide() {
        if (tip) tip.classList.remove('show');
    }
    document.addEventListener('mouseover', function (e) {
        const el = e.target.closest && e.target.closest('[data-tip]');
        if (el) show(el);
    });
    document.addEventListener('mouseout', function (e) {
        const el = e.target.closest && e.target.closest('[data-tip]');
        if (el && !el.contains(e.relatedTarget)) hide();
    });
    window.addEventListener('scroll', hide, { passive: true });
})();

// --- API token (Settings > API access) --------------------------------------
//
// The token is shown once, at creation, because only its hash is stored. The
// page therefore has two states: "no token" and "a token exists", and it can
// never display an existing one.

async function refreshApiTokenState() {
    const state = document.getElementById('token-state');
    if (!state) return; // not on the settings page
    try {
        const result = await api('token');
        setApiTokenState(result.data && result.data.exists);
    } catch (err) {
        state.textContent = t('app.token.unavailable');
    }
}

function setApiTokenState(exists) {
    const state = document.getElementById('token-state');
    const create = document.getElementById('token-create');
    const revoke = document.getElementById('token-revoke');
    if (!state) return;
    state.textContent = exists ? t('app.token.active') : t('app.token.notCreated');
    state.className = 'status-value' + (exists ? ' online' : '');
    if (create) create.textContent = exists ? t('app.token.replace') : t('app.token.create');
    if (revoke) revoke.style.display = exists ? '' : 'none';
}

async function createApiToken() {
    const status = document.getElementById('token-status');
    try {
        const result = await api('token', {}, 'POST');
        const reveal = document.getElementById('token-reveal');
        const value = document.getElementById('token-value');
        if (value) value.value = result.data.token;
        if (reveal) reveal.style.display = '';
        if (value) { value.focus(); value.select(); }
        setApiTokenState(true);
        if (status) status.textContent = '';
        showToast(t('app.token.created'), 'success');
    } catch (err) {
        if (status) status.textContent = errorText(err);
    }
}

async function revokeApiToken() {
    const status = document.getElementById('token-status');
    try {
        await api('token', {}, 'DELETE');
        const reveal = document.getElementById('token-reveal');
        const value = document.getElementById('token-value');
        if (reveal) reveal.style.display = 'none';
        if (value) value.value = '';
        setApiTokenState(false);
        if (status) status.textContent = '';
        showToast(t('app.token.revoked'), 'success');
    } catch (err) {
        if (status) status.textContent = errorText(err);
    }
}

document.addEventListener('DOMContentLoaded', refreshApiTokenState);
