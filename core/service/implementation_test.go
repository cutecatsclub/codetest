package service_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"git.neds.sh/technology/pricekinetics/tools/codetest/core"
	"git.neds.sh/technology/pricekinetics/tools/codetest/core/repository"
	"git.neds.sh/technology/pricekinetics/tools/codetest/core/service"
	"git.neds.sh/technology/pricekinetics/tools/codetest/core/transforms"
	"git.neds.sh/technology/pricekinetics/tools/codetest/core/transforms/racingtransform"
	"git.neds.sh/technology/pricekinetics/tools/codetest/core/transforms/sporttransform"
	"git.neds.sh/technology/pricekinetics/tools/codetest/merger"
	"git.neds.sh/technology/pricekinetics/tools/codetest/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestService_IntegrationTest_NewEvent(t *testing.T) {
	repo, err := repository.NewMongoRepository(context.Background(), "mongodb://localhost:27017", "codetest", "events")
	assert.NoError(t, err)
	defer repo.DeleteEventByID(context.Background(), "integration-test-1")
	host := &service.Service{
		Upstreams: &service.Upstreams{
			MergerClient: merger.NewInlineMergerClient(),
			Repo:         repo,
			Transforms: []transforms.TransformClient{
				sporttransform.NewSportTransformClient(),
				racingtransform.NewRacingTransformClient(),
			},
		},
	}

	output, err := host.Update(context.Background(), &core.UpdateRequest{
		Event: &model.Event{
			ID:          "integration-test-1",
			Name:        &model.OptionalString{Value: "Test event"},
			EventTypeID: &model.OptionalString{Value: "soccer"},
			StartTime:   &model.OptionalInt64{Value: 1758244443000000000}, // Friday, September 19, 2025 11:14:03 AM GMT+10:00
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, "New Event born integration-test-1", output.Message)

	output, err = host.Update(context.Background(), &core.UpdateRequest{
		Event: &model.Event{
			ID: "integration-test-1",
			Markets: []*model.Market{
				{
					ID:   "mkt01",
					Name: &model.OptionalString{Value: "New Market"},
				},
			},
		},
	})

	assert.NoError(t, err)
	assert.Equal(t, "Success", output.Message)

	final, err := host.GetSportEvent(context.Background(), &core.GetSportEventRequest{EventID: "integration-test-1"})
	assert.NoError(t, err)
	assert.Equal(t, "Test event", final.Event.Name)
	assert.Contains(t, final.Event.StartTime, "2025-09-19")
	assert.Equal(t, "soccer", final.Event.SportTypeID)
	assert.Equal(t, "Soccer", final.Event.SportName)
	assert.Equal(t, "New Market", final.Event.Markets[0].Name.Value)
}

func TestService_IntegrationTest_HiddenFlag(t *testing.T) {
	ctx := context.Background()
	const id = "integration-test-hidden"

	repo, err := repository.NewMongoRepository(ctx, "mongodb://localhost:27017", "codetest", "events")
	require.NoError(t, err)
	defer repo.DeleteEventByID(ctx, id)

	host := &service.Service{
		Upstreams: &service.Upstreams{
			MergerClient: merger.NewInlineMergerClient(),
			Repo:         repo,
			Transforms: []transforms.TransformClient{
				sporttransform.NewSportTransformClient(),
				racingtransform.NewRacingTransformClient(),
			},
		},
	}

	update := func(e *model.Event) {
		e.ID = id
		_, err := host.Update(ctx, &core.UpdateRequest{Event: e})
		require.NoError(t, err)
	}
	isHidden := func() bool {
		resp, err := host.GetSportEvent(ctx, &core.GetSportEventRequest{EventID: id})
		require.NoError(t, err)
		return resp.GetEvent().GetHidden()
	}

	update(&model.Event{Name: &model.OptionalString{Value: "Hidden flag test"}})
	assert.False(t, isHidden(), "new events are not hidden by default")

	update(&model.Event{Hidden: &model.OptionalBool{Value: true}})
	assert.True(t, isHidden(), "event can be hidden")

	update(&model.Event{Markets: []*model.Market{{ID: "mkt01", Name: &model.OptionalString{Value: "Head to Head"}}}})
	assert.True(t, isHidden(), "an unrelated update does not un-hide the event")

	update(&model.Event{Hidden: &model.OptionalBool{Value: false}})
	assert.False(t, isHidden(), "event can be shown again")
}

func TestService_IntegrationTest_RacingEvent(t *testing.T) {
	ctx := context.Background()
	const id = "integration-test-racing"
	const soccerID = "integration-test-racing-soccer"

	repo, err := repository.NewMongoRepository(ctx, "mongodb://localhost:27017", "codetest", "events")
	require.NoError(t, err)
	defer repo.DeleteEventByID(ctx, id)
	defer repo.DeleteEventByID(ctx, soccerID)

	host := &service.Service{
		Upstreams: &service.Upstreams{
			MergerClient: merger.NewInlineMergerClient(),
			Repo:         repo,
			Transforms: []transforms.TransformClient{
				sporttransform.NewSportTransformClient(),
				racingtransform.NewRacingTransformClient(),
			},
		},
	}

	update := func(e *model.Event) {
		_, err := host.Update(ctx, &core.UpdateRequest{Event: e})
		require.NoError(t, err)
	}
	getRace := func() *core.RacingEvent {
		resp, err := host.GetRacingEvent(ctx, &core.GetRacingEventRequest{EventID: id})
		require.NoError(t, err)
		return resp.GetEvent()
	}
	selectionStatus := func(race *core.RacingEvent, marketID, selectionID string) model.BettingStatus {
		for _, m := range race.GetMarkets() {
			if m.GetID() != marketID {
				continue
			}
			for _, s := range m.GetSelections() {
				if s.GetID() == selectionID {
					return s.GetBettingStatus().GetValue()
				}
			}
		}
		return model.BettingStatus_BettingUnknown
	}
	openMarket := func(marketID string) *model.Market {
		m := &model.Market{ID: marketID}
		for _, runnerID := range []string{"r1", "r2", "r10"} {
			m.Selections = append(m.Selections, &model.Selection{ID: runnerID, BettingStatus: &model.OptionalBettingStatus{Value: model.BettingStatus_BettingOpen}})
		}
		return m
	}

	update(&model.Event{
		ID:          id,
		Name:        &model.OptionalString{Value: "Flemington R7"},
		EventTypeID: &model.OptionalString{Value: "horse_racing"},
		RacingData: &model.RacingEvent{
			Venue:      &model.OptionalString{Value: "Flemington"},
			RaceNumber: &model.OptionalInt64{Value: 7},
			Runners: []*model.Runner{
				{ID: "r10", Number: &model.OptionalInt64{Value: 10}},
				{ID: "r2", Name: &model.OptionalString{Value: "Second Runner"}, Number: &model.OptionalInt64{Value: 2}, Jockey: &model.OptionalString{Value: "J Smith"}},
				{ID: "r1", Number: &model.OptionalInt64{Value: 1}},
			},
		},
		Markets: []*model.Market{openMarket("win"), openMarket("place")},
	})

	race := getRace()
	assert.Equal(t, "Horse Racing", race.RaceType, "RaceType is set from the EventTypeID")
	assert.Equal(t, "Flemington", race.Venue)
	assert.Equal(t, int64(7), race.RaceNumber)
	require.Len(t, race.Runners, 3)
	assert.Equal(t, []string{"r1", "r2", "r10"}, []string{race.Runners[0].ID, race.Runners[1].ID, race.Runners[2].ID}, "runners are ordered by Number not ID")

	update(&model.Event{ID: id, RacingData: &model.RacingEvent{Runners: []*model.Runner{{ID: "r2", Scratched: &model.OptionalBool{Value: true}}}}})
	race = getRace()
	assert.True(t, race.Runners[1].Scratched, "runner can be scratched")
	assert.Equal(t, "Second Runner", race.Runners[1].Name, "a partial runner update keeps the other runner fields")
	assert.Equal(t, model.BettingStatus_BettingClosed, selectionStatus(race, "win", "r2"), "scratched runner is closed in win")
	assert.Equal(t, model.BettingStatus_BettingClosed, selectionStatus(race, "place", "r2"), "scratched runner is closed in place")
	assert.Equal(t, model.BettingStatus_BettingOpen, selectionStatus(race, "win", "r1"), "other runners stay open")

	update(&model.Event{ID: id, Markets: []*model.Market{openMarket("quinella")}})
	assert.Equal(t, model.BettingStatus_BettingClosed, selectionStatus(getRace(), "quinella", "r2"), "a market added after a scratching is closed too")

	update(&model.Event{ID: id, RacingData: &model.RacingEvent{Runners: []*model.Runner{{ID: "r2", Jockey: &model.OptionalString{Value: "J Smith", Deleted: true}}}}})
	assert.Empty(t, getRace().Runners[1].Jockey, "Deleted values are returned empty")

	update(&model.Event{ID: id, EventTypeID: &model.OptionalString{Value: "greyhound_racing"}})
	assert.Equal(t, "Greyhound Racing", getRace().RaceType, "RaceType follows an EventTypeID change")

	update(&model.Event{ID: soccerID, EventTypeID: &model.OptionalString{Value: "soccer"}})
	for eventID, want := range map[string]codes.Code{
		"":                                codes.InvalidArgument,
		"integration-test-racing-missing": codes.NotFound,
		soccerID:                          codes.NotFound,
	} {
		_, err := host.GetRacingEvent(ctx, &core.GetRacingEventRequest{EventID: eventID})
		assert.Equal(t, want, status.Code(err), "GetRacingEvent(%q)", eventID)
	}
}

func TestService_IntegrationTest_SearchEvents(t *testing.T) {
	ctx := context.Background()
	const openID = "integration-test-search-open"
	const suspendedID = "integration-test-search-suspended"
	const noStatusID = "integration-test-search-no-status"
	const nextDayID = "integration-test-search-next-day"
	const noStartTimeID = "integration-test-search-no-start-time"
	// a day far in the future so other events in the database don't show up in the results
	const day = "2099-01-01T00:00:00+10:00"
	const nextDay = "2099-01-02T00:00:00+10:00"

	repo, err := repository.NewMongoRepository(ctx, "mongodb://localhost:27017", "codetest", "events")
	require.NoError(t, err)
	for _, id := range []string{openID, suspendedID, noStatusID, nextDayID, noStartTimeID} {
		defer repo.DeleteEventByID(ctx, id)
	}

	host := &service.Service{
		Upstreams: &service.Upstreams{
			MergerClient: merger.NewInlineMergerClient(),
			Repo:         repo,
			Transforms: []transforms.TransformClient{
				sporttransform.NewSportTransformClient(),
				racingtransform.NewRacingTransformClient(),
			},
		},
	}

	update := func(e *model.Event) {
		_, err := host.Update(ctx, &core.UpdateRequest{Event: e})
		require.NoError(t, err)
	}
	startTime := func(value string) *model.OptionalInt64 {
		parsed, err := time.Parse(time.RFC3339, value)
		require.NoError(t, err)
		return &model.OptionalInt64{Value: parsed.UnixNano()}
	}
	search := func(req *core.SearchEventsRequest) []string {
		resp, err := host.SearchEvents(ctx, req)
		require.NoError(t, err)
		ids := []string{}
		for _, e := range resp.GetEvents() {
			ids = append(ids, e.GetID())
		}
		return ids
	}
	inDay := func(req *core.SearchEventsRequest) *core.SearchEventsRequest {
		req.StartTimeFrom, req.StartTimeTo = day, nextDay
		return req
	}

	update(&model.Event{ID: nextDayID, StartTime: startTime(nextDay), BettingStatus: &model.OptionalBettingStatus{Value: model.BettingStatus_BettingOpen}})
	update(&model.Event{ID: noStatusID, StartTime: startTime("2099-01-01T14:00:00+10:00")})
	update(&model.Event{ID: suspendedID, StartTime: startTime("2099-01-01T12:00:00+10:00"), BettingStatus: &model.OptionalBettingStatus{Value: model.BettingStatus_BettingSuspended}, Hidden: &model.OptionalBool{Value: true}})
	update(&model.Event{ID: openID, StartTime: startTime("2099-01-01T10:00:00+10:00"), BettingStatus: &model.OptionalBettingStatus{Value: model.BettingStatus_BettingOpen}})
	update(&model.Event{ID: noStartTimeID, Hidden: &model.OptionalBool{Value: true}})

	assert.Equal(t, []string{openID, suspendedID, noStatusID}, search(inDay(&core.SearchEventsRequest{})), "StartTimeTo is exclusive, events without a StartTime are left out and events are ordered by StartTime")
	assert.Equal(t, []string{openID, suspendedID, noStatusID, nextDayID}, search(&core.SearchEventsRequest{StartTimeFrom: "2099-01-01T00:00:00Z", StartTimeTo: "2099-01-02T00:00:00Z"}), "the same day in UTC is 10 hours later so it includes the next day event")
	assert.Equal(t, []string{suspendedID, noStatusID, nextDayID}, search(&core.SearchEventsRequest{StartTimeFrom: "2099-01-01T11:00:00+10:00"}), "StartTimeTo can be left out")

	assert.Equal(t, []string{openID}, search(inDay(&core.SearchEventsRequest{BettingStatuses: []model.BettingStatus{model.BettingStatus_BettingOpen}})))
	assert.Equal(t, []string{openID, suspendedID}, search(inDay(&core.SearchEventsRequest{BettingStatuses: []model.BettingStatus{model.BettingStatus_BettingOpen, model.BettingStatus_BettingSuspended}})), "matches any of the BettingStatuses")
	assert.Equal(t, []string{noStatusID}, search(inDay(&core.SearchEventsRequest{BettingStatuses: []model.BettingStatus{model.BettingStatus_BettingUnknown}})), "an event with no BettingStatus is BettingUnknown")

	assert.Equal(t, []string{suspendedID}, search(inDay(&core.SearchEventsRequest{Hidden: &model.OptionalBool{Value: true}})))
	assert.Equal(t, []string{openID, noStatusID}, search(inDay(&core.SearchEventsRequest{Hidden: &model.OptionalBool{Value: false}})), "events that have never been sent Hidden are not hidden")
	assert.Empty(t, search(inDay(&core.SearchEventsRequest{BettingStatuses: []model.BettingStatus{model.BettingStatus_BettingSuspended}, Hidden: &model.OptionalBool{Value: false}})), "every criteria has to match")

	suspended := search(&core.SearchEventsRequest{BettingStatuses: []model.BettingStatus{model.BettingStatus_BettingSuspended}})
	assert.Contains(t, suspended, suspendedID, "criteria can be used without a date")
	assert.NotContains(t, suspended, openID)

	hidden := search(&core.SearchEventsRequest{Hidden: &model.OptionalBool{Value: true}})
	require.Contains(t, hidden, noStartTimeID, "an event without a StartTime is found when there is no date")
	require.Contains(t, hidden, suspendedID)
	assert.Less(t, slices.Index(hidden, noStartTimeID), slices.Index(hidden, suspendedID), "events without a StartTime are ordered first")

	update(&model.Event{ID: suspendedID, Hidden: &model.OptionalBool{Value: false}})
	assert.Equal(t, []string{openID, suspendedID, noStatusID}, search(inDay(&core.SearchEventsRequest{Hidden: &model.OptionalBool{Value: false}})), "an event can be shown again")

	update(&model.Event{ID: openID, BettingStatus: &model.OptionalBettingStatus{Value: model.BettingStatus_BettingOpen, Deleted: true}})
	update(&model.Event{ID: suspendedID, Hidden: &model.OptionalBool{Value: true, Deleted: true}})
	assert.Equal(t, []string{openID, noStatusID}, search(inDay(&core.SearchEventsRequest{BettingStatuses: []model.BettingStatus{model.BettingStatus_BettingUnknown}})), "a Deleted BettingStatus is BettingUnknown")
	assert.Empty(t, search(inDay(&core.SearchEventsRequest{BettingStatuses: []model.BettingStatus{model.BettingStatus_BettingOpen}})), "a Deleted BettingStatus no longer matches its old value")
	assert.Empty(t, search(inDay(&core.SearchEventsRequest{Hidden: &model.OptionalBool{Value: true}})), "a Deleted Hidden is not hidden")

	deletedStartTime := startTime("2099-01-01T14:00:00+10:00")
	deletedStartTime.Deleted = true
	update(&model.Event{ID: noStatusID, StartTime: deletedStartTime})
	assert.Equal(t, []string{openID, suspendedID}, search(inDay(&core.SearchEventsRequest{})), "an event with a Deleted StartTime is left out of date searches")
	assert.Contains(t, search(&core.SearchEventsRequest{BettingStatuses: []model.BettingStatus{model.BettingStatus_BettingUnknown}}), noStatusID)

	for name, req := range map[string]*core.SearchEventsRequest{
		"no criteria":                        {},
		"StartTimeFrom not RFC3339":          {StartTimeFrom: "2099-01-01"},
		"StartTimeTo without a timezone":     {StartTimeTo: "2099-01-01T00:00:00"},
		"StartTimeFrom after StartTimeTo":    {StartTimeFrom: nextDay, StartTimeTo: day},
		"StartTimeFrom equal to StartTimeTo": {StartTimeFrom: day, StartTimeTo: day},
		"unknown BettingStatus":              {BettingStatuses: []model.BettingStatus{7}},
	} {
		_, err := host.SearchEvents(ctx, req)
		assert.Equal(t, codes.InvalidArgument, status.Code(err), name)
	}
}
