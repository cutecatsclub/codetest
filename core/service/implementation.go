// Package service contains the grpc/http server implementation
package service

import (
	"context"
	"fmt"
	"time"

	"git.neds.sh/technology/pricekinetics/tools/codetest/core"
	"git.neds.sh/technology/pricekinetics/tools/codetest/core/repository"
	"git.neds.sh/technology/pricekinetics/tools/codetest/core/transforms"
	"git.neds.sh/technology/pricekinetics/tools/codetest/merger"
	"git.neds.sh/technology/pricekinetics/tools/codetest/model"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Upstreams defines dependencies the service has on other services
type Upstreams struct {
	MergerClient merger.ServiceClient
	Repo         repository.Repository
	Transforms   []transforms.TransformClient
}

// NewService creqtes a new instancxe of Service
func NewService(grpcPort, httpport int, upstreams *Upstreams) *Service {
	return &Service{GRPCPort: grpcPort, HTTPPort: httpport, Upstreams: upstreams}
}

// RegisterGRPCServerImplementations registers the grpc service contract implemented by this server
func (host *Service) RegisterGRPCServerImplementations(grpcServer *grpc.Server) {
	core.RegisterServiceServer(grpcServer, host)
}

// Update updates an Event and runs the pipeline of transformations
func (host *Service) Update(ctx context.Context, req *core.UpdateRequest) (*core.UpdateResponse, error) {
	existing, err := host.Upstreams.Repo.GetEventByID(ctx, req.GetEvent().GetID())
	if err != nil {
		logrus.WithError(err).Error("Update: failed to retrieve event")
		return nil, err
	}

	resp := &core.UpdateResponse{Message: "Success"}

	update := req.GetEvent()
	if existing == nil {
		resp.Message = fmt.Sprintf("New Event born %v", req.GetEvent().GetID())
	} else {
		update, err = host.Upstreams.MergerClient.MergeEvent(context.Background(), existing, req.GetEvent())
		if err != nil {
			logrus.WithError(err).Error("Update: failed to merge event")
			return nil, err
		}
	}

	for _, t := range host.Upstreams.Transforms {
		upd, tErr := t.TransformEvent(ctx, req.Event, update)
		if tErr != nil {
			logrus.WithError(tErr).Errorf("Update: failed to run transform %v", t.GetName())
		}
		if upd != nil {
			update, err = host.Upstreams.MergerClient.MergeEvent(context.Background(), update, upd)
			if err != nil {
				logrus.WithError(err).Errorf("Update: failed to merge event in transform %v", t.GetName())
				return nil, err
			}
		}
	}

	err = host.Upstreams.Repo.UpdateEvent(ctx, update)
	if err != nil {
		logrus.WithError(err).Error("Update: failed to update event")
		return nil, err
	}

	return resp, nil
}

// GetSportEvent retrieves a model.Event from the database and returns a core.SportEvent - this is a more UserConsumable representation of the model that is specific to sport events
func (host *Service) GetSportEvent(ctx context.Context, req *core.GetSportEventRequest) (*core.GetSportEventResponse, error) {
	existing, err := host.Upstreams.Repo.GetEventByID(ctx, req.GetEventID())
	if err != nil {
		logrus.WithError(err).Error("GetSportEvent: failed to retrieve event")
		return nil, err
	}

	resp := &core.GetSportEventResponse{}

	if existing == nil {
		return resp, nil
	}

	rslt := &core.SportEvent{}
	rslt.ConvertFromModel(existing)
	resp.Event = rslt

	return resp, nil
}

// GetRacingEvent retrieves a model.Event from the database and returns a core.RacingEvent - this is a more UserConsumable representation of the model that is specific to racing events
func (host *Service) GetRacingEvent(ctx context.Context, req *core.GetRacingEventRequest) (*core.GetRacingEventResponse, error) {
	// an empty ID is a malformed request, and Update does not reject empty IDs so it could match an event stored under an empty key
	if req.GetEventID() == "" {
		return nil, status.Error(codes.InvalidArgument, "EventID is required")
	}

	existing, err := host.Upstreams.Repo.GetEventByID(ctx, req.GetEventID())
	if err != nil {
		logrus.WithError(err).Error("GetRacingEvent: failed to retrieve event")
		return nil, status.Error(codes.Internal, "failed to retrieve event")
	}

	if existing == nil {
		return nil, status.Errorf(codes.NotFound, "event %v not found", req.GetEventID())
	}
	if existing.GetRacingData() == nil {
		return nil, status.Errorf(codes.NotFound, "event %v is not a racing event", req.GetEventID())
	}

	rslt := &core.RacingEvent{}
	rslt.ConvertFromModel(existing)

	return &core.GetRacingEventResponse{Event: rslt}, nil
}

// SearchEvents retrieves every model.Event that matches all of the criteria set on the request - at least one is required
func (host *Service) SearchEvents(ctx context.Context, req *core.SearchEventsRequest) (*core.SearchEventsResponse, error) {
	// with no criteria this would return every event in the database
	if req.GetStartTimeFrom() == "" && req.GetStartTimeTo() == "" && len(req.GetBettingStatuses()) == 0 && req.GetHidden() == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one of StartTimeFrom, StartTimeTo, BettingStatuses or Hidden is required")
	}

	filter := repository.EventFilter{BettingStatuses: req.GetBettingStatuses()}

	var err error
	if filter.StartTimeFrom, err = searchTime("StartTimeFrom", req.GetStartTimeFrom()); err != nil {
		return nil, err
	}
	if filter.StartTimeTo, err = searchTime("StartTimeTo", req.GetStartTimeTo()); err != nil {
		return nil, err
	}
	if filter.StartTimeFrom != nil && filter.StartTimeTo != nil && !filter.StartTimeFrom.Before(*filter.StartTimeTo) {
		return nil, status.Error(codes.InvalidArgument, "StartTimeFrom must be before StartTimeTo")
	}

	// proto3 enums accept any number, so reject statuses that don't exist
	for _, s := range filter.BettingStatuses {
		if _, ok := model.BettingStatus_name[int32(s)]; !ok {
			return nil, status.Errorf(codes.InvalidArgument, "unknown BettingStatus %v", s)
		}
	}

	if req.GetHidden() != nil {
		hidden := req.GetHidden().GetValue()
		filter.Hidden = &hidden
	}

	events, err := host.Upstreams.Repo.SearchEvents(ctx, filter)
	if err != nil {
		logrus.WithError(err).Error("SearchEvents: failed to search events")
		return nil, status.Error(codes.Internal, "failed to search events")
	}

	return &core.SearchEventsResponse{Events: events}, nil
}

// searchTime parses an optional RFC3339 time from a search request, the same format StartTime is returned in, returning InvalidArgument if it isn't valid
func searchTime(name string, value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v must be an RFC3339 time e.g 2025-09-19T00:00:00+10:00", name)
	}
	return &t, nil
}
