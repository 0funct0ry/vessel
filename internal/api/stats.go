package api

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/0funct0ry/vessel/internal/dockerapi"
	"github.com/gin-gonic/gin"
)

const defaultStatsInterval = 2 * time.Second

type statsSubscription struct {
	updates <-chan dockerapi.Stats
	errs    <-chan error
	cancel  func()
}

type statsHub struct {
	docker  DockerClient
	mu      sync.Mutex
	streams map[string]*statsHubStream
}

type statsHubStream struct {
	cancel context.CancelFunc
	subs   map[uint64]chan dockerapi.Stats
	errs   map[uint64]chan error
	nextID uint64
}

func newStatsHub(docker DockerClient) *statsHub {
	return &statsHub{docker: docker, streams: make(map[string]*statsHubStream)}
}

func (h *statsHub) subscribe(id string) statsSubscription {
	h.mu.Lock()
	stream := h.streams[id]
	if stream == nil {
		ctx, cancel := context.WithCancel(context.Background())
		stream = &statsHubStream{cancel: cancel, subs: make(map[uint64]chan dockerapi.Stats), errs: make(map[uint64]chan error)}
		h.streams[id] = stream
		go h.run(ctx, id, stream)
	}
	subID := stream.nextID
	stream.nextID++
	updates := make(chan dockerapi.Stats, 1)
	errs := make(chan error, 1)
	stream.subs[subID], stream.errs[subID] = updates, errs
	h.mu.Unlock()

	var once sync.Once
	return statsSubscription{updates: updates, errs: errs, cancel: func() { once.Do(func() { h.unsubscribe(id, subID) }) }}
}

func (h *statsHub) unsubscribe(id string, subID uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	stream := h.streams[id]
	if stream == nil {
		return
	}
	delete(stream.subs, subID)
	delete(stream.errs, subID)
	if len(stream.subs) == 0 {
		delete(h.streams, id)
		stream.cancel()
	}
}

func (h *statsHub) run(ctx context.Context, id string, stream *statsHubStream) {
	reader, err := h.docker.StatsStream(ctx, id)
	if err == nil {
		readerDone := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				_ = reader.Close()
			case <-readerDone:
			}
		}()
		defer close(readerDone)
		defer reader.Close()
		for {
			sample, nextErr := reader.Next()
			if nextErr != nil {
				if !errors.Is(nextErr, io.EOF) && !errors.Is(nextErr, context.Canceled) {
					err = nextErr
				}
				break
			}
			h.publish(id, stream, sample, nil)
		}
	}
	if err != nil {
		h.publish(id, stream, dockerapi.Stats{}, err)
	}
	h.mu.Lock()
	if h.streams[id] == stream {
		delete(h.streams, id)
	}
	for _, updates := range stream.subs {
		close(updates)
	}
	for _, errs := range stream.errs {
		close(errs)
	}
	h.mu.Unlock()
}

func (h *statsHub) publish(id string, stream *statsHubStream, sample dockerapi.Stats, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.streams[id] != stream {
		return
	}
	if err != nil {
		for _, errs := range stream.errs {
			select {
			case errs <- err:
			default:
			}
		}
		return
	}
	for _, updates := range stream.subs {
		select {
		case updates <- sample:
		default:
			select {
			case <-updates:
			default:
			}
			select {
			case updates <- sample:
			default:
			}
		}
	}
}

func parseStatsInterval(c *gin.Context) (time.Duration, error) {
	raw := c.Query("interval")
	if raw == "" {
		return defaultStatsInterval, nil
	}
	interval, err := time.ParseDuration(raw)
	if err != nil || interval <= 0 {
		return 0, invalidQuery("interval must be a positive duration")
	}
	if interval < time.Second {
		return time.Second, nil
	}
	if interval > 30*time.Second {
		return 30 * time.Second, nil
	}
	return interval, nil
}

func statsPayload(sample dockerapi.Stats) map[string]any {
	return map[string]any{
		"ts": sample.Read, "cpu_pct": sample.CPUPercent,
		"mem": map[string]uint64{"used": sample.MemUsage, "limit": sample.MemLimit},
		"net": map[string]uint64{"rx": sample.NetRX, "tx": sample.NetTX},
		"blk": map[string]uint64{"read": sample.BlkRead, "write": sample.BlkWrite},
	}
}

func (s *server) handleContainerStats(c *gin.Context) {
	interval, err := parseStatsInterval(c)
	if err != nil {
		Fail(c, err)
		return
	}
	subscription := s.stats.subscribe(c.Param("id"))
	defer subscription.cancel()
	Stream(c, func(send func(string, any) error) error {
		last := time.Time{}
		for {
			select {
			case err, ok := <-subscription.errs:
				if ok && err != nil {
					return forResource("container", c.Param("id"), err)
				}
				return nil
			case sample, ok := <-subscription.updates:
				if !ok {
					return nil
				}
				now := time.Now()
				if !last.IsZero() && now.Sub(last) < interval {
					continue
				}
				last = now
				if err := send("stats", statsPayload(sample)); err != nil {
					return err
				}
			case <-c.Request.Context().Done():
				return c.Request.Context().Err()
			}
		}
	})
}
