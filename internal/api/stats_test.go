package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

type fakeStatsStream struct {
	mu      sync.Mutex
	samples []dockerapi.Stats
	next    int
	closed  bool
}

func (s *fakeStatsStream) Next() (dockerapi.Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.next == len(s.samples) {
		return dockerapi.Stats{}, io.EOF
	}
	value := s.samples[s.next]
	s.next++
	return value, nil
}
func (s *fakeStatsStream) Close() error { s.mu.Lock(); s.closed = true; s.mu.Unlock(); return nil }

func TestContainerStatsSSEPayload(t *testing.T) {
	fake := newFakeDockerClient()
	fake.statsStream = &fakeStatsStream{samples: []dockerapi.Stats{{Read: "2026-09-08T00:00:00Z", MemUsage: 4, MemLimit: 8, NetRX: 2, NetTX: 3, BlkRead: 5, BlkWrite: 6}}}
	response := performRequest(NewRouter(Config{Docker: fake}), http.MethodGet, "/api/v1/containers/c1/stats?interval=2s")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	want := "event: stats\ndata: {\"blk\":{\"read\":5,\"write\":6},\"cpu_pct\":null,\"mem\":{\"limit\":8,\"used\":4},\"net\":{\"rx\":2,\"tx\":3},\"ts\":\"2026-09-08T00:00:00Z\"}"
	if !strings.Contains(response.Body.String(), want) {
		t.Fatalf("body=%q", response.Body.String())
	}
	if len(fake.statsCalls) != 1 || fake.statsCalls[0] != "c1" {
		t.Fatalf("stats calls=%v", fake.statsCalls)
	}
}

func TestContainerStatsRejectsMalformedInterval(t *testing.T) {
	response := performRequest(NewRouter(Config{Docker: newFakeDockerClient()}), http.MethodGet, "/api/v1/containers/c1/stats?interval=soon")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"code":"invalid_query"`) {
		t.Fatalf("body=%s", response.Body.String())
	}
}

func TestStatsHubSharesAndClosesStream(t *testing.T) {
	stream := newBlockingStatsStream(dockerapi.Stats{Read: "now"})
	fake := newFakeDockerClient()
	fake.statsStream = stream
	hub := newStatsHub(fake)
	a, b := hub.subscribe("c1"), hub.subscribe("c1")
	defer a.cancel()
	defer b.cancel()
	select {
	case <-a.updates:
	case <-time.After(time.Second):
		t.Fatal("first subscriber did not receive sample")
	}
	select {
	case <-b.updates:
	case <-time.After(time.Second):
		t.Fatal("second subscriber did not receive sample")
	}
	if len(fake.statsCalls) != 1 {
		t.Fatalf("upstream calls=%d, want 1", len(fake.statsCalls))
	}
	a.cancel()
	b.cancel()
	select {
	case <-stream.done:
	case <-time.After(time.Second):
		t.Fatal("upstream did not close after last subscriber")
	}
}

type blockingStatsStream struct {
	sample  dockerapi.Stats
	mu      sync.Mutex
	sent    bool
	closeCh chan struct{}
	done    chan struct{}
	once    sync.Once
}

func newBlockingStatsStream(sample dockerapi.Stats) *blockingStatsStream {
	return &blockingStatsStream{sample: sample, closeCh: make(chan struct{}), done: make(chan struct{})}
}
func (s *blockingStatsStream) Next() (dockerapi.Stats, error) {
	s.mu.Lock()
	if !s.sent {
		s.sent = true
		s.mu.Unlock()
		return s.sample, nil
	}
	s.mu.Unlock()
	<-s.closeCh
	return dockerapi.Stats{}, context.Canceled
}
func (s *blockingStatsStream) Close() error {
	s.once.Do(func() { close(s.closeCh); close(s.done) })
	return nil
}

func TestHostAggregatesAndDiskBytes(t *testing.T) {
	fake := newFakeDockerClient()
	pct := 12.5
	fake.stats = dockerapi.Stats{CPUPercent: &pct, MemUsage: 10, MemLimit: 20}
	server := &server{docker: fake}
	cpu, memory, top, err := server.hostAggregates(context.Background(), []dockerapi.Container{{ID: "run", State: "running"}, {ID: "stop", State: "exited"}})
	if err != nil || cpu != 12.5 || memory.Used != 10 || memory.Limit != 20 {
		t.Fatalf("cpu=%v memory=%+v err=%v", cpu, memory, err)
	}
	if len(top) != 1 || top[0].ID != "run" || top[0].CPUPercent != 12.5 {
		t.Fatalf("top=%+v", top)
	}
	disk := diskToHostView(&dockerapi.DiskUsageInfo{Images: []dockerapi.DiskImage{{Size: 10, Containers: 0}, {Size: 7, Containers: 1}}, Containers: []dockerapi.DiskContainer{{SizeRW: 3, State: "exited"}}, BuildCache: []dockerapi.DiskBuildCache{{Size: 4, UsageCount: 0}}})
	if disk.Images != 17 || disk.Containers != 3 || disk.BuildCache != 4 || disk.Reclaimable != 17 {
		t.Fatalf("disk=%+v", disk)
	}
	if disk.ImagesReclaimable != 10 || disk.ContainersReclaimable != 3 || disk.BuildCacheReclaimable != 4 {
		t.Fatalf("per-kind reclaimable disk=%+v", disk)
	}
}

func TestHostAggregatesTopCPUAndMemTruncateAndSort(t *testing.T) {
	fake := newFakeDockerClient()
	fake.statsByID = map[string]dockerapi.Stats{}
	containers := make([]dockerapi.Container, 0, 7)
	for i := 0; i < 7; i++ {
		id := fmt.Sprintf("c%d", i)
		pct := float64(i)
		mem := uint64(i) * 10
		fake.statsByID[id] = dockerapi.Stats{CPUPercent: &pct, MemUsage: mem, MemLimit: 100}
		containers = append(containers, dockerapi.Container{ID: id, Names: []string{"/" + id}, State: "running"})
	}
	server := &server{docker: fake}
	_, _, top, err := server.hostAggregates(context.Background(), containers)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	byCPU := topByCPU(top)
	byMem := topByMem(top)
	if len(byCPU) != 5 || len(byMem) != 5 {
		t.Fatalf("expected 5 entries each, got cpu=%d mem=%d", len(byCPU), len(byMem))
	}
	if byCPU[0].ID != "c6" || byCPU[0].CPUPercent != 6 || byCPU[4].ID != "c2" {
		t.Fatalf("byCPU=%+v", byCPU)
	}
	if byMem[0].ID != "c6" || byMem[0].MemUsed != 60 || byMem[4].ID != "c2" {
		t.Fatalf("byMem=%+v", byMem)
	}
}
