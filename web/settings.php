<?php
/**
 * LAN Orangutan - Settings Page
 */
require_once __DIR__ . '/api.php';

// CSRF token generation and validation
session_start();
if (empty($_SESSION['csrf_token'])) {
    $_SESSION['csrf_token'] = bin2hex(random_bytes(32));
}

function validateCsrfToken($token) {
    return isset($_SESSION['csrf_token']) && hash_equals($_SESSION['csrf_token'], $token);
}

function sanitizeBindAddress($value) {
    $allowed = ['0.0.0.0', '127.0.0.1'];
    return in_array($value, $allowed, true) ? $value : '0.0.0.0';
}

function sanitizeTheme($value) {
    $allowed = ['light', 'dark', 'auto'];
    return in_array($value, $allowed, true) ? $value : 'auto';
}

function sanitizePortRange($value) {
    // Only allow valid port range format: number-number or single number
    if (preg_match('/^(\d{1,5})(-(\d{1,5}))?$/', $value, $m)) {
        $start = (int)$m[1];
        $end = isset($m[3]) ? (int)$m[3] : $start;
        if ($start >= 1 && $start <= 65535 && $end >= 1 && $end <= 65535 && $start <= $end) {
            return $value;
        }
    }
    return '1-1024';
}

$api = new LanOrangutanAPI();
$config = $api->getConfig();
$networks = $api->getNetworks();
$tailscale = $api->getTailscaleStatus();
$status = $api->getSystemStatus();
$message = '';
$messageType = '';

if ($_SERVER['REQUEST_METHOD'] === 'POST' && ($_POST['action'] ?? '') === 'save_settings') {
    // Validate CSRF token
    if (!validateCsrfToken($_POST['csrf_token'] ?? '')) {
        $message = 'Invalid security token. Please try again.';
        $messageType = 'error';
    } else {
        $port = intval($_POST['port'] ?? 291);
        $port = ($port >= 1 && $port <= 65535) ? $port : 291;

        $maxDevices = intval($_POST['max_devices'] ?? 1000);
        $maxDevices = ($maxDevices >= 10 && $maxDevices <= 10000) ? $maxDevices : 1000;

        $retentionDays = intval($_POST['retention_days'] ?? 90);
        $retentionDays = ($retentionDays >= 1 && $retentionDays <= 365) ? $retentionDays : 90;

        $scanInterval = intval($_POST['scan_interval'] ?? 300);
        $allowedIntervals = [0, 300, 900, 1800, 3600];
        $scanInterval = in_array($scanInterval, $allowedIntervals, true) ? $scanInterval : 300;

        $newConfig = [
            'server' => [
                'port' => $port,
                'bind_address' => sanitizeBindAddress($_POST['bind_address'] ?? '0.0.0.0'),
                'enable_api' => isset($_POST['enable_api'])
            ],
            'scanning' => [
                'scan_interval' => $scanInterval,
                'min_scan_interval' => 30,
                'enable_port_scan' => isset($_POST['enable_port_scan']),
                'port_scan_range' => sanitizePortRange($_POST['port_scan_range'] ?? '1-1024')
            ],
            'storage' => [
                'max_devices' => $maxDevices,
                'retention_days' => $retentionDays,
                'data_dir' => '/var/lib/lan-orangutan'
            ],
            'tailscale' => [
                'enable' => isset($_POST['tailscale_enable']),
                'auto_detect' => isset($_POST['tailscale_auto_detect'])
            ],
            'ui' => ['theme' => sanitizeTheme($_POST['theme'] ?? 'auto')]
        ];
        if ($api->saveConfig($newConfig)) {
            $message = 'Settings saved successfully';
            $messageType = 'success';
            $config = $newConfig;
        } else {
            $message = 'Failed to save settings';
            $messageType = 'error';
        }
    }
}
$theme = $config['ui']['theme'] ?? 'auto';
?>
<!DOCTYPE html>
<html lang="en" data-theme="<?= htmlspecialchars($theme) ?>">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Settings - LAN Orangutan</title>
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
            <a href="index.php" class="nav-link">Dashboard</a>
            <a href="settings.php" class="nav-link active">Settings</a>
            <button class="theme-toggle" onclick="toggleTheme()" title="Toggle theme">◐</button>
        </nav>
    </header>

    <main class="main">
        <?php if ($message): ?>
        <div class="alert alert-<?= $messageType ?>"><?= htmlspecialchars($message) ?></div>
        <?php endif; ?>

        <form method="POST" class="settings-form">
            <input type="hidden" name="action" value="save_settings">
            <input type="hidden" name="csrf_token" value="<?= htmlspecialchars($_SESSION['csrf_token'], ENT_QUOTES, 'UTF-8') ?>">

            <section class="section">
                <h2 class="section-title">Server Settings</h2>
                <div class="card">
                    <div class="form-group">
                        <label for="port">Port</label>
                        <input type="number" id="port" name="port" class="input" value="<?= intval($config['server']['port'] ?? 291) ?>" min="1" max="65535">
                        <p class="form-help">Default: 291. Changing requires service restart.</p>
                    </div>
                    <div class="form-group">
                        <label for="bind_address">Bind Address</label>
                        <select id="bind_address" name="bind_address" class="select">
                            <option value="0.0.0.0" <?= ($config['server']['bind_address'] ?? '') === '0.0.0.0' ? 'selected' : '' ?>>0.0.0.0 (All interfaces)</option>
                            <option value="127.0.0.1" <?= ($config['server']['bind_address'] ?? '') === '127.0.0.1' ? 'selected' : '' ?>>127.0.0.1 (Localhost only)</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label class="checkbox-label">
                            <input type="checkbox" name="enable_api" <?= ($config['server']['enable_api'] ?? true) ? 'checked' : '' ?>>
                            Enable API
                        </label>
                    </div>
                </div>
            </section>

            <section class="section">
                <h2 class="section-title">Scanning</h2>
                <div class="card">
                    <div class="form-group">
                        <label for="scan_interval">Auto-scan Interval</label>
                        <select id="scan_interval" name="scan_interval" class="select">
                            <option value="0" <?= ($config['scanning']['scan_interval'] ?? 0) == 0 ? 'selected' : '' ?>>Manual Only</option>
                            <option value="300" <?= ($config['scanning']['scan_interval'] ?? 0) == 300 ? 'selected' : '' ?>>5 minutes</option>
                            <option value="900" <?= ($config['scanning']['scan_interval'] ?? 0) == 900 ? 'selected' : '' ?>>15 minutes</option>
                            <option value="1800" <?= ($config['scanning']['scan_interval'] ?? 0) == 1800 ? 'selected' : '' ?>>30 minutes</option>
                            <option value="3600" <?= ($config['scanning']['scan_interval'] ?? 0) == 3600 ? 'selected' : '' ?>>1 hour</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label class="checkbox-label">
                            <input type="checkbox" name="enable_port_scan" <?= ($config['scanning']['enable_port_scan'] ?? false) ? 'checked' : '' ?>>
                            Enable Port Scanning
                        </label>
                    </div>
                </div>
            </section>

            <section class="section">
                <h2 class="section-title">Networks</h2>
                <div class="card">
                    <h3 class="subsection-title">Detected Networks</h3>
                    <div class="network-list">
                        <?php foreach ($networks as $net): ?>
                        <div class="network-item">
                            <strong><?= htmlspecialchars($net['friendly_name']) ?></strong>
                            <span class="network-cidr"><?= htmlspecialchars($net['cidr']) ?></span>
                            <span class="network-interface"><?= htmlspecialchars($net['interface']) ?></span>
                        </div>
                        <?php endforeach; ?>
                    </div>
                </div>
            </section>

            <section class="section">
                <h2 class="section-title">Tailscale</h2>
                <div class="card">
                    <div class="status-row">
                        <span class="status-label">Status:</span>
                        <span class="status-value <?= $tailscale['running'] ? 'online' : ($tailscale['installed'] ? 'offline' : 'warning') ?>">
                            <?= $tailscale['running'] ? '✓ Connected' : ($tailscale['installed'] ? '✗ Disconnected' : 'Not Installed') ?>
                        </span>
                    </div>
                    <?php if ($tailscale['ip']): ?>
                    <div class="status-row">
                        <span class="status-label">IP:</span>
                        <span class="status-value"><?= htmlspecialchars($tailscale['ip']) ?></span>
                    </div>
                    <?php endif; ?>
                    <div class="form-group">
                        <label class="checkbox-label">
                            <input type="checkbox" name="tailscale_enable" <?= ($config['tailscale']['enable'] ?? true) ? 'checked' : '' ?>>
                            Enable Tailscale Integration
                        </label>
                    </div>
                </div>
            </section>

            <section class="section">
                <h2 class="section-title">Storage</h2>
                <div class="card">
                    <div class="form-group">
                        <label for="max_devices">Max Devices</label>
                        <input type="number" id="max_devices" name="max_devices" class="input" value="<?= intval($config['storage']['max_devices'] ?? 1000) ?>" min="10" max="10000">
                    </div>
                    <div class="form-group">
                        <label for="retention_days">Device Retention (days)</label>
                        <input type="number" id="retention_days" name="retention_days" class="input" value="<?= intval($config['storage']['retention_days'] ?? 90) ?>" min="1" max="365">
                    </div>
                    <div class="status-row">
                        <span class="status-label">Disk Space:</span>
                        <span class="status-value <?= $status['disk_ok'] ? 'online' : 'offline' ?>"><?= $status['disk_free_mb'] ?> MB free</span>
                    </div>
                </div>
            </section>

            <section class="section">
                <h2 class="section-title">Appearance</h2>
                <div class="card">
                    <div class="form-group">
                        <label>Theme</label>
                        <div class="radio-group">
                            <label class="radio-label"><input type="radio" name="theme" value="light" <?= ($config['ui']['theme'] ?? '') === 'light' ? 'checked' : '' ?>> Light</label>
                            <label class="radio-label"><input type="radio" name="theme" value="dark" <?= ($config['ui']['theme'] ?? '') === 'dark' ? 'checked' : '' ?>> Dark</label>
                            <label class="radio-label"><input type="radio" name="theme" value="auto" <?= ($config['ui']['theme'] ?? 'auto') === 'auto' ? 'checked' : '' ?>> Auto</label>
                        </div>
                    </div>
                </div>
            </section>

            <section class="section">
                <h2 class="section-title">System Status</h2>
                <div class="card">
                    <div class="status-row"><span class="status-label">PHP:</span><span class="status-value"><?= $status['php_version'] ?></span></div>
                    <div class="status-row"><span class="status-label">Scanner:</span><span class="status-value <?= $status['scanner_available'] ? 'online' : 'offline' ?>"><?= $status['scanner_available'] ? '✓ Available' : '✗ Not found' ?></span></div>
                    <div class="status-row"><span class="status-label">nmap:</span><span class="status-value <?= $status['nmap_available'] ? 'online' : 'warning' ?>"><?= $status['nmap_available'] ? '✓ Installed' : '⚠ Not installed' ?></span></div>
                </div>
            </section>

            <div class="form-actions">
                <button type="submit" class="btn btn-primary btn-large">Save Settings</button>
            </div>
        </form>
    </main>

    <footer class="footer">
        <p>🦧 LAN Orangutan by <a href="https://291group.com" target="_blank">291 Group</a></p>
    </footer>
    <script src="assets/app.js"></script>
</body>
</html>
