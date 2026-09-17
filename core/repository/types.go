package repository

import (
	"context"
	"time"

	"git.neds.sh/technology/pricekinetics/tools/codetest/model"
)

// Repository is an interface for something that can Retrieve, Update and remove Events from a persistance layer
type Repository interface {
	HealthCheck(ctx context.Context) bool
	GetEventByID(ctx context.Context, id string) (*model.Event, error)
	UpdateEvent(ctx context.Context, event *model.Event) error
	DeleteEventByID(ctx context.Context, id string) error
	SearchEvents(ctx context.Context, filter EventFilter) ([]*model.Event, error)
}

// EventFilter is the criteria for SearchEvents, an event has to match everything that is set and anything left empty matches any event
type EventFilter struct {
	StartTimeFrom   *time.Time            // inclusive
	StartTimeTo     *time.Time            // exclusive
	BettingStatuses []model.BettingStatus // matches any of these, an event with no BettingStatus is BettingUnknown
	Hidden          *bool                 // an event that has never been sent Hidden is not hidden
}
