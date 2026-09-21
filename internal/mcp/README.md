# Miniflux MCP

The HTTP endpoint is `/mcp` (Streamable HTTP), authenticated with the
`X-Auth-Token` header containing a Miniflux API key.

## Semantic search lookback

`semantic_search` accepts an optional `lookback` alongside its existing `query`,
`limit`, `status`, and `category_id` parameters:

```json
{
  "query": "AI training data licensing",
  "lookback": "7d",
  "limit": 10
}
```

Supported units are `h`/`hour`/`hours`, `d`/`day`/`days`, `w`/`week`/`weeks`, and
`mo`/`month`/`months`. Use a positive whole number, for example `24h`, `7 days`,
`2w`, or `3mo`. Unit names are case-insensitive. Omit `lookback` to search all
time; empty, zero, negative, fractional, and unsupported values return a tool
error.

The filter uses the article's **publication time**, not when Miniflux fetched
it. Days and weeks are rolling 24-hour and 168-hour windows. Months are calendar
months in UTC, preserving the time of day and clamping to the last valid day
(for example, March 31 minus one month is February 28, or 29 in a leap year).

Only articles published after the cutoff are eligible, and the filter applies
before ranking and limiting results. It also applies when semantic search falls
back to full-text search following an embedding-service failure. Existing calls
without `lookback` keep their previous behaviour.
