# Browser profile reference

Profiles are immutable JSON templates owned by `scrape-api`. They configure a new Playwright browser context, but they never contain or receive runtime cookies, cache, local storage, downloads, or login state. Every session starts from a new temporary user-data directory.

## Storage and API behavior

- Files live in `PROFILES_DIR` and are named `<profile-name>.json`.
- The bundled `generic` definition is created automatically if it is missing.
- Files are loaded and validated at API startup; an invalid source file prevents startup instead of being silently skipped.
- `POST /v1/profiles` validates and atomically creates a new file.
- Existing names return HTTP 409. Updates and deletion are not supported in v1.
- Profile names must match `^[a-z0-9][a-z0-9-]{0,62}$`; the filename and JSON `name` must match.
- Session creation resolves the profile centrally and sends a value snapshot to the chosen worker. Later file changes cannot mutate a running context.

## Fields

| Field | Type | Rules / Playwright mapping |
| --- | --- | --- |
| `name` | string | Required; lowercase alphanumeric/hyphen name |
| `description` | string | Optional operator-facing text |
| `browser` | string | Required; currently only `chromium` |
| `userAgent` | string | Browser-context user agent |
| `locale` | string | Browser-context locale, such as `de-DE` |
| `timezoneId` | string | IANA timezone ID, such as `Europe/Berlin` |
| `viewport` | object | Width 320–7680 and height 240–4320; defaults to 1920×1080 |
| `deviceScaleFactor` | number | 0–4; omission uses Playwright's default |
| `isMobile` | boolean | Mobile meta viewport behavior |
| `hasTouch` | boolean | Touch support |
| `colorScheme` | string | `light`, `dark`, or `no-preference` |
| `reducedMotion` | string | `reduce` or `no-preference` |
| `javaScriptEnabled` | boolean | Enables/disables page JavaScript |
| `ignoreHTTPSErrors` | boolean | Ignores HTTPS certificate errors when true |
| `acceptDownloads` | boolean | Playwright download acceptance policy |
| `geolocation` | object | `latitude`, `longitude`, and optional `accuracy` |
| `permissions` | string[] | Context permissions, for example `geolocation` |
| `extraHTTPHeaders` | object | String-to-string HTTP headers |
| `proxy` | object | Required `server`; optional `bypass`, `username`, and `password` |

Unknown JSON fields are ignored by Go's decoder in v1, but they are not forwarded to Playwright. Only the typed fields above affect a browser context; arbitrary Chromium arguments are intentionally unsupported.

## Complete example

```json
{
  "name": "desktop-de",
  "description": "German desktop context behind the LAN proxy",
  "browser": "chromium",
  "userAgent": "Mozilla/5.0 ...",
  "locale": "de-DE",
  "timezoneId": "Europe/Berlin",
  "viewport": {
    "width": 1920,
    "height": 1080
  },
  "deviceScaleFactor": 1,
  "isMobile": false,
  "hasTouch": false,
  "colorScheme": "light",
  "reducedMotion": "no-preference",
  "javaScriptEnabled": true,
  "ignoreHTTPSErrors": false,
  "acceptDownloads": true,
  "geolocation": {
    "latitude": 52.52,
    "longitude": 13.405,
    "accuracy": 50
  },
  "permissions": ["geolocation"],
  "extraHTTPHeaders": {
    "x-client": "scraper"
  },
  "proxy": {
    "server": "http://proxy.lan:3128",
    "bypass": "localhost,*.lan",
    "username": "scraper",
    "password": "replace-me"
  }
}
```

Create it through the API:

```bash
curl -fsS http://localhost:8080/v1/profiles \
  -H 'content-type: application/json' \
  --data-binary @desktop-de.json
```

Profile files can contain plaintext proxy credentials. Restrict directory and backup access, and do not return these documents to untrusted callers.
