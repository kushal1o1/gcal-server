# API reference

Base paths below are relative to your server URL.

## Conventions

Every response uses one envelope:

```json
{ "success": true, "message": "...", "data": { } }
```

Errors return `"success": false`, `"data": null`, and the HTTP status:

| Status | Meaning |
|---|---|
| 400 | bad request (invalid JSON, conflicting flags) |
| 401 | missing or wrong `API_TOKEN` |
| 404 | event not found |
| 422 | the phrase could not be understood |
| 500 | server configuration problem |
| 502 | Google Calendar call failed |

All endpoints except `GET /` and `GET /api/health` require:

```
Authorization: Bearer <API_TOKEN>
Content-Type: application/json
```

## `GET /`

Public HTML landing page. No auth.

## `GET /api/health`

```json
{ "success": true, "message": "ok", "data": { "service": "gcal-svr" } }
```

## `POST /api/remind`

Turns a phrase into an event.

Request:

```json
{ "text": "remind me in 2 hours 30 minutes to leave", "create": false }
```

- `create: false`: preview only. Nothing is written, and Google credentials are not needed.
- `create: true`: writes the event to the calendar.

Response:

```json
{
  "success": true,
  "message": "Event created",
  "data": {
    "summary": "leave",
    "starts_at": "2026-11-15 8:36 PM",
    "starts_at_iso": "2026-11-15T20:36:24+05:45",
    "critical": false,
    "recurring": false,
    "amount": 150,
    "unit": "MIN",
    "event_id": "abc123",
    "created": true
  }
}
```

- `event_id` and `created` appear only when `create` is `true`.

## `GET /api/events`

Lists upcoming events.

Query parameters:

| Name | Default | Notes |
|---|---|---|
| `limit` | 5 | max 500 |
| `recurring_only` | false | master recurring events only, one per series |
| `critical_only` | false | critical events only |
| `normal_only` | false | non-critical events only; cannot combine with `critical_only` (400) |

Behavior:

- By default a recurring series appears once, at its next occurrence.
- `has_more` is `true` only when `limit` was filled, so a client can show "load more" exactly when needed.
- Responses are cached in memory for 15 seconds. Any create, edit, or delete clears the cache.

```json
{
  "success": true,
  "message": "5 events found",
  "data": {
    "count": 5,
    "has_more": true,
    "events": [
      {
        "id": "instance_20260925T144500Z",
        "master_id": "master_id",
        "summary": "pay rent",
        "critical": false,
        "recurring": true,
        "starts_at": "2026-09-25 8:30 PM",
        "starts_at_iso": "2026-09-25T20:30:00+05:45",
        "time": "8:30 PM, 25-Sep",
        "recurrence_display": "Daily",
        "reminders": [ { "method": "popup", "minutes": 0 } ]
      }
    ]
  }
}
```

- `id` is the single occurrence. `master_id` is the whole series.
- `reminders` lists the event's reminder overrides. The critical profile is `email 1440`, `popup 60`, `email 0`, `popup 0`. The normal profile is `popup 0`. The field is absent when the event uses the calendar default.

## `POST /api/events/delete`

```json
{ "id": "master_id", "delete_series": true }
```

For a recurring series, pass the `master_id` and set `delete_series` to `true`. Returns 404 if the event does not exist.

## `POST /api/events/bulk-delete`

```json
{ "kind": "recurring" }
```

`kind` is `recurring` (every master recurring series) or `critical` (every critical event). Each series is deleted once, through its master id.

```json
{
  "success": true,
  "message": "Deleted 2 recurring events",
  "data": {
    "deleted": 2,
    "failed": 0,
    "items": [ { "summary": "pay rent", "series": true, "ok": true } ]
  }
}
```

## `POST /api/events/edit`

Replaces an event's reminder overrides.

```json
{
  "id": "master_id",
  "reminders": [
    { "method": "email", "minutes": 1440 },
    { "method": "popup", "minutes": 60 },
    { "method": "email", "minutes": 0 },
    { "method": "popup", "minutes": 0 }
  ]
}
```

- `id` may be an occurrence (changes only that one) or a `master_id` (changes the whole series).
- `"use_default": true` in place of `reminders` restores the calendar default and clears overrides.
- Returns 404 if the event does not exist.
