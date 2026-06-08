package servers

import (
	"encoding/json"
	"testing"
	"time"
)

type metricRowStub struct {
	values []any
}

func (r metricRowStub) Scan(dest ...any) error {
	for i := range dest {
		switch target := dest[i].(type) {
		case *string:
			*target = r.values[i].(string)
		case *float64:
			*target = r.values[i].(float64)
		case *int64:
			*target = r.values[i].(int64)
		case *[]byte:
			*target = r.values[i].([]byte)
		case *time.Time:
			*target = r.values[i].(time.Time)
		}
	}
	return nil
}

func TestScanMetricSamplePreservesRaw(t *testing.T) {
	collectedAt := time.Date(2026, 6, 9, 13, 0, 0, 0, time.UTC)
	raw, _ := json.Marshal(map[string]any{"source": "agent"})
	row := metricRowStub{values: []any{
		"server-1",
		1.5,
		int64(2),
		3.5,
		int64(4),
		int64(5),
		int64(6),
		int64(7),
		int64(8),
		int64(9),
		int64(10),
		int64(11),
		12.5,
		13.5,
		14.5,
		15.5,
		16.5,
		int64(17),
		int64(18),
		int64(19),
		int64(20),
		"amd64",
		"kvm",
		"Ubuntu",
		"EPYC",
		"none",
		"cn",
		raw,
		collectedAt,
	}}

	metric, err := scanMetricSample(row)
	if err != nil {
		t.Fatalf("scanMetricSample returned error: %v", err)
	}
	if metric.ServerID != "server-1" || metric.CPUPercent != 1.5 || metric.CollectedAt != collectedAt {
		t.Fatalf("metric fields not scanned: %+v", metric)
	}
	if metric.Raw["source"] != "agent" {
		t.Fatalf("raw payload not preserved: %#v", metric.Raw)
	}
}
