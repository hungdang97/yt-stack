package repository

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// CookieDoc represents a TikTok cookie document in MongoDB
type CookieDoc struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	Cookie    string             `bson:"cookie_string"`
	Status    string             `bson:"status"` // "active" or "inactive"
	CreatedAt time.Time          `bson:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at,omitempty"`
	Note      string             `bson:"note,omitempty"`
}

// CookieRepository handles cookie operations in MongoDB
type CookieRepository struct {
	collection *mongo.Collection
}

// NewCookieRepository creates a new CookieRepository
func NewCookieRepository(collection *mongo.Collection) *CookieRepository {
	return &CookieRepository{collection: collection}
}

// GetRandomActiveCookies returns up to 'limit' active cookies randomly
func (r *CookieRepository) GetRandomActiveCookies(limit int) ([]CookieDoc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		{"$match": bson.M{"status": "active"}},
		{"$sample": bson.M{"size": limit}},
	}

	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate active cookies: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []CookieDoc
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("failed to decode cookies: %w", err)
	}

	return docs, nil
}

// InvalidateCookie marks a cookie as inactive
func (r *CookieRepository) InvalidateCookie(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid cookie ID: %w", err)
	}

	filter := bson.M{"_id": objID}
	update := bson.M{
		"$set": bson.M{
			"status":     "inactive",
			"updated_at": time.Now(),
		},
	}

	_, err = r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to invalidate cookie: %w", err)
	}

	return nil
}

// AddCookie inserts a new active cookie into the pool
func (r *CookieRepository) AddCookie(cookie, note string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	doc := CookieDoc{
		Cookie:    cookie,
		Status:    "active",
		CreatedAt: time.Now(),
		Note:      note,
	}

	_, err := r.collection.InsertOne(ctx, doc)
	if err != nil {
		return fmt.Errorf("failed to add cookie: %w", err)
	}

	return nil
}
