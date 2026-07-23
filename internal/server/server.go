package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Muxcore-Media/cache-redis/internal/cache"
	cachev1 "github.com/Muxcore-Media/core/proto/gen/muxcore/cache/v1"
)

type Server struct {
	cachev1.UnimplementedCacheServiceServer
	cache     *cache.Cache
	getCount  atomic.Int64
	setCount  atomic.Int64
	delCount  atomic.Int64
	incrCount atomic.Int64
	lockMu    sync.Mutex
	locks     map[string]*cache.Lock
}

func New(c *cache.Cache) *Server {
	return &Server{
		cache: c,
		locks: make(map[string]*cache.Lock),
	}
}

func (s *Server) RegisterWithGRPC(srv *grpc.Server) {
	cachev1.RegisterCacheServiceServer(srv, s)
}

func (s *Server) Get(ctx context.Context, req *cachev1.GetCacheRequest) (*cachev1.GetCacheResponse, error) {
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}
	data, err := s.cache.Get(ctx, req.GetKey())
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
	if err := s.cache.Set(ctx, req.GetKey(), req.GetValue(), ttl); err != nil {
		slog.Error("cache: set failed", "key", req.GetKey(), "error", err)
		return nil, status.Error(codes.Internal, "set failed")
	}
	s.setCount.Add(1)
	return &cachev1.SetCacheResponse{Status: "ok"}, nil
}

func (s *Server) Delete(ctx context.Context, req *cachev1.DeleteCacheRequest) (*cachev1.DeleteCacheResponse, error) {
	if err := s.cache.Delete(ctx, req.GetKeys()...); err != nil {
		slog.Error("cache: delete failed", "keys", req.GetKeys(), "error", err)
		return nil, status.Error(codes.Internal, "delete failed")
	}
	s.delCount.Add(1)
	return &cachev1.DeleteCacheResponse{Deleted: int32(len(req.GetKeys()))}, nil
}

func (s *Server) Exists(ctx context.Context, req *cachev1.ExistsCacheRequest) (*cachev1.ExistsCacheResponse, error) {
	exists, err := s.cache.Exists(ctx, req.GetKey())
	if err != nil {
		return nil, status.Error(codes.Internal, "exists check failed")
	}
	return &cachev1.ExistsCacheResponse{Exists: exists}, nil
}

func (s *Server) Incr(ctx context.Context, req *cachev1.IncrCacheRequest) (*cachev1.IncrCacheResponse, error) {
	n, err := s.cache.Incr(ctx, req.GetKey(), req.GetDelta())
	if err != nil {
		return nil, status.Error(codes.Internal, "incr failed")
	}
	s.incrCount.Add(1)
	return &cachev1.IncrCacheResponse{Value: n}, nil
}

func (s *Server) CompareAndSwap(ctx context.Context, req *cachev1.CompareAndSwapCacheRequest) (*cachev1.CompareAndSwapCacheResponse, error) {
	swapped, err := s.cache.CompareAndSwap(ctx, req.GetKey(), req.GetOldValue(), req.GetNewValue())
	if err != nil {
		return nil, status.Error(codes.Internal, "cas failed")
	}
	return &cachev1.CompareAndSwapCacheResponse{Swapped: swapped}, nil
}

func (s *Server) Lock(ctx context.Context, req *cachev1.LockCacheRequest) (*cachev1.LockCacheResponse, error) {
	ttl := time.Duration(req.GetTtlSeconds()) * time.Second
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	lk, err := s.cache.Lock(ctx, req.GetKey(), ttl)
	if err != nil {
		return &cachev1.LockCacheResponse{
			Key:      req.GetKey(),
			Acquired: false,
		}, nil
	}
	token := fmt.Sprintf("%x", time.Now().UnixNano())
	s.lockMu.Lock()
	s.locks[token] = lk
	s.lockMu.Unlock()
	return &cachev1.LockCacheResponse{
		Key:      req.GetKey(),
		Token:    token,
		Acquired: true,
	}, nil
}

func (s *Server) Unlock(ctx context.Context, req *cachev1.UnlockCacheRequest) (*cachev1.UnlockCacheResponse, error) {
	token := req.GetToken()
	s.lockMu.Lock()
	lk, ok := s.locks[token]
	delete(s.locks, token)
	s.lockMu.Unlock()
	if !ok {
		return &cachev1.UnlockCacheResponse{Status: "lock not found"}, nil
	}
	lk.Unlock(ctx)
	return &cachev1.UnlockCacheResponse{Status: "ok"}, nil
}

func (s *Server) Publish(ctx context.Context, req *cachev1.PublishCacheRequest) (*cachev1.PublishCacheResponse, error) {
	if err := s.cache.Publish(ctx, req.GetChannel(), req.GetMessage()); err != nil {
		return nil, status.Error(codes.Internal, "publish failed")
	}
	return &cachev1.PublishCacheResponse{}, nil
}

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
	ch, err := s.cache.Subscribe(stream.Context(), req.GetChannel())
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
