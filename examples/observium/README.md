# Experimental Observium server-side stub (not installed)

This directory contains **reference-only** PHP for parsing the `pve_zfs_monitor`
Unix Agent application section. It is **not** validated against a live Observium
installation.

## Required Observium components (server side)

Per [Adding an Application](https://docs.observium.org/developing/add_app/):

1. `includes/polling/applications/pve_zfs_monitor.inc.php` — parse `$agent_data['app']['pve_zfs_monitor']`.
2. `includes/definitions/rrdtypes.inc.php` — RRD data sources (if graphing).
3. `html/pages/device/apps/pve_zfs_monitor.inc.php` — device UI.
4. Optional graph includes under `html/includes/graphs/application/`.

Without these, the agent section is collected but produces **no graphs or alert rules**.

## Lab verification

Do not claim production compatibility until a non-production Observium poller
ingests synthetic fixture output end-to-end.
