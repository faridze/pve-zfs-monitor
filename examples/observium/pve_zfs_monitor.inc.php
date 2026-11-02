<?php
// Experimental reference poller — NOT installed by pve-zfs-monitor.
// Expects $agent_data['app']['pve_zfs_monitor'] as newline key=value text.

if (empty($agent_data['app']['pve_zfs_monitor'])) {
    return;
}

$lines = explode("\n", trim($agent_data['app']['pve_zfs_monitor']));
$fields = array();
foreach ($lines as $line) {
    $line = trim($line);
    if ($line === '' || strpos($line, '=') === false) {
        continue;
    }
    list($key, $val) = explode('=', $line, 2);
    if (isset($fields[$key])) {
        return; // duplicate field — fail closed
    }
    $fields[$key] = $val;
}

$required = array(
    'schema_version', 'collected_at', 'evaluated_at', 'host_overall',
    'worst_level', 'report_fully_healthy', 'signals_total',
);

foreach ($required as $req) {
    if (!isset($fields[$req])) {
        return;
    }
}

if ((int)$fields['schema_version'] !== 2) {
    return;
}

// Example: store latest worst level as application metric (no RRD without definitions).
$app_id = discover_app($device, 'pve_zfs_monitor');
update_application($app_id, array(
    'worst_level' => $fields['worst_level'],
    'signals_total' => (int)$fields['signals_total'],
    'report_fully_healthy' => (int)$fields['report_fully_healthy'],
));
