package monitor

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Store persists probe results: Redis holds the hot current status per
// endpoint; Mongo holds the 30-day history (auto-expired by a TTL index).
type Store struct {
	rdb  *redis.Client
	hist *mongo.Collection
}

func NewStore(ctx context.Context, mongoURI, redisAddr string) (*Store, error) {
	mc, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, err
	}
	hist := mc.Database("statusping").Collection("history")

	// TTL index: drop docs older than 30 days.
	_, err = hist.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "ts", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(30 * 24 * 3600),
	})
	if err != nil {
		return nil, err
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	return &Store{rdb: rdb, hist: hist}, nil
}

// Save writes the latest status to Redis and appends to Mongo history.
func (s *Store) Save(ctx context.Context, st Status) {
	if b, err := json.Marshal(st); err == nil {
		s.rdb.Set(ctx, "status:"+st.Name, b, 0)
	}
	s.hist.InsertOne(ctx, st) // history is best-effort; a dropped point isn't fatal
}
