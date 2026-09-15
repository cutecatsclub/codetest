package service_test

import (
	"context"
	"testing"

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

	repo, err := repository.NewRedisRepository(ctx, "localhost:6379", "")
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
