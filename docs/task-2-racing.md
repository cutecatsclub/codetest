# Task 2 - racing events

Racing is a new `RacingData` sub-message on `Event` next to `SportData`. It isn't a `oneof` because the Redis repository saves events with `encoding/json`, which can't decode the interface Go generates for a `oneof`. An event is a racing event when `RacingData` is set.

Runners are stored once on `RacingEvent` instead of on each `Selection`, as the same runner is in every market. `Runner.ID` matches the `Selection.ID`, so price and betting status stay on the selection. Numbers use the existing `OptionalInt64` so partial updates don't reset them. Scratched runners are flagged with `Scratched` instead of removed, as the merger can't remove from a slice.

`RacingTransform` sets `RaceType` from the `EventTypeID` (`horse_racing`, `greyhound_racing`, `harness_racing`) and updates it if the type changes, as it is only ever derived. It also closes a scratched runner's selections in every market, checking the full event so markets added later are closed too. It never re-opens a selection, a reinstated runner is left to the feed.

`GetRacingEvent` returns `InvalidArgument` for an empty `EventID`, as `Update` accepts empty IDs so it could match an event saved under an empty key. It returns `NotFound` if the event is missing or isn't a racing event. Deleted values are returned empty and runners are ordered by `Number`.

I stacked this branch on `task-1-new-field` because `RacingData = 9` follows Task 1's `Hidden = 8` on `Event`, so it should be merged after Task 1. This also lets `GetRacingEvent` return `Hidden`.

Testing was done with Redis running (`docker compose up -d`). `TestService_IntegrationTest_RacingEvent` runs `Update` and `GetRacingEvent` end to end, covering the race type, runner order, scratching across markets including a market added later, deleted values and the error codes (`go test ./core/service/ -run RacingEvent -v -count=1`). To test in Postman, send `04_new_race_event.json` then `05_scratch_runner.json` to `Update`, calling `GetRacingEvent` with `{"EventID": "testRace"}` after each. Runner `r2` should come back scratched and closed in both the Win and Place markets.
