package database

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Database wraps the MongoDB client and database
type Database struct {
	client *mongo.Client
	db     *mongo.Database
}

// InitMongoDB connects to MongoDB and returns a Database instance
func InitMongoDB(uri, dbName string) (*Database, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	return &Database{
		client: client,
		db:     client.Database(dbName),
	}, nil
}

// CookieCollection returns the insta_cookies collection
func (db *Database) CookieCollection() *mongo.Collection {
	return db.db.Collection("insta_cookies")
}

// Close disconnects the MongoDB client
func (d *Database) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d.client.Disconnect(ctx)
}
