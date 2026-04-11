package control

import (
	"fmt"
	"strings"
	"time"
)

type metricsReporter interface {
	MetricsSnapshot() (*MetricsSnapshot, error)
}

func controlMetricsSnapshot(store Store) (*MetricsSnapshot, error) {
	if reporter, ok := store.(metricsReporter); ok {
		return reporter.MetricsSnapshot()
	}
	return &MetricsSnapshot{
		Backend:   "unknown",
		Healthy:   true,
		CheckedAt: time.Now().UTC(),
	}, nil
}

func renderPrometheusMetrics(snapshot *MetricsSnapshot) string {
	if snapshot == nil {
		snapshot = &MetricsSnapshot{
			Backend: "unknown",
			Healthy: false,
		}
	}

	var b strings.Builder
	writeMetricHelp(&b, "trojan_control_up", "Whether the control plane is healthy.")
	writeGauge(&b, "trojan_control_up", nil, boolGauge(snapshot.Healthy))

	writeMetricHelp(&b, "trojan_control_store_info", "Control plane backend information.")
	writeGauge(&b, "trojan_control_store_info", map[string]string{
		"backend": snapshot.Backend,
	}, 1)

	writeMetricHelp(&b, "trojan_control_nodes_total", "Number of registered nodes.")
	writeGauge(&b, "trojan_control_nodes_total", nil, snapshot.NodeCount)
	writeMetricHelp(&b, "trojan_control_nodes_active", "Number of recently active nodes.")
	writeGauge(&b, "trojan_control_nodes_active", nil, snapshot.ActiveNodeCount)

	writeMetricHelp(&b, "trojan_control_users_total", "Number of control-plane users.")
	writeGauge(&b, "trojan_control_users_total", nil, snapshot.UserCount)
	writeMetricHelp(&b, "trojan_control_admins_total", "Number of control-plane admins.")
	writeGauge(&b, "trojan_control_admins_total", nil, snapshot.AdminCount)

	writeMetricHelp(&b, "trojan_control_tasks_total", "Number of control-plane tasks by status.")
	writeGauge(&b, "trojan_control_tasks_total", map[string]string{"status": "all"}, snapshot.TaskCount)
	writeGauge(&b, "trojan_control_tasks_total", map[string]string{"status": "pending"}, snapshot.TaskPendingCount)
	writeGauge(&b, "trojan_control_tasks_total", map[string]string{"status": "running"}, snapshot.TaskRunningCount)
	writeGauge(&b, "trojan_control_tasks_total", map[string]string{"status": "succeeded"}, snapshot.TaskSucceededCount)
	writeGauge(&b, "trojan_control_tasks_total", map[string]string{"status": "failed"}, snapshot.TaskFailedCount)

	writeMetricHelp(&b, "trojan_control_task_events_total", "Number of task lifecycle events.")
	writeGauge(&b, "trojan_control_task_events_total", nil, snapshot.TaskEventCount)
	writeMetricHelp(&b, "trojan_control_audit_logs_total", "Number of control-plane audit logs.")
	writeGauge(&b, "trojan_control_audit_logs_total", nil, snapshot.AuditLogCount)
	writeMetricHelp(&b, "trojan_control_usage_reports_total", "Number of usage snapshots.")
	writeGauge(&b, "trojan_control_usage_reports_total", nil, snapshot.UsageCount)

	return b.String()
}

func writeMetricHelp(builder *strings.Builder, name string, help string) {
	builder.WriteString("# HELP ")
	builder.WriteString(name)
	builder.WriteString(" ")
	builder.WriteString(help)
	builder.WriteString("\n# TYPE ")
	builder.WriteString(name)
	builder.WriteString(" gauge\n")
}

func writeGauge(builder *strings.Builder, name string, labels map[string]string, value int64) {
	builder.WriteString(name)
	if len(labels) > 0 {
		builder.WriteString("{")
		first := true
		for key, labelValue := range labels {
			if !first {
				builder.WriteString(",")
			}
			first = false
			builder.WriteString(key)
			builder.WriteString("=\"")
			builder.WriteString(escapePrometheusLabel(labelValue))
			builder.WriteString("\"")
		}
		builder.WriteString("}")
	}
	builder.WriteString(" ")
	builder.WriteString(fmt.Sprintf("%d", value))
	builder.WriteString("\n")
}

func escapePrometheusLabel(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\n", "\\n")
	return value
}

func boolGauge(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
