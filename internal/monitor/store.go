package monitor

import (
	"context"
	"encoding/json"
	"time"

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

// HistoryResult is the uptime summary + recent points for one endpoint.
type HistoryResult struct {
	Name   string   `json:"name"`
	Uptime float64  `json:"uptime"` // fraction up over the window, 0..1
	Points []Status `json:"points"` // most-recent-last, capped
}

// History returns the uptime fraction and up to `limit` most-recent points
// for an endpoint since `since`.
// ponytail: two counts + a capped Find; add server-side bucketing only if a
// window ever returns enough points to matter for the sparkline.
func (s *Store) History(ctx context.Context, name string, since time.Time, limit int64) (HistoryResult, error) {
	res := HistoryResult{Name: name, Points: []Status{}}
	filter := bson.M{"name": name, "ts": bson.M{"$gte": since}}

	total, err := s.hist.CountDocuments(ctx, filter)
	if err != nil {
		return res, err
	}
	if total > 0 {
		upFilter := bson.M{"name": name, "ts": bson.M{"$gte": since}, "up": true}
		up, err := s.hist.CountDocuments(ctx, upFilter)
		if err != nil {
			return res, err
		}
		res.Uptime = float64(up) / float64(total)
	}

	// newest `limit`, then reverse to chronological order for the sparkline
	cur, err := s.hist.Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "ts", Value: -1}}).SetLimit(limit))
	if err != nil {
		return res, err
	}
	var pts []Status
	if err := cur.All(ctx, &pts); err != nil {
		return res, err
	}
	for i := len(pts) - 1; i >= 0; i-- {
		res.Points = append(res.Points, pts[i])
	}
	return res, nil
}
