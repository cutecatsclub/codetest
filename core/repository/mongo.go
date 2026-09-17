// Package repository contains data storage methods
package repository

import (
	"context"
	"errors"
	"fmt"

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
