// Package racingtransform supplies a racingtransformClient
package racingtransform

import (
	"context"

	"git.neds.sh/technology/pricekinetics/tools/codetest/core/transforms"
	"git.neds.sh/technology/pricekinetics/tools/codetest/model"
)

type racingtransformClient struct{}

// NewRacingTransformClient creates a new Racing transform client
func NewRacingTransformClient() transforms.TransformClient {
	return &racingtransformClient{}
}

var raceTypeMap = map[string]string{
	"horse_racing":     "Horse Racing",
	"greyhound_racing": "Greyhound Racing",
	"harness_racing":   "Harness Racing",
}

// TransformEvent performs racing specific transformation on the Event
func (t *racingtransformClient) TransformEvent(_ context.Context, partialUpdate, fullModel *model.Event) (*model.Event, error) {
	if partialUpdate.EventTypeID == nil && partialUpdate.RacingData == nil && len(partialUpdate.Markets) == 0 {
		return nil, nil // nothing racing relevant changed on this update so skip processing the event
	}

	raceType := raceTypeDelta(fullModel)
	markets := closeScratchedSelections(fullModel)
	if raceType == nil && len(markets) == 0 {
		return nil, nil // nothing to change so dont cost Update another merge
	}

	// ID must be set as MergeEvent copies the ID from the delta
	outDelta := &model.Event{ID: partialUpdate.ID, Markets: markets}
	if raceType != nil {
		outDelta.RacingData = &model.RacingEvent{RaceType: raceType}
	}

	return outDelta, nil
}

// raceTypeDelta returns the RaceType derived from the EventTypeID, or nil if it is not a known racing type or is already up to date.
// RaceType is always derived rather than sent by the feed, so it is overwritten if the EventTypeID changes.
func raceTypeDelta(event *model.Event) *model.OptionalString {
	eventTypeID := event.GetEventTypeID()
	if eventTypeID.GetDeleted() {
		return nil
	}

	raceType := raceTypeMap[eventTypeID.GetValue()]
	if raceType == "" {
		return nil // unknown or non racing type so dont change anything
	}

	current := event.GetRacingData().GetRaceType()
	if current.GetValue() == raceType && !current.GetDeleted() {
		return nil
	}

	return &model.OptionalString{Value: raceType}
}

// closeScratchedSelections returns market deltas closing any selection of a scratched runner that is not already closed.
// It checks the full event rather than the partial update so markets added after a scratching are closed too.
// Selections are never re-opened, a reinstated runner is left to the feed.
func closeScratchedSelections(event *model.Event) []*model.Market {
	scratched := make(map[string]struct{})
	for _, r := range event.GetRacingData().GetRunners() {
		if r.GetID() != "" && r.GetScratched().GetValue() && !r.GetScratched().GetDeleted() {
			scratched[r.GetID()] = struct{}{}
		}
	}
	if len(scratched) == 0 {
		return nil
	}

	var markets []*model.Market
	for _, m := range event.GetMarkets() {
		var selections []*model.Selection
		for _, s := range m.GetSelections() {
			if _, ok := scratched[s.GetID()]; !ok {
				continue
			}
			status := s.GetBettingStatus()
			if status.GetValue() == model.BettingStatus_BettingClosed && !status.GetDeleted() {
				continue
			}
			selections = append(selections, &model.Selection{
				ID:            s.GetID(),
				BettingStatus: &model.OptionalBettingStatus{Value: model.BettingStatus_BettingClosed},
			})
		}
		if len(selections) > 0 {
			markets = append(markets, &model.Market{ID: m.GetID(), Selections: selections})
		}
	}

	return markets
}

func (t *racingtransformClient) GetName() string {
	return "RacingTransform"
}
