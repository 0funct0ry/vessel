package dockerapi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"testing"
)

func newStatsReader(t *testing.T, samples ...string) *StatsReader {
	t.Helper()
	data := bytes.NewBufferString("")
	for _, s := range samples {
		data.WriteString(s)
	}
	return &StatsReader{
		body: io.NopCloser(data),
		dec:  json.NewDecoder(bufio.NewReader(bytes.NewReader(data.Bytes()))),
	}
}

const statsSampleV2 = `{
  "read": "2024-01-01T00:00:00Z",
  "cpu_stats": {"cpu_usage": {"total_usage": 1000}, "system_cpu_usage": 100000, "online_cpus": 2},
  "precpu_stats": {"cpu_usage": {"total_usage": 0}, "system_cpu_usage": 0},
  "memory_stats": {"usage": 5000000, "limit": 8000000, "stats": {"inactive_file": 1000000}},
  "networks": {"eth0": {"rx_bytes": 11, "tx_bytes": 12}, "eth1": {"rx_bytes": 3, "tx_bytes": 4}},
  "blkio_stats": {"io_service_bytes_recursive": [{"op":"Read","value":9},{"op":"Write","value":8},{"op":"Sync","value":7}]}
}`

const statsSampleV2Next = `{
  "read": "2024-01-01T00:00:02Z",
  "cpu_stats": {"cpu_usage": {"total_usage": 3000}, "system_cpu_usage": 300000, "online_cpus": 2},
  "precpu_stats": {"cpu_usage": {"total_usage": 1000}, "system_cpu_usage": 100000},
	"memory_stats": {"usage": 6000000, "limit": 8000000, "stats": {"inactive_file": 1000000}}
}`

const statsSampleV1 = `{
  "read": "2024-01-01T00:00:00Z",
  "cpu_stats": {"cpu_usage": {"total_usage": 1000}, "system_cpu_usage": 100000, "online_cpus": 2},
  "precpu_stats": {"cpu_usage": {"total_usage": 0}, "system_cpu_usage": 0},
	"memory_stats": {"usage": 5000000, "limit": 9000000, "stats": {"cache": 2000000}}
}`

func TestStatsReader_FirstSampleCPUPercentIsNil(t *testing.T) {
	r := newStatsReader(t, statsSampleV2)

	s, err := r.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if s.CPUPercent != nil {
		t.Fatalf("CPUPercent = %v, want nil for first sample", *s.CPUPercent)
	}
	if s.MemUsage != 4000000 {
		t.Fatalf("MemUsage = %d, want 4000000 (5000000 - 1000000 inactive_file)", s.MemUsage)
	}
	if s.MemLimit != 8000000 || s.NetRX != 14 || s.NetTX != 16 || s.BlkRead != 9 || s.BlkWrite != 8 {
		t.Fatalf("stats fields = %+v", s)
	}
}

func TestStatsReader_SecondSampleComputesCPUPercent(t *testing.T) {
	r := newStatsReader(t, statsSampleV2, statsSampleV2Next)

	if _, err := r.Next(); err != nil {
		t.Fatalf("first Next: %v", err)
	}

	s, err := r.Next()
	if err != nil {
		t.Fatalf("second Next: %v", err)
	}
	if s.CPUPercent == nil {
		t.Fatal("CPUPercent = nil, want a computed value for second sample")
	}
	// delta(cpu)=2000, delta(sys)=200000, online=2 -> (2000/200000)*2*100 = 2.0
	want := 2.0
	if *s.CPUPercent != want {
		t.Fatalf("CPUPercent = %v, want %v", *s.CPUPercent, want)
	}
}

func TestStatsReader_CgroupV1MemoryUsesCache(t *testing.T) {
	r := newStatsReader(t, statsSampleV1)

	s, err := r.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if s.MemUsage != 3000000 {
		t.Fatalf("MemUsage = %d, want 3000000 (5000000 - 2000000 cache)", s.MemUsage)
	}
}
