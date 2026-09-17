// Package repository contains data storage methods
package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"git.neds.sh/technology/pricekinetics/tools/codetest/model"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type mongoRepo struct {
	client *mongo.Client
	events *mongo.Collection
}

// NewMongoRepository creates a new instance of a Repository using MongoDB as the persistance layer, storing each event as a document with the event ID as its _id
func NewMongoRepository(ctx context.Context, uri string, database string, collection string) (Repository, error) {
	// The generated model structs only have json tags, so use them for the field names to store events in the same shape as their JSON
	clientOpts := options.Client().ApplyURI(uri).SetBSONOptions(&options.BSONOptions{UseJSONStructTags: true})
	client, err := mongo.Connect(clientOpts)
	if err != nil {
		logrus.Errorf("could not create MongoDB client: %v", err)
		return nil, err
	}

	// Check connection
	rslt := &mongoRepo{client: client, events: client.Database(database).Collection(collection)}
	if !rslt.HealthCheck(ctx) {
		_ = client.Disconnect(ctx)
		return nil, fmt.Errorf("failed_to_init_mongo")
	}

	// Indexes for SearchEvents, including _id so results can be sorted from the index. Indexes that already exist are left as they are
	_, err = rslt.events.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "StartTime.Value", Value: 1}, {Key: "_id", Value: 1}}},
		{Keys: bson.D{{Key: "BettingStatus.Value", Value: 1}, {Key: "StartTime.Value", Value: 1}, {Key: "_id", Value: 1}}},
	})
	if err != nil {
		logrus.Errorf("could not create MongoDB indexes: %v", err)
		_ = client.Disconnect(ctx)
		return nil, err
	}

	return rslt, nil
}

func (c *mongoRepo) HealthCheck(ctx context.Context) bool {
	if err := c.client.Ping(ctx, nil); err != nil {
		logrus.Errorf("could not connect to MongoDB: %v", err)
		return false
	}

	return true
}

func (c *mongoRepo) UpdateEvent(ctx context.Context, event *model.Event) error {
	// Replace the whole event, the upsert inserts it with the _id from the filter if it doesn't exist yet
	_, err := c.events.ReplaceOne(ctx, bson.M{"_id": event.ID}, event, options.Replace().SetUpsert(true))
	if err != nil {
		logrus.Errorf("could not update event %v", err)
	}

	return err
}

func (c *mongoRepo) GetEventByID(ctx context.Context, id string) (*model.Event, error) {
	event := &model.Event{}
	err := c.events.FindOne(ctx, bson.M{"_id": id}).Decode(event)
	if errors.Is(err, mongo.ErrNoDocuments) {
		logrus.Infof("Event not found")
		return nil, nil
	} else if err != nil {
		logrus.Errorf("could not get event %v", err)
		return nil, err
	}

	return event, nil
}

func (c *mongoRepo) DeleteEventByID(ctx context.Context, id string) error {
	_, err := c.events.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		logrus.Errorf("could not delete event %v", err)
	}

	return err
}

func (c *mongoRepo) SearchEvents(ctx context.Context, filter EventFilter) ([]*model.Event, error) {
	// Zero values aren't stored and a Deleted value counts as unset, the same as GetRacingEvent, e.g an event that isn't hidden has no Hidden.Value
	conditions := bson.A{}

	if filter.StartTimeFrom != nil || filter.StartTimeTo != nil {
		startTime := bson.M{}
		if filter.StartTimeFrom != nil {
			startTime["$gte"] = unixNano(*filter.StartTimeFrom)
		}
		if filter.StartTimeTo != nil {
			startTime["$lt"] = unixNano(*filter.StartTimeTo)
		}
		conditions = append(conditions, bson.M{"StartTime.Value": startTime, "StartTime.Deleted": bson.M{"$ne": true}})
	}

	if len(filter.BettingStatuses) > 0 {
		anyStatus := bson.A{bson.M{"BettingStatus.Value": bson.M{"$in": filter.BettingStatuses}, "BettingStatus.Deleted": bson.M{"$ne": true}}}
		if slices.Contains(filter.BettingStatuses, model.BettingStatus_BettingUnknown) {
			// BettingUnknown is the zero value so it is never stored, match events without a status or with a Deleted one
			anyStatus = append(anyStatus, bson.M{"BettingStatus.Value": nil}, bson.M{"BettingStatus.Deleted": true})
		}
		conditions = append(conditions, bson.M{"$or": anyStatus})
	}

	if filter.Hidden != nil {
		hidden := bson.M{"Hidden.Value": true, "Hidden.Deleted": bson.M{"$ne": true}}
		if *filter.Hidden {
			conditions = append(conditions, hidden)
		} else {
			conditions = append(conditions, bson.M{"$nor": bson.A{hidden}})
		}
	}

	query := bson.M{}
	if len(conditions) > 0 {
		query["$and"] = conditions
	}

	cursor, err := c.events.Find(ctx, query, options.Find().SetSort(bson.D{{Key: "StartTime.Value", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		logrus.Errorf("could not search events %v", err)
		return nil, err
	}

	events := []*model.Event{}
	if err := cursor.All(ctx, &events); err != nil {
		logrus.Errorf("could not decode events %v", err)
		return nil, err
	}

	return events, nil
}

// unixNano converts t to unix nanoseconds like the model's StartTime, clamping times that are too far from 1970 to fit in an int64
func unixNano(t time.Time) int64 {
	if t.Before(time.Unix(0, math.MinInt64)) {
		return math.MinInt64
	}
	if t.After(time.Unix(0, math.MaxInt64)) {
		return math.MaxInt64
	}
	return t.UnixNano()
}
