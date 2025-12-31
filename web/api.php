<?php
/**
 * LAN Orangutan API
 */
class LanOrangutanAPI {
    private $configFile = '/etc/lan-orangutan/config.ini';
    private $devicesFile = '/var/lib/lan-orangutan/devices.json';
    private $scannerPath = '/opt/lan-orangutan/scanner/scan.py';
    private $utilsPath = '/opt/lan-orangutan/scanner/utils.py';

    public function __construct() {
        if (!file_exists($this->scannerPath)) {
            $this->scannerPath = __DIR__ . '/../scanner/scan.py';
            $this->utilsPath = __DIR__ . '/../scanner/utils.py';
        }
        if (!file_exists($this->configFile)) {
            $this->configFile = __DIR__ . '/../config.example.ini';
        }
        if (!file_exists(dirname($this->devicesFile))) {
            $this->devicesFile = '/tmp/lan-orangutan-devices.json';
        }
    }

    public function getConfig() {
        $defaults = [
            'server' => ['port' => 291, 'bind_address' => '0.0.0.0', 'enable_api' => true],
            'scanning' => ['scan_interval' => 300, 'min_scan_interval' => 30, 'enable_port_scan' => false],
            'storage' => ['max_devices' => 1000, 'retention_days' => 90],
            'tailscale' => ['enable' => true, 'auto_detect' => true],
            'ui' => ['theme' => 'auto'],
            'networks' => []
        ];
        if (!file_exists($this->configFile)) return $defaults;
        $config = parse_ini_file($this->configFile, true);
        if (!$config) return $defaults;
        // Deep merge: merge each section individually
        foreach ($defaults as $section => $values) {
            if (!isset($config[$section])) {
                $config[$section] = $values;
            } elseif (is_array($values)) {
                $config[$section] = array_merge($values, $config[$section]);
            }
        }
        return $config;
    }

    public function saveConfig($config) {
        $content = "";
        foreach ($config as $section => $values) {
            $content .= "[$section]\n";
            if (is_array($values)) {
                foreach ($values as $key => $value) {
                    if (is_bool($value)) $value = $value ? 'true' : 'false';
                    $content .= "$key = $value\n";
                }
            }
            $content .= "\n";
        }
        $dir = dirname($this->configFile);
        if (!is_dir($dir)) {
            if (!mkdir($dir, 0755, true)) {
                error_log("LAN Orangutan: Failed to create config directory: $dir");
                return false;
            }
        }
        return file_put_contents($this->configFile, $content) !== false;
    }

    public function getDevices() {
        if (!file_exists($this->devicesFile)) return ['devices' => [], 'networks' => []];
        $content = file_get_contents($this->devicesFile);
        $data = json_decode($content, true);
        if (!$data) {
            $backup = $this->devicesFile . '.backup';
            if (file_exists($backup)) {
                $content = file_get_contents($backup);
                $data = json_decode($content, true);
            }
        }
        return $data ?: ['devices' => [], 'networks' => []];
    }

    public function saveDevices($data) {
        $dir = dirname($this->devicesFile);
        if (!is_dir($dir)) {
            if (!mkdir($dir, 0755, true)) {
                error_log("LAN Orangutan: Failed to create devices directory: $dir");
                return false;
            }
        }
        if (file_exists($this->devicesFile)) {
            if (!copy($this->devicesFile, $this->devicesFile . '.backup')) {
                error_log("LAN Orangutan: Failed to create devices backup");
            }
        }
        return file_put_contents($this->devicesFile, json_encode($data, JSON_PRETTY_PRINT)) !== false;
    }

    private function isValidIp($ip) {
        return filter_var($ip, FILTER_VALIDATE_IP, FILTER_FLAG_IPV4) !== false;
    }

    public function getDevice($ip) {
        if (!$this->isValidIp($ip)) return null;
        $data = $this->getDevices();
        return $data['devices'][$ip] ?? null;
    }

    public function updateDevice($ip, $updates) {
        if (!$this->isValidIp($ip)) return false;
        $data = $this->getDevices();
        if (!isset($data['devices'][$ip])) return false;
        foreach ($updates as $key => $value) {
            if (in_array($key, ['label', 'notes', 'group'])) {
                $data['devices'][$ip][$key] = $value;
            }
        }
        return $this->saveDevices($data);
    }

    public function deleteDevice($ip) {
        if (!$this->isValidIp($ip)) return false;
        $data = $this->getDevices();
        if (isset($data['devices'][$ip])) {
            unset($data['devices'][$ip]);
            return $this->saveDevices($data);
        }
        return false;
    }

    public function getNetworks() {
        $networks = [];
        $utilsDir = dirname($this->utilsPath);
        $cmd = sprintf(
            'PYTHONPATH=%s python3 -c %s 2>/dev/null',
            escapeshellarg($utilsDir),
            escapeshellarg('from utils import detect_networks; import json; print(json.dumps(detect_networks()))')
        );
        $output = shell_exec($cmd);
        if ($output) {
            $detected = json_decode($output, true);
            if ($detected) $networks = $detected;
        }
        if (empty($networks)) $networks = $this->detectNetworksFallback();
        return $networks;
    }

    private function detectNetworksFallback() {
        $networks = [];
        $output = shell_exec("ip -4 addr show 2>/dev/null");
        if (!$output) return $networks;
        $lines = explode("\n", $output);
        $currentInterface = '';
        foreach ($lines as $line) {
            if (preg_match('/^\d+:\s+(\S+):/', $line, $m)) $currentInterface = $m[1];
            if (preg_match('/inet\s+(\d+\.\d+\.\d+\.\d+)\/(\d+)/', $line, $m)) {
                $ip = $m[1];
                $prefix = $m[2];
                if ($currentInterface === 'lo' || $ip === '127.0.0.1') continue;
                $ipLong = ip2long($ip);
                $mask = -1 << (32 - $prefix);
                $netLong = $ipLong & $mask;
                $network = long2ip($netLong);
                $cidr = "$network/$prefix";
                $isTailscale = strpos($currentInterface, 'tailscale') === 0;
                $isWireless = strpos($currentInterface, 'wlan') === 0 || strpos($currentInterface, 'wlp') === 0;
                $friendlyName = $isTailscale ? 'Tailscale' : ($isWireless ? "WiFi ($currentInterface)" : "LAN ($currentInterface)");
                $networks[] = ['cidr' => $cidr, 'interface' => $currentInterface, 'friendly_name' => $friendlyName, 'ip' => $ip, 'is_tailscale' => $isTailscale];
            }
        }
        return $networks;
    }

    public function scanNetwork($cidr) {
        if (!$this->isValidCidr($cidr)) {
            return ['success' => false, 'error' => 'Invalid CIDR format'];
        }
        $cmd = sprintf(
            '%s %s %s 2>&1',
            escapeshellarg('python3'),
            escapeshellarg($this->scannerPath),
            escapeshellarg($cidr)
        );
        $output = shell_exec($cmd);
        if (!$output) return ['success' => false, 'error' => 'Scanner failed'];
        $result = json_decode($output, true);
        return $result ?: ['success' => false, 'error' => 'Invalid scanner output'];
    }

    private function isValidCidr($cidr) {
        if (!preg_match('/^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})\/(\d{1,2})$/', $cidr, $matches)) {
            return false;
        }
        for ($i = 1; $i <= 4; $i++) {
            if ((int)$matches[$i] > 255) return false;
        }
        $prefix = (int)$matches[5];
        return $prefix >= 0 && $prefix <= 32;
    }

    public function scanAllNetworks() {
        $networks = $this->getNetworks();
        $results = [];
        foreach ($networks as $net) {
            $results[$net['cidr']] = $this->scanNetwork($net['cidr']);
        }
        return $results;
    }

    public function getTailscaleStatus() {
        $result = ['installed' => false, 'running' => false, 'ip' => '', 'hostname' => '', 'peers' => 0, 'exit_node' => null];
        $which = shell_exec("which tailscale 2>/dev/null");
        if (!$which) return $result;
        $result['installed'] = true;
        $output = shell_exec("tailscale status --json 2>/dev/null");
        if (!$output) return $result;
        $data = json_decode($output, true);
        if (!$data) return $result;
        $result['running'] = ($data['BackendState'] ?? '') === 'Running';
        $self = $data['Self'] ?? [];
        $result['ip'] = $self['TailscaleIPs'][0] ?? '';
        $result['hostname'] = $self['HostName'] ?? '';
        $result['peers'] = count($data['Peer'] ?? []);
        $result['exit_node'] = ($data['ExitNodeStatus'] ?? [])['ID'] ?? null;
        return $result;
    }

    public function getStats() {
        $data = $this->getDevices();
        $devices = $data['devices'] ?? [];
        $stats = ['total' => count($devices), 'online' => 0, 'new_24h' => 0];
        $now = time();
        foreach ($devices as $device) {
            $lastSeen = strtotime($device['last_seen'] ?? '');
            $firstSeen = strtotime($device['first_seen'] ?? '');
            if ($lastSeen && $lastSeen > $now - 3600) $stats['online']++;
            if ($firstSeen && $firstSeen > $now - 86400) $stats['new_24h']++;
        }
        return $stats;
    }

    public function getSystemStatus() {
        $diskFree = disk_free_space('/var/lib/lan-orangutan') ?: disk_free_space('/');
        $diskTotal = disk_total_space('/var/lib/lan-orangutan') ?: disk_total_space('/');
        return [
            'disk_free_mb' => round($diskFree / 1024 / 1024),
            'disk_total_mb' => round($diskTotal / 1024 / 1024),
            'disk_ok' => $diskFree > (100 * 1024 * 1024),
            'php_version' => PHP_VERSION,
            'scanner_available' => file_exists($this->scannerPath),
            'nmap_available' => !empty(shell_exec("which nmap 2>/dev/null"))
        ];
    }
}

if (basename($_SERVER['SCRIPT_NAME']) === 'api.php') {
    header('Content-Type: application/json');
    $api = new LanOrangutanAPI();
    $method = $_SERVER['REQUEST_METHOD'];
    $path = $_GET['action'] ?? '';

    // CSRF protection for state-changing API calls
    // Require X-Requested-With header for POST/DELETE (set by JavaScript fetch)
    $isMutatingRequest = in_array($method, ['POST', 'PUT', 'DELETE']);
    $hasXhrHeader = isset($_SERVER['HTTP_X_REQUESTED_WITH']) &&
                    strtolower($_SERVER['HTTP_X_REQUESTED_WITH']) === 'xmlhttprequest';
    $hasContentTypeJson = isset($_SERVER['CONTENT_TYPE']) &&
                          strpos($_SERVER['CONTENT_TYPE'], 'application/json') !== false;

    if ($isMutatingRequest && !$hasXhrHeader && !$hasContentTypeJson) {
        http_response_code(403);
        echo json_encode(['success' => false, 'error' => 'CSRF validation failed']);
        exit;
    }

    try {
        switch ($path) {
            case 'networks':
                echo json_encode(['success' => true, 'networks' => $api->getNetworks()]);
                break;
            case 'devices':
                echo json_encode(['success' => true, 'data' => $api->getDevices()]);
                break;
            case 'device':
                $ip = $_GET['ip'] ?? '';
                if ($method === 'GET') {
                    $device = $api->getDevice($ip);
                    echo json_encode($device ? ['success' => true, 'device' => $device] : ['success' => false, 'error' => 'Not found']);
                } elseif ($method === 'POST') {
                    $input = json_decode(file_get_contents('php://input'), true) ?: $_POST;
                    echo json_encode(['success' => $api->updateDevice($ip, $input)]);
                } elseif ($method === 'DELETE') {
                    echo json_encode(['success' => $api->deleteDevice($ip)]);
                }
                break;
            case 'scan':
                $network = $_GET['network'] ?? '';
                if ($network === 'all') {
                    echo json_encode(['success' => true, 'results' => $api->scanAllNetworks()]);
                } else {
                    echo json_encode($api->scanNetwork($network));
                }
                break;
            case 'tailscale':
                echo json_encode(['success' => true, 'data' => $api->getTailscaleStatus()]);
                break;
            case 'stats':
                echo json_encode(['success' => true, 'stats' => $api->getStats()]);
                break;
            case 'status':
                echo json_encode(['success' => true, 'status' => $api->getSystemStatus()]);
                break;
            case 'settings':
                if ($method === 'GET') {
                    echo json_encode(['success' => true, 'config' => $api->getConfig()]);
                } elseif ($method === 'POST') {
                    $input = json_decode(file_get_contents('php://input'), true) ?: $_POST;
                    echo json_encode(['success' => $api->saveConfig($input)]);
                }
                break;
            default:
                http_response_code(404);
                echo json_encode(['success' => false, 'error' => 'Unknown endpoint']);
        }
    } catch (Exception $e) {
        http_response_code(500);
        echo json_encode(['success' => false, 'error' => $e->getMessage()]);
    }
}
