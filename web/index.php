<?php
/**
 * LAN Orangutan - Network Discovery Dashboard
 */
require_once __DIR__ . '/api.php';

$api = new LanOrangutanAPI();
$config = $api->getConfig();
$devices = $api->getDevices();
$networks = $api->getNetworks();
$tailscale = $api->getTailscaleStatus();
$stats = $api->getStats();
$theme = $config['ui']['theme'] ?? 'auto';

function timeAgo($timestamp) {
    $diff = time() - $timestamp;
    if ($diff < 60) return 'Just now';
    if ($diff < 3600) return floor($diff / 60) . ' min ago';
    if ($diff < 86400) return floor($diff / 3600) . ' hr ago';
    if ($diff < 604800) return floor($diff / 86400) . ' days ago';
    return date('M j', $timestamp);
}
?>
<!DOCTYPE html>
<html lang="en" data-theme="<?= htmlspecialchars($theme) ?>">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>LAN Orangutan - Network Discovery</title>
    <link rel="stylesheet" href="assets/style.css">
    <link rel="icon" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 100 100'><text y='.9em' font-size='90'>🦧</text></svg>">
</head>
<body>
    <header class="header">
        <div class="header-brand">
            <span class="logo">🦧</span>
            <h1>LAN Orangutan</h1>
        </div>
        <nav class="header-nav">
            <a href="index.php" class="nav-link active">Dashboard</a>
            <a href="settings.php" class="nav-link">Settings</a>
            <button class="theme-toggle" onclick="toggleTheme()" title="Toggle theme">◐</button>
        </nav>
    </header>

    <main class="main">
        <section class="section">
            <h2 class="section-title">Network Status</h2>
            <div class="network-cards">
                <?php foreach ($networks as $network): ?>
                <div class="card network-card <?= $network['is_tailscale'] ? 'tailscale' : '' ?>">
                    <div class="card-header">
                        <span class="network-name"><?= htmlspecialchars($network['friendly_name']) ?></span>
                        <span class="status-badge online">●</span>
                    </div>
                    <div class="card-body">
                        <div class="network-detail">
                            <span class="label">CIDR</span>
                            <span class="value"><?= htmlspecialchars($network['cidr']) ?></span>
                        </div>
                        <div class="network-detail">
                            <span class="label">Interface</span>
                            <span class="value"><?= htmlspecialchars($network['interface']) ?></span>
                        </div>
                        <div class="network-detail">
                            <span class="label">IP</span>
                            <span class="value"><?= htmlspecialchars($network['ip']) ?></span>
                        </div>
                    </div>
                    <div class="card-footer">
                        <button class="btn btn-primary btn-sm" onclick="scanNetwork('<?= htmlspecialchars($network['cidr'], ENT_QUOTES, 'UTF-8') ?>')">Scan Now</button>
                    </div>
                </div>
                <?php endforeach; ?>

                <?php if ($tailscale['installed'] && $tailscale['running']): ?>
                <div class="card network-card tailscale">
                    <div class="card-header">
                        <span class="network-name">Tailscale</span>
                        <span class="status-badge online">●</span>
                    </div>
                    <div class="card-body">
                        <div class="network-detail">
                            <span class="label">Status</span>
                            <span class="value">✓ Connected</span>
                        </div>
                        <?php if ($tailscale['ip']): ?>
                        <div class="network-detail">
                            <span class="label">IP</span>
                            <span class="value"><?= htmlspecialchars($tailscale['ip']) ?></span>
                        </div>
                        <?php endif; ?>
                        <div class="network-detail">
                            <span class="label">Peers</span>
                            <span class="value"><?= intval($tailscale['peers']) ?></span>
                        </div>
                    </div>
                </div>
                <?php endif; ?>
            </div>
        </section>

        <section class="section">
            <div class="stats-bar">
                <div class="stat">
                    <span class="stat-value"><?= intval($stats['total']) ?></span>
                    <span class="stat-label">Total Devices</span>
                </div>
                <div class="stat">
                    <span class="stat-value online"><?= intval($stats['online']) ?></span>
                    <span class="stat-label">Online Now</span>
                </div>
                <div class="stat">
                    <span class="stat-value"><?= intval($stats['new_24h']) ?></span>
                    <span class="stat-label">New (24h)</span>
                </div>
                <div class="stat">
                    <span class="stat-value"><?= count($networks) ?></span>
                    <span class="stat-label">Networks</span>
                </div>
            </div>
        </section>

        <section class="section">
            <div class="section-header">
                <h2 class="section-title">Discovered Devices</h2>
                <div class="section-actions">
                    <input type="text" id="device-search" class="input" placeholder="Search..." oninput="filterDevices()">
                    <select id="device-filter" class="select" onchange="filterDevices()">
                        <option value="all">All</option>
                        <option value="online">Online</option>
                        <option value="offline">Offline</option>
                    </select>
                    <button class="btn btn-primary" onclick="scanAllNetworks()">Scan All</button>
                </div>
            </div>

            <div class="table-container">
                <table class="table" id="devices-table">
                    <thead>
                        <tr>
                            <th>Status</th>
                            <th>IP Address</th>
                            <th>Hostname</th>
                            <th>MAC Address</th>
                            <th>Vendor</th>
                            <th>Label</th>
                            <th>Group</th>
                            <th>Last Seen</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody id="devices-tbody">
                        <?php
                        $deviceList = $devices['devices'] ?? [];
                        uksort($deviceList, function($a, $b) {
                            return ip2long($a) <=> ip2long($b);
                        });
                        foreach ($deviceList as $ip => $device): 
                            $lastSeen = strtotime($device['last_seen'] ?? '');
                            $isOnline = $lastSeen && (time() - $lastSeen) < 3600;
                            $isRecent = $lastSeen && (time() - $lastSeen) < 300;
                            $statusClass = $isRecent ? 'online' : ($isOnline ? 'recent' : 'offline');
                        ?>
                        <tr class="device-row <?= $statusClass ?>"
                            data-ip="<?= htmlspecialchars($ip) ?>"
                            data-hostname="<?= htmlspecialchars(strtolower($device['hostname'] ?? '')) ?>"
                            data-mac="<?= htmlspecialchars(strtolower($device['mac'] ?? '')) ?>"
                            data-vendor="<?= htmlspecialchars(strtolower($device['vendor'] ?? '')) ?>"
                            data-label="<?= htmlspecialchars(strtolower($device['label'] ?? '')) ?>"
                            data-label-original="<?= htmlspecialchars($device['label'] ?? '', ENT_QUOTES, 'UTF-8') ?>"
                            data-status="<?= $statusClass ?>">
                            <td><span class="status-indicator <?= $statusClass ?>">●</span></td>
                            <td class="ip-cell"><?= htmlspecialchars($ip) ?></td>
                            <td><?= htmlspecialchars($device['hostname'] ?? '-') ?></td>
                            <td class="mac-cell"><?= htmlspecialchars($device['mac'] ?? '-') ?></td>
                            <td><?= htmlspecialchars($device['vendor'] ?? 'Unknown') ?></td>
                            <td><?= htmlspecialchars($device['label'] ?? '') ?></td>
                            <td>
                                <select class="group-select" data-ip="<?= htmlspecialchars($ip, ENT_QUOTES, 'UTF-8') ?>" onchange="updateDeviceGroup(this)">
                                    <option value="">-</option>
                                    <option value="Server" <?= ($device['group'] ?? '') === 'Server' ? 'selected' : '' ?>>Server</option>
                                    <option value="Desktop" <?= ($device['group'] ?? '') === 'Desktop' ? 'selected' : '' ?>>Desktop</option>
                                    <option value="Laptop" <?= ($device['group'] ?? '') === 'Laptop' ? 'selected' : '' ?>>Laptop</option>
                                    <option value="Mobile" <?= ($device['group'] ?? '') === 'Mobile' ? 'selected' : '' ?>>Mobile</option>
                                    <option value="IoT" <?= ($device['group'] ?? '') === 'IoT' ? 'selected' : '' ?>>IoT</option>
                                    <option value="Network" <?= ($device['group'] ?? '') === 'Network' ? 'selected' : '' ?>>Network</option>
                                    <option value="Pi" <?= ($device['group'] ?? '') === 'Pi' ? 'selected' : '' ?>>Pi</option>
                                </select>
                            </td>
                            <td class="time-cell"><?= $lastSeen ? timeAgo($lastSeen) : '-' ?></td>
                            <td class="actions-cell">
                                <button class="btn-icon" onclick="editDevice('<?= htmlspecialchars($ip, ENT_QUOTES, 'UTF-8') ?>')" title="Edit">✎</button>
                                <button class="btn-icon danger" onclick="deleteDevice('<?= htmlspecialchars($ip, ENT_QUOTES, 'UTF-8') ?>')" title="Delete">✕</button>
                            </td>
                        </tr>
                        <?php endforeach; ?>
                    </tbody>
                </table>
            </div>
            <?php if (empty($deviceList)): ?>
            <div class="empty-state">
                <p>No devices discovered yet. Click "Scan All" to discover devices.</p>
            </div>
            <?php endif; ?>
        </section>
    </main>

    <div id="edit-modal" class="modal" style="display:none">
        <div class="modal-content">
            <div class="modal-header">
                <h3>Edit Device</h3>
                <button class="modal-close" onclick="closeModal()">×</button>
            </div>
            <div class="modal-body">
                <form id="edit-form">
                    <input type="hidden" id="edit-ip" name="ip">
                    <div class="form-group">
                        <label>IP Address</label>
                        <input type="text" id="edit-ip-display" class="input" disabled>
                    </div>
                    <div class="form-group">
                        <label>Label</label>
                        <input type="text" id="edit-label" name="label" class="input" placeholder="e.g., Living Room TV">
                    </div>
                    <div class="form-group">
                        <label>Group</label>
                        <select id="edit-group" name="group" class="select">
                            <option value="">None</option>
                            <option value="Server">Server</option>
                            <option value="Desktop">Desktop</option>
                            <option value="Laptop">Laptop</option>
                            <option value="Mobile">Mobile</option>
                            <option value="IoT">IoT</option>
                            <option value="Network">Network</option>
                            <option value="Pi">Pi</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label>Notes</label>
                        <textarea id="edit-notes" name="notes" class="input" rows="3"></textarea>
                    </div>
                </form>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="closeModal()">Cancel</button>
                <button class="btn btn-primary" onclick="saveDevice()">Save</button>
            </div>
        </div>
    </div>

    <div id="toast" class="toast"></div>
    <div id="scan-progress" class="scan-progress" style="display:none">
        <div class="scan-spinner"></div>
        <span>Scanning...</span>
    </div>

    <footer class="footer">
        <p>🦧 LAN Orangutan by <a href="https://291group.com" target="_blank">291 Group</a></p>
    </footer>

    <script src="assets/app.js"></script>
</body>
</html>
