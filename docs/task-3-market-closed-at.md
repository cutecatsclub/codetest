# Task 3 - market ClosedAt

`ClosedAt` is an `OptionalInt64` on `Market` holding unix nanoseconds, same as `StartTime`. `GetSportEvent` already returns markets as they are stored, so it comes back without any `core.proto` change.

I added a new `MarketTransform` to set it instead of putting it in `SportTransform`, as markets aren't specific to sport and `SportTransform` skips any update that doesn't change the `EventTypeID`. When an update closes a market that doesn't have a `ClosedAt` yet it's set to the current time, including markets that are created closed. The transform never overwrites it, so a market that is re-opened and closed again keeps the time it first closed. A `Deleted` `ClosedAt` is treated as not set. Updates with no markets are skipped, and nothing is returned when there's no `ClosedAt` to set so `Update` doesn't do an extra merge.

`ClosedAt` only comes from the market's own `BettingStatus`, closing the event doesn't set it on the event's markets.

I based this branch on `master` instead of stacking it on Task 2, as it doesn't use anything from Tasks 1 or 2. Whichever of them is merged last will need `model/event.pb.go` regenerated with `gen-proto.sh`, and both sides kept in `core/cmd/core/main.go` and `core/service/implementation_test.go`.

Testing was done with Redis running (`docker compose up -d`). `TestService_IntegrationTest_MarketClosedAt` runs `Update` and `GetSportEvent` end to end, covering the first close, re-opening and closing again, and a `Deleted` `ClosedAt` (`go test ./core/service/ -run MarketClosedAt -v -count=1`). It was also run against the service with grpcurl using the example payloads. To test in Postman, send `01_new_event.json` then `06_close_market.json` to `Update` and call `GetSportEvent` with `{"EventID": "testEvent"}` after each. `H2H` should have no `ClosedAt` after 01 and a `ClosedAt` after 06. Sending 01 again re-opens it, and sending 06 again closes it with the same `ClosedAt`.
