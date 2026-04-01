---
name: ooo-nopog
description: ooo PostgreSQL storage adapter — large-scale historical data, time-range queries, bulk export, long-term retention.
user-invocable: false
---

# ooo Nopog (PostgreSQL) Reference

For full API reference, call `kb_get(name="ooo/nopog")` if the detritus MCP server is available.

PostgreSQL storage adapter for ooo. Use when data volume exceeds LevelDB capacity or when SQL queries are needed.

## When to Use
- Large-scale data (millions of records)
- Time-range queries
- Bulk export requirements
- Long-term data retention with SQL access
