// Package core contains the proto definitions of the Core Service
package core

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"git.neds.sh/technology/pricekinetics/tools/codetest/model"
)

//go:generate ./gen-proto.sh

// ConvertFromModel converts a model.Event to a core.SportEvent
func (to *SportEvent) ConvertFromModel(model *model.Event) {
	to.ID = model.ID
	to.Name = model.GetName().GetValue()
	to.StartTime = time.Unix(0, model.StartTime.GetValue()).Format(time.RFC3339)
	to.BettingStatus = model.GetBettingStatus().GetValue().String()
	to.SportTypeID = model.GetEventTypeID().GetValue()
	to.Markets = model.Markets
	to.League = model.GetSportData().GetLeague().GetValue()
	to.SportName = model.GetSportData().GetName().GetValue()
	to.Round = model.GetSportData().GetRound().GetValue()
	to.Region = model.GetSportData().GetRegion().GetValue()
	to.Hidden = model.GetHidden().GetValue()
}

// optional is implemented by the model Optional types e.g model.OptionalString
type optional[T any] interface {
	GetValue() T
	GetDeleted() bool
}

// valueOf returns the value of an optional field, or the zero value if it is unset or Deleted
func valueOf[T any](o optional[T]) T {
	if o.GetDeleted() {
		var zero T
		return zero
	}
	return o.GetValue()
}

// ConvertFromModel converts a model.Event to a core.RacingEvent, Deleted values are returned as empty
func (to *RacingEvent) ConvertFromModel(event *model.Event) {
	race := event.GetRacingData()

	to.ID = event.GetID()
	to.Name = valueOf(event.GetName())
	if startTime := valueOf(event.GetStartTime()); startTime != 0 {
		to.StartTime = time.Unix(0, startTime).Format(time.RFC3339)
	}
	to.BettingStatus = valueOf(event.GetBettingStatus()).String()
	to.Markets = event.GetMarkets()
	to.RaceTypeID = valueOf(event.GetEventTypeID())
	to.RaceType = valueOf(race.GetRaceType())
	to.Venue = valueOf(race.GetVenue())
	to.Region = valueOf(race.GetRegion())
	to.RaceNumber = valueOf(race.GetRaceNumber())
	to.Distance = valueOf(race.GetDistance())
	to.TrackCondition = valueOf(race.GetTrackCondition())
	to.RaceClass = valueOf(race.GetRaceClass())
	to.Hidden = valueOf(event.GetHidden())

	to.Runners = make([]*Runner, 0, len(race.GetRunners()))
	for _, r := range race.GetRunners() {
		to.Runners = append(to.Runners, &Runner{
			ID:        r.GetID(),
			Name:      valueOf(r.GetName()),
			Number:    valueOf(r.GetNumber()),
			Barrier:   valueOf(r.GetBarrier()),
			Jockey:    valueOf(r.GetJockey()),
			Trainer:   valueOf(r.GetTrainer()),
			Weight:    valueOf(r.GetWeight()),
			Scratched: valueOf(r.GetScratched()),
		})
	}

	// Runners are stored in ID order, sort by Number for consumers with runners missing a Number last
	slices.SortFunc(to.Runners, func(a, b *Runner) int {
		if a.Number != b.Number {
			if a.Number == 0 {
				return 1
			}
			if b.Number == 0 {
				return -1
			}
			return cmp.Compare(a.Number, b.Number)
		}
		return strings.Compare(a.ID, b.ID)
	})
}
