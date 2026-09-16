package service_test

import (
	"context"
	"testing"
	"time"

	"git.neds.sh/technology/pricekinetics/tools/codetest/core"
	"git.neds.sh/technology/pricekinetics/tools/codetest/core/repository"
	"git.neds.sh/technology/pricekinetics/tools/codetest/core/service"
	"git.neds.sh/technology/pricekinetics/tools/codetest/core/transforms"
	"git.neds.sh/technology/pricekinetics/tools/codetest/core/transforms/markettransform"
	"git.neds.sh/technology/pricekinetics/tools/codetest/core/transforms/sporttransform"
	"git.neds.sh/technology/pricekinetics/tools/codetest/merger"
	"git.neds.sh/technology/pricekinetics/tools/codetest/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_IntegrationTest_NewEvent(t *testing.T) {
	repo, err := repository.NewRedisRepository(context.Background(), "localhost:6379", "")
	assert.NoError(t, err)
	defer repo.DeleteEventByID(context.Background(), "integration-test-1")
	host := &service.Service{
		Upstreams: &service.Upstreams{
			MergerClient: merger.NewInlineMergerClient(),
			Repo:         repo,
			Transforms: []transforms.TransformClient{
				sporttransform.NewSportTransformClient(),
				markettransform.NewMarketTransformClient(),
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

func TestService_IntegrationTest_MarketClosedAt(t *testing.T) {
	ctx := context.Background()
	const id = "integration-test-closed-at"

	repo, err := repository.NewRedisRepository(ctx, "localhost:6379", "")
	require.NoError(t, err)
	defer repo.DeleteEventByID(ctx, id)

	host := &service.Service{
		Upstreams: &service.Upstreams{
			MergerClient: merger.NewInlineMergerClient(),
			Repo:         repo,
			Transforms: []transforms.TransformClient{
				sporttransform.NewSportTransformClient(),
				markettransform.NewMarketTransformClient(),
			},
		},
	}

	update := func(markets ...*model.Market) {
		_, err := host.Update(ctx, &core.UpdateRequest{Event: &model.Event{ID: id, Markets: markets}})
		require.NoError(t, err)
	}
	withStatus := func(marketID string, s model.BettingStatus) *model.Market {
		return &model.Market{ID: marketID, BettingStatus: &model.OptionalBettingStatus{Value: s}}
	}
	closedAt := func(marketID string) *model.OptionalInt64 {
		resp, err := host.GetSportEvent(ctx, &core.GetSportEventRequest{EventID: id})
		require.NoError(t, err)
		for _, m := range resp.GetEvent().GetMarkets() {
			if m.GetID() == marketID {
				return m.GetClosedAt()
			}
		}
		require.Failf(t, "market not found", "market %v", marketID)
		return nil
	}

	update(withStatus("h2h", model.BettingStatus_BettingOpen), withStatus("totals", model.BettingStatus_BettingClosed))
	assert.Nil(t, closedAt("h2h"), "an open market has no ClosedAt")
	assert.NotNil(t, closedAt("totals"), "a market created closed gets a ClosedAt")

	before := time.Now().UnixNano()
	update(withStatus("h2h", model.BettingStatus_BettingClosed))
	after := time.Now().UnixNano()
	first := closedAt("h2h")
	require.NotNil(t, first, "closing a market sets ClosedAt")
	assert.True(t, first.GetValue() >= before && first.GetValue() <= after, "ClosedAt is the time the market was closed")

	update(&model.Market{ID: "h2h", Name: &model.OptionalString{Value: "Head to Head"}})
	assert.Equal(t, first.GetValue(), closedAt("h2h").GetValue(), "an unrelated update keeps ClosedAt")

	update(withStatus("h2h", model.BettingStatus_BettingOpen))
	assert.Equal(t, first.GetValue(), closedAt("h2h").GetValue(), "re-opening a market keeps ClosedAt")

	time.Sleep(time.Millisecond) // let the clock move on so a new ClosedAt would be a different time
	update(withStatus("h2h", model.BettingStatus_BettingClosed))
	assert.Equal(t, first.GetValue(), closedAt("h2h").GetValue(), "closing again keeps the time it was first closed")

	update(&model.Market{ID: "line", ClosedAt: &model.OptionalInt64{Deleted: true}})
	update(withStatus("line", model.BettingStatus_BettingClosed))
	line := closedAt("line")
	assert.True(t, line.GetValue() != 0 && !line.GetDeleted(), "a Deleted ClosedAt doesn't stop a market getting one when it closes")
}
