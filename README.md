<pre align="center" style="color:cyan; font-weight:bold;">

██████╗   ███████╗  ███████╗ ██╗     
██╔════╝  ██╔════╝  ██╔══██╗ ██║     
██║  ███╗ ██║       ███████║ ██║     
██║   ██║ ██║       ██╔══██║ ██║     
╚██████╔╝ ╚██████╗  ██║  ██║ ███████╗
╚═════╝   ╚══════╝  ╚═╝  ╚═╝ ╚══════╝


</pre>


<p align="center">
  <b>Gcal Server</b>
</p>

Turns a casual English reminder into a Google Calendar event. Used by
[gcal-app](https://github.com/kushal1o1/gcal-app). No LLM is involved: the
phrase is parsed by a deterministic regex grammar, so the same input always
gives the same event.

## Architecture

```mermaid
sequenceDiagram
    autonumber
    participant A as Client/app
    participant S as gcal-svr
    participant G as Google Calendar
    participant D as Your phone or PC

   
    rect
    A->>S: phrase + API_TOKEN
    S->>S: parse phrase, apply TIMEZONE
    S->>G: events.insert
    G-->>S: event id
    S-->>A: event created
    end

    rect 
    Note over G,D: Google handles the rest
    G->>G: store event, schedule reminder
    G->>D: notification at reminder time
    end

    rect 
    Note over A,G: List events
    A->>S: list request
    S->>G: events.list
    G-->>S: events
    S-->>A: upcoming events
    end
```

The server keeps no database and runs no scheduler. Google Calendar stores
the events, fires the reminders, and sends the notifications.

## What you can say

People speak naturally, and the server reads the time, the repeat, the importance,
and the reminder text from one sentence:

```text
remind me in 2 hours to leave
remind me tomorrow at 9am to take meds
remind me to pay rent every month
remind me on friday to submit the report
remind me every monday to take out the bins
remind me to submit the tax form by tomorrow, it's urgent
```

See how server interprets : [docs/PHRASES.md](docs/PHRASES.md).

## Self-hosting

This server writes to **one** Google Calendar, the one shared with its service
account. Anyone who has the server URL and `API_TOKEN` can create and delete
events on that calendar.

- Run your own deployment with your own service account, calendar and token.
- Do not share a deployment URL or token publicly.

## Config

| Var | Purpose |
|---|---|
| `TIMEZONE` | IANA time zone for event times. Default `Asia/Kathmandu`. |
| `GCAL_SERVICE_ACCOUNT_B64` | base64 of the service-account JSON (used on Vercel) |
| `GCAL_SERVICE_ACCOUNT_FILE` | path to key file (local alternative) |
| `GCAL_CALENDAR_ID` | shared calendar id the service account writes to |
| `API_TOKEN` | shared secret the app sends as a Bearer token |

Local runs also auto-detect `./gcal-service-key.json`. The template is in
`.env.example`. Never commit the raw service-account key or `.env`.

## API

Full endpoint reference: [docs/API.md](docs/API.md).

## Event types

An event can be critical, recurring, both, or neither.

The phrase sets them when the event is created:

| Type | Set by the phrase | Stored in Google as |
|---|---|---|
| Critical | `urgent`, `critical`, `important`, `asap`, `emergency`, `high priority`, or a capital `Remind` | 4 reminder overrides: `email 1440`, `popup 60`, `email 0`, `popup 0` |
| Recurring | `every …`, `daily`, `weekly`, `monthly`, `yearly`, or `-A` | an RRULE on the event |
| Normal | none of the above | 1 reminder override: `popup 0` |

When events are read back, the server uses the same stored shape: more than one
reminder override means critical, and a recurrence rule means recurring.


## Tests

```bash
go test ./...
```

`internal/nlu/corpus_test.go` is a table of real phrases with their expected
output. When a phrase is parsed wrongly, add it there first.

## Related

- [gcal-app](https://github.com/kushal1o1/gcal-app): the Flutter mobile app that
  calls this server.
