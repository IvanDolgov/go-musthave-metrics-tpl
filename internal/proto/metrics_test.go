package proto

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestMetric_MType_Enum(t *testing.T) {
	tests := []struct {
		name     string
		mtype    Metric_MType
		expected string
		value    int32
	}{
		{
			name:     "GAUGE type",
			mtype:    Metric_GAUGE,
			expected: "GAUGE",
			value:    0,
		},
		{
			name:     "COUNTER type",
			mtype:    Metric_COUNTER,
			expected: "COUNTER",
			value:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Enum()
			if got := tt.mtype.Enum(); *got != tt.mtype {
				t.Errorf("Enum() = %v, want %v", *got, tt.mtype)
			}

			// Test String()
			if got := tt.mtype.String(); got != tt.expected {
				t.Errorf("String() = %v, want %v", got, tt.expected)
			}

			// Test Type()
			if got := tt.mtype.Type(); got == nil {
				t.Error("Type() returned nil")
			}

			// Test Number()
			if got := tt.mtype.Number(); got != protoreflect.EnumNumber(tt.value) {
				t.Errorf("Number() = %v, want %v", got, tt.value)
			}

			// Test Descriptor()
			if got := tt.mtype.Descriptor(); got == nil {
				t.Error("Descriptor() returned nil")
			}
		})
	}
}

func TestMetric_MType_EnumDescriptor(t *testing.T) {
	mtype := Metric_GAUGE
	desc, _ := mtype.EnumDescriptor()
	if desc == nil {
		t.Error("EnumDescriptor() returned nil")
	}
}

func TestMetric_Reset(t *testing.T) {
	metric := &Metric{
		Id:    "test",
		Type:  Metric_GAUGE,
		Delta: 100,
		Value: 123.456,
	}

	metric.Reset()

	if metric.Id != "" || metric.Type != 0 || metric.Delta != 0 || metric.Value != 0 {
		t.Error("Reset() failed to reset all fields")
	}
}

func TestMetric_String(t *testing.T) {
	metric := &Metric{
		Id:    "test_metric",
		Type:  Metric_GAUGE,
		Value: 123.456,
	}

	str := metric.String()
	if str == "" {
		t.Error("String() returned empty string")
	}
}

func TestMetric_ProtoMessage(t *testing.T) {
	metric := &Metric{}
	metric.ProtoMessage() // Just ensure it doesn't panic
}

func TestMetric_ProtoReflect(t *testing.T) {
	metric := &Metric{}
	ref := metric.ProtoReflect()
	if ref == nil {
		t.Error("ProtoReflect() returned nil")
	}
}

func TestMetric_Descriptor(t *testing.T) {
	metric := &Metric{}
	desc, _ := metric.Descriptor()
	if desc == nil {
		t.Error("Descriptor() returned nil")
	}
}

func TestMetric_GetId(t *testing.T) {
	tests := []struct {
		name     string
		metric   *Metric
		expected string
	}{
		{
			name:     "with id",
			metric:   &Metric{Id: "test_id"},
			expected: "test_id",
		},
		{
			name:     "nil metric",
			metric:   nil,
			expected: "",
		},
		{
			name:     "empty id",
			metric:   &Metric{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.metric.GetId(); got != tt.expected {
				t.Errorf("GetId() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMetric_GetType(t *testing.T) {
	tests := []struct {
		name     string
		metric   *Metric
		expected Metric_MType
	}{
		{
			name:     "gauge type",
			metric:   &Metric{Type: Metric_GAUGE},
			expected: Metric_GAUGE,
		},
		{
			name:     "counter type",
			metric:   &Metric{Type: Metric_COUNTER},
			expected: Metric_COUNTER,
		},
		{
			name:     "nil metric",
			metric:   nil,
			expected: Metric_GAUGE, // zero value
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.metric.GetType(); got != tt.expected {
				t.Errorf("GetType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMetric_GetDelta(t *testing.T) {
	tests := []struct {
		name     string
		metric   *Metric
		expected int64
	}{
		{
			name:     "with delta",
			metric:   &Metric{Delta: 42},
			expected: 42,
		},
		{
			name:     "nil metric",
			metric:   nil,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.metric.GetDelta(); got != tt.expected {
				t.Errorf("GetDelta() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMetric_GetValue(t *testing.T) {
	tests := []struct {
		name     string
		metric   *Metric
		expected float64
	}{
		{
			name:     "with value",
			metric:   &Metric{Value: 3.14},
			expected: 3.14,
		},
		{
			name:     "nil metric",
			metric:   nil,
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.metric.GetValue(); got != tt.expected {
				t.Errorf("GetValue() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestUpdateMetricsRequest_Reset(t *testing.T) {
	req := &UpdateMetricsRequest{
		Metrics: []*Metric{
			{Id: "test1", Type: Metric_GAUGE, Value: 1.0},
			{Id: "test2", Type: Metric_COUNTER, Delta: 5},
		},
	}

	req.Reset()

	if req.Metrics != nil {
		t.Error("Reset() failed to reset Metrics field")
	}
}

func TestUpdateMetricsRequest_String(t *testing.T) {
	req := &UpdateMetricsRequest{
		Metrics: []*Metric{
			{Id: "test", Type: Metric_GAUGE, Value: 1.0},
		},
	}

	str := req.String()
	if str == "" {
		t.Error("String() returned empty string")
	}
}

func TestUpdateMetricsRequest_ProtoMessage(t *testing.T) {
	req := &UpdateMetricsRequest{}
	req.ProtoMessage() // Just ensure it doesn't panic
}

func TestUpdateMetricsRequest_ProtoReflect(t *testing.T) {
	req := &UpdateMetricsRequest{}
	ref := req.ProtoReflect()
	if ref == nil {
		t.Error("ProtoReflect() returned nil")
	}
}

func TestUpdateMetricsRequest_Descriptor(t *testing.T) {
	req := &UpdateMetricsRequest{}
	desc, _ := req.Descriptor()
	if desc == nil {
		t.Error("Descriptor() returned nil")
	}
}

func TestUpdateMetricsRequest_GetMetrics(t *testing.T) {
	tests := []struct {
		name     string
		req      *UpdateMetricsRequest
		expected []*Metric
	}{
		{
			name: "with metrics",
			req: &UpdateMetricsRequest{
				Metrics: []*Metric{
					{Id: "test1", Value: 1.0},
					{Id: "test2", Delta: 5},
				},
			},
			expected: []*Metric{
				{Id: "test1", Value: 1.0},
				{Id: "test2", Delta: 5},
			},
		},
		{
			name:     "nil request",
			req:      nil,
			expected: nil,
		},
		{
			name:     "empty metrics",
			req:      &UpdateMetricsRequest{},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.req.GetMetrics()

			if tt.expected == nil {
				if got != nil {
					t.Errorf("GetMetrics() = %v, want nil", got)
				}
				return
			}

			if len(got) != len(tt.expected) {
				t.Errorf("GetMetrics() returned %d metrics, want %d", len(got), len(tt.expected))
				return
			}

			for i, metric := range got {
				if metric.Id != tt.expected[i].Id {
					t.Errorf("Metric[%d].Id = %v, want %v", i, metric.Id, tt.expected[i].Id)
				}
			}
		})
	}
}

func TestUpdateMetricsResponse_Reset(t *testing.T) {
	resp := &UpdateMetricsResponse{}
	resp.Reset() // Just ensure it doesn't panic
}

func TestUpdateMetricsResponse_String(t *testing.T) {
	resp := &UpdateMetricsResponse{}
	str := resp.String()
	if str == "" {
		t.Error("String() returned empty string")
	}
}

func TestUpdateMetricsResponse_ProtoMessage(t *testing.T) {
	resp := &UpdateMetricsResponse{}
	resp.ProtoMessage() // Just ensure it doesn't panic
}

func TestUpdateMetricsResponse_ProtoReflect(t *testing.T) {
	resp := &UpdateMetricsResponse{}
	ref := resp.ProtoReflect()
	if ref == nil {
		t.Error("ProtoReflect() returned nil")
	}
}

func TestUpdateMetricsResponse_Descriptor(t *testing.T) {
	resp := &UpdateMetricsResponse{}
	desc, _ := resp.Descriptor()
	if desc == nil {
		t.Error("Descriptor() returned nil")
	}
}

func TestFileDescriptor(t *testing.T) {
	desc := file_internal_proto_metrics_proto_rawDescGZIP()
	if desc == nil {
		t.Error("file_internal_proto_metrics_proto_rawDescGZIP() returned nil")
	}
	if len(desc) == 0 {
		t.Error("file descriptor is empty")
	}
}

func TestProtoInit(t *testing.T) {
	// Just ensure init() doesn't panic
	// We can't directly call init(), but we can trigger it by using the package
	_ = file_internal_proto_metrics_proto_init
}

func TestMetricProtoMarshalUnmarshal(t *testing.T) {
	original := &Metric{
		Id:    "test_metric",
		Type:  Metric_GAUGE,
		Value: 123.456,
	}

	// Test marshaling
	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal Metric: %v", err)
	}

	// Test unmarshaling
	unmarshaled := &Metric{}
	err = proto.Unmarshal(data, unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal Metric: %v", err)
	}

	// Compare
	if unmarshaled.Id != original.Id {
		t.Errorf("Unmarshaled.Id = %v, want %v", unmarshaled.Id, original.Id)
	}
	if unmarshaled.Type != original.Type {
		t.Errorf("Unmarshaled.Type = %v, want %v", unmarshaled.Type, original.Type)
	}
	if unmarshaled.Value != original.Value {
		t.Errorf("Unmarshaled.Value = %v, want %v", unmarshaled.Value, original.Value)
	}
}

func TestUpdateMetricsRequestProtoMarshalUnmarshal(t *testing.T) {
	original := &UpdateMetricsRequest{
		Metrics: []*Metric{
			{Id: "gauge1", Type: Metric_GAUGE, Value: 1.23},
			{Id: "counter1", Type: Metric_COUNTER, Delta: 42},
		},
	}

	// Test marshaling
	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal UpdateMetricsRequest: %v", err)
	}

	// Test unmarshaling
	unmarshaled := &UpdateMetricsRequest{}
	err = proto.Unmarshal(data, unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal UpdateMetricsRequest: %v", err)
	}

	// Compare
	if len(unmarshaled.Metrics) != len(original.Metrics) {
		t.Fatalf("Unmarshaled has %d metrics, want %d", len(unmarshaled.Metrics), len(original.Metrics))
	}

	for i, metric := range unmarshaled.Metrics {
		if metric.Id != original.Metrics[i].Id {
			t.Errorf("Metric[%d].Id = %v, want %v", i, metric.Id, original.Metrics[i].Id)
		}
		if metric.Type != original.Metrics[i].Type {
			t.Errorf("Metric[%d].Type = %v, want %v", i, metric.Type, original.Metrics[i].Type)
		}
	}
}
