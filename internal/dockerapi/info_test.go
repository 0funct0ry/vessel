package dockerapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiskUsageRetainsVolumeNameAndUnknownUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.43/system/df" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"Volumes":[{"Name":"known","UsageData":{"Size":42,"RefCount":1}},{"Name":"unknown"}]}`))
	}))
	defer server.Close()

	client, err := New("tcp://" + server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	disk, err := client.DiskUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(disk.Volumes) != 2 || disk.Volumes[0].Name != "known" || !disk.Volumes[0].UsageKnown || disk.Volumes[0].UsageData.Size != 42 || disk.Volumes[1].Name != "unknown" || disk.Volumes[1].UsageKnown {
		t.Fatalf("volumes = %#v", disk.Volumes)
	}
}
