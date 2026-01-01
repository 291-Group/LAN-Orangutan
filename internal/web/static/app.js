// Theme toggle
function toggleTheme() {
    const html = document.documentElement;
    const current = html.getAttribute('data-theme');
    let next = current === 'light' ? 'dark' : current === 'dark' ? 'auto' : 'light';
    html.setAttribute('data-theme', next);
    localStorage.setItem('theme', next);
}

// Initialize theme
(function() {
    const saved = localStorage.getItem('theme');
    if (saved) document.documentElement.setAttribute('data-theme', saved);
})();

// Toast notifications
function showToast(message, type = 'info') {
    const toast = document.getElementById('toast');
    if (!toast) return;
    toast.textContent = message;
    toast.className = 'toast ' + type + ' show';
    setTimeout(() => toast.classList.remove('show'), 3000);
}

// API helper
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
        if (!response.ok) throw new Error(`HTTP error: ${response.status}`);
        const text = await response.text();
        if (!text) throw new Error('Empty response');
        return JSON.parse(text);
    } catch (error) {
        console.error('API error:', error);
        throw error;
    }
}

// Network scanning
async function scanNetwork(cidr) {
    const progress = document.getElementById('scan-progress');
    if (progress) progress.style.display = 'flex';
    try {
        const result = await api('scan', { network: cidr });
        if (result.success) {
            showToast(`Found ${result.data?.device_count || 0} devices`, 'success');
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
            showToast('Scan complete', 'success');
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

// Device filtering
function filterDevices() {
    const search = (document.getElementById('device-search')?.value || '').toLowerCase();
    const statusFilter = document.getElementById('device-filter')?.value || 'all';
    const groupFilter = document.getElementById('group-filter')?.value || 'all';

    let visible = 0;
    document.querySelectorAll('.device-row').forEach(row => {
        const text = [row.dataset.ip, row.dataset.hostname, row.dataset.mac, row.dataset.vendor, row.dataset.label].join(' ').toLowerCase();
        const status = row.dataset.status;
        const group = row.dataset.group || '';

        const matchSearch = !search || text.includes(search);
        const matchStatus = statusFilter === 'all' ||
            (statusFilter === 'online' && (status === 'online' || status === 'recent')) ||
            (statusFilter === 'offline' && status === 'offline');
        const matchGroup = groupFilter === 'all' || group === groupFilter;

        const show = matchSearch && matchStatus && matchGroup;
        row.style.display = show ? '' : 'none';
        if (show) visible++;
    });

    const countEl = document.getElementById('device-count');
    if (countEl) countEl.textContent = `Showing ${visible} devices`;
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
        const response = await fetch(`/api/device?ip=${encodeURIComponent(ip)}`, {
            method: 'DELETE',
            headers: { 'Content-Type': 'application/json' }
        });
        const result = await response.json();
        if (result.success) {
            showToast('Device deleted', 'success');
            document.querySelector(`.device-row[data-ip="${CSS.escape(ip)}"]`)?.remove();
            filterDevices(); // Update count
        } else {
            showToast(result.error || 'Failed to delete', 'error');
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
            // Update data attribute
            const row = select.closest('.device-row');
            if (row) row.dataset.group = group;
        } else {
            showToast(result.error || 'Failed to update', 'error');
        }
    } catch (e) {
        showToast('Failed to update group', 'error');
    }
}

// Copy to clipboard
function copyToClipboard(text, event) {
    navigator.clipboard.writeText(text).then(() => {
        // Show feedback at cursor position
        const feedback = document.createElement('div');
        feedback.className = 'copy-feedback';
        feedback.textContent = 'Copied!';
        feedback.style.left = event.pageX + 'px';
        feedback.style.top = (event.pageY - 30) + 'px';
        document.body.appendChild(feedback);
        setTimeout(() => feedback.remove(), 800);
    }).catch(() => {
        showToast('Failed to copy', 'error');
    });
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
                status: row.dataset.status || ''
            });
        }
    });

    let content, filename, type;

    if (format === 'csv') {
        const headers = ['IP', 'Hostname', 'MAC', 'Vendor', 'Label', 'Group', 'Status'];
        const csvRows = [headers.join(',')];
        devices.forEach(d => {
            csvRows.push([d.ip, d.hostname, d.mac, d.vendor, d.label, d.group, d.status]
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
    showToast(`Exported ${devices.length} devices`, 'success');
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

function sortTable(column) {
    const tbody = document.getElementById('devices-tbody');
    if (!tbody) return;

    if (sortColumn === column) {
        sortAsc = !sortAsc;
    } else {
        sortColumn = column;
        sortAsc = true;
    }

    const rows = Array.from(tbody.querySelectorAll('.device-row'));

    rows.sort((a, b) => {
        let valA, valB;

        switch (column) {
            case 'ip':
                // Sort IP addresses numerically
                valA = a.dataset.ip.split('.').map(n => n.padStart(3, '0')).join('');
                valB = b.dataset.ip.split('.').map(n => n.padStart(3, '0')).join('');
                break;
            case 'hostname':
                valA = a.dataset.hostname || 'zzz';
                valB = b.dataset.hostname || 'zzz';
                break;
            case 'mac':
                valA = a.dataset.mac || 'zzz';
                valB = b.dataset.mac || 'zzz';
                break;
            case 'vendor':
                valA = a.dataset.vendor || 'zzz';
                valB = b.dataset.vendor || 'zzz';
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

        if (valA < valB) return sortAsc ? -1 : 1;
        if (valA > valB) return sortAsc ? 1 : -1;
        return 0;
    });

    rows.forEach(row => tbody.appendChild(row));

    // Update sort indicators
    document.querySelectorAll('.table th').forEach(th => th.classList.remove('sorted'));
}

// Auto-refresh
let autoRefreshInterval = null;

function toggleAutoRefresh() {
    const toggle = document.getElementById('auto-refresh-toggle');
    if (!toggle) return;

    if (autoRefreshInterval) {
        clearInterval(autoRefreshInterval);
        autoRefreshInterval = null;
        toggle.classList.remove('active');
        localStorage.setItem('autoRefresh', 'false');
    } else {
        autoRefreshInterval = setInterval(() => location.reload(), 30000);
        toggle.classList.add('active');
        localStorage.setItem('autoRefresh', 'true');
        showToast('Auto-refresh enabled (30s)', 'info');
    }
}

// Initialize auto-refresh from localStorage
(function() {
    if (localStorage.getItem('autoRefresh') === 'true') {
        const toggle = document.getElementById('auto-refresh-toggle');
        if (toggle) {
            toggle.classList.add('active');
            autoRefreshInterval = setInterval(() => location.reload(), 30000);
        }
    }
})();

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
            location.reload();
            break;
        case 't':
            toggleTheme();
            break;
        case 'escape':
            closeModal();
            break;
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
