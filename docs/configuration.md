# Configuration

`aliyunenterprise` requires three values:

| Option | Env var (rclone convention) | Description |
|---|---|---|
| `api_key` | `RCLONE_ALIYUNENTERPRISE_API_KEY` | Aliyun Drive Enterprise API Key, `uk-...` |
| `domain_id` | `RCLONE_ALIYUNENTERPRISE_DOMAIN_ID` | enterprise domain id, e.g. `bj37789` |
| `drive_id` | `RCLONE_ALIYUNENTERPRISE_DRIVE_ID` | drive id inside the domain (team space) |

The backend also falls back to bare env vars `ALIYUN_ENTERPRISE_API_KEY`,
`ALIYUN_ENTERPRISE_DOMAIN_ID`, `ALIYUN_ENTERPRISE_DRIVE_ID` when the config
file/CLI has no value (useful for scripts and the integration test suite).

## How to get a key

1. Sign in to the Aliyun Drive Enterprise admin console (the domain owner).
2. Create an API Key (**least privilege**: grant only the target drive/space
   you intend to back up, not the whole enterprise space), set an expiry.
3. Copy the `uk-...` value. It is shown only once.

> Aliyun Drive Enterprise (企业版) only. This backend does **not** work with
> PDS Developer Edition, RAM AccessKeys, or `*.api.aliyunpds.com` endpoints.

## rclone config

```bash
rclone-aliyunenterprise config
```

Create a remote of type `aliyunenterprise` and answer:

```text
api_key    > uk-...            (obscured input)
domain_id  > bj37789
drive_id   > 101
```

Then:

```bash
rclone-aliyunenterprise lsf aliyun: -R
```

## Without a config file (env only)

```bash
export ALIYUN_ENTERPRISE_API_KEY='uk-...'
export ALIYUN_ENTERPRISE_DOMAIN_ID='bj37789'
export ALIYUN_ENTERPRISE_DRIVE_ID='101'
./rclone-aliyunenterprise lsf :aliyunenterprise: -R
```

## Advanced options

| Option | Default | Meaning |
|---|---|---|
| `hidden_trash_name` | `_aliyunenterprise_rclone_trash` | provider-private trash folder name (logical delete target) |
| `catalog_path` | `~/.cache/rclone-aliyunenterprise/catalog.json` | local safety catalog (see docs/safety.md) |
| `search_retries` | 3 | search-fallback retries before failing closed |
| `search_retry_delay` | 2s | delay between search retries |

## Secret handling

- The API key is a password option: `rclone config` obscures it; flags never
  print it.
- The backend never logs the `Authorization` header or signed URLs.
- Rotate the key immediately if it leaks; prefer short-lived keys.