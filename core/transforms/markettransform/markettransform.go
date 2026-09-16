// Package markettransform supplies a markettransformClient
package markettransform

import (
	"context"
	"time"

	"git.neds.sh/technology/pricekinetics/tools/codetest/core/transforms"
	"git.neds.sh/technology/pricekinetics/tools/codetest/model"
)

type markettransformClient struct{}

// NewMarketTransformClient creates a new Market transform client
func NewMarketTransformClient() transforms.TransformClient {
	return &markettransformClient{}
}

// TransformEvent performs market transformations on the Event, these apply to every event type
func (t *markettransformClient) TransformEvent(_ context.Context, partialUpdate, fullModel *model.Event) (*model.Event, error) {
	if len(partialUpdate.Markets) == 0 {
		return nil, nil // no markets changed on this update so skip processing the event
	}

	markets := closedAtDeltas(partialUpdate, fullModel, time.Now().UnixNano())
	if len(markets) == 0 {
		return nil, nil // nothing to change so dont cost Update another merge
	}

	// ID must be set as MergeEvent copies the ID from the delta
	return &model.Event{ID: partialUpdate.ID, Markets: markets}, nil
}

// closedAtDeltas returns market deltas setting ClosedAt on each market this update closed that has no ClosedAt, a Deleted ClosedAt counts as none.
// The transform never clears or overwrites a ClosedAt, so a market that is re-opened and closed again keeps the time it was first closed.
func closedAtDeltas(partialUpdate, fullModel *model.Event, closedAt int64) []*model.Market {
	closing := make(map[string]struct{})
	for _, m := range partialUpdate.GetMarkets() {
		status := m.GetBettingStatus()
		if status.GetValue() == model.BettingStatus_BettingClosed && !status.GetDeleted() {
			closing[m.GetID()] = struct{}{}
		}
	}
	if len(closing) == 0 {
		return nil
	}

	var markets []*model.Market
	for _, m := range fullModel.GetMarkets() {
		if _, ok := closing[m.GetID()]; !ok {
			continue
		}
		if existing := m.GetClosedAt(); existing != nil && !existing.GetDeleted() {
			continue // closed before so keep the first ClosedAt
		}
		markets = append(markets, &model.Market{ID: m.GetID(), ClosedAt: &model.OptionalInt64{Value: closedAt}})
	}

	return markets
}

func (t *markettransformClient) GetName() string {
	return "MarketTransform"
}
