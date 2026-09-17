# Task 4 - MongoDB

I swapped Redis for MongoDB behind the existing `Repository` interface, so nothing in the service, merger or transforms had to change. `docker-compose.yml` now runs Mongo on port 27017, with mongo-express on 8081 in place of Redis Commander.

Each event is saved as its own document in the `events` collection of the `codetest` database, using the event ID as `_id`. The generated structs only have `json` tags, so I set the driver to use those for field names. That way the documents look just like the JSON that used to go into Redis. I saved proper documents rather than a JSON string so Task 5 can search on fields like `StartTime`, `BettingStatus` and `Hidden` directly.

Saving an event replaces the whole document, the same as a Redis `SET`. Looking up an event that doesn't exist still returns nothing without an error, which `Update` relies on to create new events. The only differences I found are that a document can't be bigger than 16MB, and a `NaN` price now saves where the JSON encoding used to reject it.

`go mod tidy` removed go-redis and I ran `go mod vendor` afterwards. The vendor output is in its own commit, so it can be skipped when reviewing.

I built this on top of Task 2 so Task 5 can use both the `Hidden` flag and Mongo. Task 3 still uses the Redis repository in `main.go` and its tests, so whichever branch is merged second will need those switched to `NewMongoRepository`.

To test, start Mongo with `docker compose up -d --remove-orphans` (the flag also clears out the old Redis containers) and run `go test ./... -count=1`. The existing integration tests all pass, and the old Redis repository test now runs against Mongo. I also ran the old Redis code and the new Mongo code side by side over the example payloads and some edge cases, and they stored and returned the same data. In Postman, send the example payloads to `Update` as usual, and you can see the saved events in mongo-express at `http://localhost:8081`.
