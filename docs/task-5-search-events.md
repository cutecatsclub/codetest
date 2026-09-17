# Task 5 - SearchEvents

`SearchEvents` finds events by start time, betting status and whether they're hidden. Anything left out of the request isn't used to filter, and an event has to match everything that is sent. For example, this finds events on 18 September in AEST that are open and not hidden:

```json
{
    "StartTimeFrom": "2025-09-18T00:00:00+10:00",
    "StartTimeTo": "2025-09-19T00:00:00+10:00",
    "BettingStatuses": ["BettingOpen"],
    "Hidden": {"Value": false}
}
```

The times are RFC3339 strings, the same format `GetSportEvent` and `GetRacingEvent` already return `StartTime` in. `StartTimeFrom` is inclusive and `StartTimeTo` is exclusive, so midnight to midnight covers exactly one day, and either one can be left out. `BettingStatuses` is a list so you can search for more than one status at a time. `Hidden` is an `OptionalBool` like it is on `Event`, so leaving it out isn't the same as asking for events that aren't hidden.

The response is a list of `model.Event`s sorted by `StartTime`. I didn't use `SportEvent` or `RacingEvent` because one search can return both kinds of event. If nothing matches you get an empty list. An empty request is rejected with `InvalidArgument` rather than returning the whole database, and so are times that aren't RFC3339, a `StartTimeFrom` that isn't before `StartTimeTo`, and statuses that don't exist.

The filtering happens in the Mongo query rather than in the service. Mongo doesn't store zero values, so an event with no status counts as `BettingUnknown` and an event that was never sent `Hidden` counts as not hidden. Deleted values are treated as not set, the same as in `GetRacingEvent`, so an event with no `StartTime` won't show up in a date search. I added indexes on `StartTime`, and on `BettingStatus` with `StartTime`, so searches don't have to read every event.

This branch is stacked on Task 4 because it needs both Mongo and the `Hidden` flag from Task 1. Task 3 also adds a test to `core/service/implementation_test.go`, so whichever is merged second needs to keep both.

To run the test, start Mongo with `docker compose up -d` and run `go test ./core/service/ -run SearchEvents -v -count=1`. It covers timezones, both ends of the date range, ordering, `BettingUnknown`, `Hidden`, Deleted values, events with no `StartTime` and the error cases. I also checked it against the running service with grpcurl.

To try it in Postman:

1. Send `01_new_event.json`, `03_hide_event.json` and `04_new_race_event.json` to `Update`.
2. Search with `{"StartTimeFrom": "2025-09-18T00:00:00+10:00", "StartTimeTo": "2025-09-19T00:00:00+10:00"}`. Both `testEvent` and `testRace` come back.
3. Search with `{"Hidden": {"Value": true}}`. Only `testEvent` comes back, as 03 hid it.
4. Send `07_suspend_race.json` to `Update`. Now `{"BettingStatuses": ["BettingSuspended"]}` returns `testRace`, and `{"BettingStatuses": ["BettingOpen"]}` returns only `testEvent`.
