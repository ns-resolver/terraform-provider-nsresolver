## 1.0.0 (November 10, 2026)

First release.

- Resources: `nsresolver_zone` (zone and its settings), `nsresolver_record` (A, AAAA, CNAME, MX,
  TXT, SRV, CAA, NS).
- Data sources: `nsresolver_zone`, `nsresolver_records`.
- Import of zones and records.
- Retries on rate limiting (honours `Retry-After`).
