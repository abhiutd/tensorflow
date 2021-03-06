package predict

import (
	"encoding/json"
	"errors"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
	proto "github.com/rai-project/tensorflow"
	"golang.org/x/net/context"
)

type traceNode struct {
	device string
	node   *proto.NodeExecStats
}

type Trace struct {
	nodes []traceNode
}

func NewTrace(data *proto.StepStats) (*Trace, error) {
	if len(data.GetDevStats()) == 0 {
		return nil, errors.New("no device stats available")
	}

	nodes := []traceNode{}
	for _, dev := range data.GetDevStats() {
		for _, nd := range dev.GetNodeStats() {
			nodes = append(nodes, traceNode{
				device: dev.GetDevice(),
				node:   nd,
			})
		}
	}
	return &Trace{
		nodes: nodes,
	}, nil
}

func (t *Trace) Publish(ctx context.Context, opts ...opentracing.StartSpanOption) error {
	for _, tr := range t.nodes {
		device := tr.device
		node := tr.node
		startTime := time.Unix(0, node.GetAllStartMicros()*int64(time.Microsecond))
		endTime := time.Unix(0, (node.GetAllStartMicros()+node.GetAllEndRelMicros())*int64(time.Microsecond))

		tags := opentracing.Tags{
			"device": device,
			// "all_start_micros":    node.GetAllStartMicros(),
			// "all_end_rel_micros":  node.GetAllEndRelMicros(),
			// "op_start_rel_micros": node.GetOpStartRelMicros(),
			// "op_end_rel_micros":   node.GetOpEndRelMicros(),
			"timeline_label":   node.GetTimelineLabel(),
			"scheduled_micros": node.GetScheduledMicros(),
			"thread_id":        node.GetThreadId(),
			// "start_time":          startTime,
			// "end_time":            endTime,
		}
		memStatsTags := opentracing.Tags{}
		if node.GetMemoryStats() != nil {
			stats := node.GetMemoryStats()
			memStatsTags = opentracing.Tags{
				"host_temp_memory_size":              stats.GetHostTempMemorySize(),
				"device_temp_memory_size":            stats.GetDeviceTempMemorySize(),
				"host_persistent_memory_size":        stats.GetHostPersistentMemorySize(),
				"device_persistent_memory_size":      stats.GetDevicePersistentMemorySize(),
				"host_persistent_tensor_alloc_ids":   stats.GetHostPersistentTensorAllocIds(),
				"device_persistent_tensor_alloc_ids": stats.GetDevicePersistentTensorAllocIds(),
			}
		}

		memAllocTags := opentracing.Tags{}
		if node.GetMemory() != nil {
			stats := node.GetMemory()
			bts, err := json.Marshal(stats)
			if err != nil {
				break
			}
			memAllocTags = opentracing.Tags{
				"memory": string(bts),
			}
		}

		tensorTags := opentracing.Tags{}
		if node.GetReferencedTensor() != nil {
			stats := node.GetReferencedTensor()
			bts, err := json.Marshal(stats)
			if err != nil {
				break
			}
			memAllocTags = opentracing.Tags{
				"tensor": string(bts),
			}
		}

		outputTags := opentracing.Tags{}
		if node.GetOutput() != nil {
			stats := node.GetOutput()
			bts, err := json.Marshal(stats)
			if err != nil {
				break
			}
			memAllocTags = opentracing.Tags{
				"output": string(bts),
			}
		}

		s, _ := opentracing.StartSpanFromContext(
			ctx,
			node.GetNodeName(),
			opentracing.StartTime(startTime),
			tags,
			memStatsTags,
			memAllocTags,
			tensorTags,
			outputTags,
		)
		if s == nil {
			log.WithField("node_name", node.GetNodeName()).
				WithField("tags", tags).
				Error("failed to create span from context")
			continue
		}
		s.FinishWithOptions(opentracing.FinishOptions{
			FinishTime: endTime,
		})
	}

	return nil
}
