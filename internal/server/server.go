package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Muxcore-Media/cache-redis/internal/cache"
	cachev1 "github.com/Muxcore-Media/core/proto/gen/muxcore/cache/v1"
)

// DefaultLockTTL is applied when Lock requests ttl_seconds <= 0.
const DefaultLockTTL = 30 * time.Second

type Server struct {
	cachev1.UnimplementedCacheServiceServer
	cachePtr  atomic.Pointer[cache.Cache]
	getCount  atomic.Int64
	setCount  atomic.Int64
	delCount  atomic.Int64
	incrCount atomic.Int64
}

func New(c *cache.Cache) *Server {
	s := &Server{}
	s.cachePtr.Store(c)
	return s
}

// ReplaceCache swaps the backing Redis client.
// Returns the previous cache (caller should Close it).
func (s *Server) ReplaceCache(c *cache.Cache) *cache.Cache {
	return s.cachePtr.Swap(c)
}

func (s *Server) cache() *cache.Cache {
	return s.cachePtr.Load()
}

func (s *Server) RegisterWithGRPC(srv *grpc.Server) {
	cachev1.RegisterCacheServiceServer(srv, s)
}

func (s *Server) Get(ctx context.Context, req *cachev1.GetCacheRequest) (*cachev1.GetCacheResponse, error) {
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}
	data, err := s.cache().Get(ctx, req.GetKey())
	if err != nil {
		slog.Error("cache: get failed", "key", req.GetKey(), "error", err)
		return nil, status.Error(codes.Internal, "get failed")
	}
	s.getCount.Add(1)
	if data == nil {
		return &cachev1.GetCacheResponse{Key: req.GetKey(), Found: false}, nil
	}
	return &cachev1.GetCacheResponse{Key: req.GetKey(), Value: data, Found: true}, nil
}

func (s *Server) Set(ctx context.Context, req *cachev1.SetCacheRequest) (*cachev1.SetCacheResponse, error) {
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}
	ttl := time.Duration(req.GetTtlSeconds()) * time.Second
	if err := s.cache().Set(ctx, req.GetKey(), req.GetValue(), ttl); err != nil {
		slog.Error("cache: set failed", "key", req.GetKey(), "error", err)
		return nil, status.Error(codes.Internal, "set failed")
	}
	s.setCount.Add(1)
	return &cachev1.SetCacheResponse{Status: "ok"}, nil
}

func (s *Server) Delete(ctx context.Context, req *cachev1.DeleteCacheRequest) (*cachev1.DeleteCacheResponse, error) {
	n, err := s.cache().Delete(ctx, req.GetKeys()...)
	if err != nil {
		slog.Error("cache: delete failed", "keys", req.GetKeys(), "error", err)
		return nil, status.Error(codes.Internal, "delete failed")
	}
	s.delCount.Add(1)
	return &cachev1.DeleteCacheResponse{Deleted: int32(n)}, nil
}

func (s *Server) Exists(ctx context.Context, req *cachev1.ExistsCacheRequest) (*cachev1.ExistsCacheResponse, error) {
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}
	exists, err := s.cache().Exists(ctx, req.GetKey())
	if err != nil {
		return nil, status.Error(codes.Internal, "exists check failed")
	}
	return &cachev1.ExistsCacheResponse{Exists: exists}, nil
}

func (s *Server) Incr(ctx context.Context, req *cachev1.IncrCacheRequest) (*cachev1.IncrCacheResponse, error) {
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}
	n, err := s.cache().Incr(ctx, req.GetKey(), req.GetDelta())
	if err != nil {
		return nil, status.Error(codes.Internal, "incr failed")
	}
	s.incrCount.Add(1)
	return &cachev1.IncrCacheResponse{Value: n}, nil
}

func (s *Server) CompareAndSwap(ctx context.Context, req *cachev1.CompareAndSwapCacheRequest) (*cachev1.CompareAndSwapCacheResponse, error) {
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}
	swapped, err := s.cache().CompareAndSwap(ctx, req.GetKey(), req.GetOldValue(), req.GetNewValue())
	if err != nil {
		return nil, status.Error(codes.Internal, "cas failed")
	}
	return &cachev1.CompareAndSwapCacheResponse{Swapped: swapped}, nil
}

func (s *Server) Lock(ctx context.Context, req *cachev1.LockCacheRequest) (*cachev1.LockCacheResponse, error) {
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}
	ttl := time.Duration(req.GetTtlSeconds()) * time.Second
	if ttl <= 0 {
		ttl = DefaultLockTTL
	}
	token, err := s.cache().Lock(ctx, req.GetKey(), ttl)
	if err != nil {
		return &cachev1.LockCacheResponse{
			Key:      req.GetKey(),
			Acquired: false,
		}, nil
	}
	return &cachev1.LockCacheResponse{
		Key:      req.GetKey(),
		Token:    token,
		Acquired: true,
	}, nil
}

func (s *Server) Unlock(ctx context.Context, req *cachev1.UnlockCacheRequest) (*cachev1.UnlockCacheResponse, error) {
	token := req.GetToken()
	if token == "" {
		return nil, status.Error(codes.InvalidArgument, "token is required")
	}
	err := s.cache().UnlockByToken(ctx, token)
	if errors.Is(err, cache.ErrLockNotHeld) {
		return &cachev1.UnlockCacheResponse{Status: "lock not held"}, nil
	}
	if err != nil {
		slog.Error("cache: unlock failed", "error", err)
		return nil, status.Error(codes.Internal, "unlock failed")
	}
	return &cachev1.UnlockCacheResponse{Status: "ok"}, nil
}

func (s *Server) Publish(ctx context.Context, req *cachev1.PublishCacheRequest) (*cachev1.PublishCacheResponse, error) {
	if req.GetChannel() == "" {
		return nil, status.Error(codes.InvalidArgument, "channel is required")
	}
	if err := s.cache().Publish(ctx, req.GetChannel(), req.GetMessage()); err != nil {
		return nil, status.Error(codes.Internal, "publish failed")
	}
	return &cachev1.PublishCacheResponse{}, nil
}

// Metrics renders Prometheus text for cache operation counters.
func (s *Server) Metrics() string {
	var b strings.Builder
	b.WriteString("# HELP cache_get_total Total cache get operations\n")
	b.WriteString("# TYPE cache_get_total counter\n")
	fmt.Fprintf(&b, "cache_get_total %d\n", s.getCount.Load())
	b.WriteString("# HELP cache_set_total Total cache set operations\n")
	b.WriteString("# TYPE cache_set_total counter\n")
	fmt.Fprintf(&b, "cache_set_total %d\n", s.setCount.Load())
	b.WriteString("# HELP cache_delete_total Total cache delete operations\n")
	b.WriteString("# TYPE cache_delete_total counter\n")
	fmt.Fprintf(&b, "cache_delete_total %d\n", s.delCount.Load())
	b.WriteString("# HELP cache_incr_total Total cache increment operations\n")
	b.WriteString("# TYPE cache_incr_total counter\n")
	fmt.Fprintf(&b, "cache_incr_total %d\n", s.incrCount.Load())
	return b.String()
}

func (s *Server) Subscribe(req *cachev1.SubscribeCacheRequest, stream cachev1.CacheService_SubscribeServer) error {
	if req.GetChannel() == "" {
		return status.Error(codes.InvalidArgument, "channel is required")
	}
	ch, err := s.cache().Subscribe(stream.Context(), req.GetChannel())
	if err != nil {
		return status.Error(codes.Internal, "subscribe failed")
	}
	for msg := range ch {
		if err := stream.Send(&cachev1.SubscribeCacheResponse{
			Channel: req.GetChannel(),
			Message: msg,
		}); err != nil {
			return err
		}
	}
	return nil
}
