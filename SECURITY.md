# Security policy

This repository is at an early, unverified stage. Do not install it on production nodes until a reviewed and tested release is available.

Report vulnerabilities privately through GitHub Security Advisories if enabled, or contact the maintainer privately. Do not disclose credentials, customer data, hostnames, private IPs, drive serial numbers, SMTP passwords or live telemetry in public issues.

The program must not mutate storage or auto-repair ZFS. Notification integrations remain opt-in and require explicitly documented data handling. Tests must use synthetic fixtures, not production devices or real email recipients.

Public fork pull requests must only run in least-privilege GitHub-hosted CI with no secrets. Dedicated signing runners must never execute code from untrusted pull requests.
