function toggleTheme() {
    const html = document.documentElement;
    const current = html.getAttribute('data-theme');
    let next = current === 'light' ? 'dark' : current === 'dark' ? 'auto' : 'light';
    html.setAttribute('data-theme', next);
    localStorage.setItem('theme', next);
}

(function() {
    const saved = localStorage.getItem('theme');
    if (saved) document.documentElement.setAttribute('data-theme', saved);
})();

function showToast(message, type = 'info') {
    const toast = document.getElementById('toast');
    if (!toast) return;
    toast.textContent = message;
    toast.className = 'toast ' + type + ' show';
    setTimeout(() => toast.classList.remove('show'), 3000);
}

async function api(action, params = {}, method = 'GET') {
    let url = `api.php?action=${action}`;
    const options = { method };
    if (method === 'GET') {
        Object.keys(params).forEach(key => url += `&${key}=${encodeURIComponent(params[key])}`);
    } else {
        options.headers = { 'Content-Type': 'application/json' };
        options.body = JSON.stringify(params);
    }
    try {
        const response = await fetch(url, options);
        if (!response.ok) {
            throw new Error(`HTTP error: ${response.status}`);
        }
        const text = await response.text();
        if (!text) {
            throw new Error('Empty response from server');
        }
        try {
            return JSON.parse(text);
        } catch (parseError) {
            console.error('JSON parse error:', parseError, 'Response:', text);
            throw new Error('Invalid JSON response from server');
        }
    } catch (error) {
        console.error('API error:', error);
        throw error;
    }
}

async function scanNetwork(cidr) {
    const progress = document.getElementById('scan-progress');
    if (progress) progress.style.display = 'flex';
    try {
        const result = await api('scan', { network: cidr });
        if (result.success) {
            showToast(`Found ${result.device_count || 0} devices`, 'success');
            setTimeout(() => location.reload(), 1000);
        } else {
            showToast(result.error || 'Scan failed', 'error');
        }
    } catch (e) {
        showToast('Scan failed: ' + e.message, 'error');
    } finally {
        if (progress) progress.style.display = 'none';
    }
}

async function scanAllNetworks() {
    const progress = document.getElementById('scan-progress');
    if (progress) progress.style.display = 'flex';
    try {
        const result = await api('scan', { network: 'all' });
        if (result.success) {
            let total = 0;
            Object.values(result.results || {}).forEach(r => { if (r.device_count) total += r.device_count; });
            showToast(`Scan complete. Found ${total} devices.`, 'success');
            setTimeout(() => location.reload(), 1000);
        } else {
            showToast('Scan failed', 'error');
        }
    } catch (e) {
        showToast('Scan failed: ' + e.message, 'error');
    } finally {
        if (progress) progress.style.display = 'none';
    }
}

function filterDevices() {
    const search = (document.getElementById('device-search')?.value || '').toLowerCase();
    const filter = document.getElementById('device-filter')?.value || 'all';
    document.querySelectorAll('.device-row').forEach(row => {
        const text = [row.dataset.ip, row.dataset.hostname, row.dataset.mac, row.dataset.vendor, row.dataset.label].join(' ');
        const matchSearch = !search || text.includes(search);
        const matchFilter = filter === 'all' || (filter === 'online' && (row.dataset.status === 'online' || row.dataset.status === 'recent')) || (filter === 'offline' && row.dataset.status === 'offline');
        row.style.display = matchSearch && matchFilter ? '' : 'none';
    });
}

function editDevice(ip) {
    const modal = document.getElementById('edit-modal');
    const row = document.querySelector(`.device-row[data-ip="${CSS.escape(ip)}"]`);
    if (!modal || !row) return;
    document.getElementById('edit-ip').value = ip;
    document.getElementById('edit-ip-display').value = ip;
    document.getElementById('edit-label').value = row.dataset.labelOriginal || '';
    document.getElementById('edit-group').value = row.querySelector('.group-select')?.value || '';
    document.getElementById('edit-notes').value = '';
    modal.style.display = 'flex';
}

function closeModal() {
    const modal = document.getElementById('edit-modal');
    if (modal) modal.style.display = 'none';
}

async function saveDevice() {
    const ip = document.getElementById('edit-ip').value;
    const label = document.getElementById('edit-label').value;
    const group = document.getElementById('edit-group').value;
    const notes = document.getElementById('edit-notes').value;
    try {
        const result = await api('device', { ip, label, group, notes }, 'POST');
        if (result.success) {
            showToast('Device updated', 'success');
            closeModal();
            location.reload();
        } else {
            showToast(result.error || 'Failed to update device', 'error');
        }
    } catch (e) {
        showToast('Error: ' + e.message, 'error');
    }
}

async function deleteDevice(ip) {
    if (!confirm(`Delete device ${ip}?`)) return;
    try {
        const response = await fetch(`api.php?action=device&ip=${encodeURIComponent(ip)}`, {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' }
        });
        const result = await response.json();
        if (result.success) {
            showToast('Device deleted', 'success');
            document.querySelector(`.device-row[data-ip="${CSS.escape(ip)}"]`)?.remove();
        } else {
            showToast(result.error || 'Failed to delete device', 'error');
        }
    } catch (e) {
        showToast('Error: ' + e.message, 'error');
    }
}

async function updateDeviceGroup(select) {
    const ip = select.dataset.ip;
    const group = select.value;
    try {
        const result = await api('device', { ip, group }, 'POST');
        if (result.success) {
            showToast('Group updated', 'success');
        } else {
            showToast(result.error || 'Failed to update group', 'error');
        }
    } catch (e) {
        showToast('Failed to update group', 'error');
    }
}

document.addEventListener('click', e => { if (e.target.classList.contains('modal')) closeModal(); });
document.addEventListener('keydown', e => { if (e.key === 'Escape') closeModal(); });
