package control

import (
	"fmt"
	"time"
)

func controlAlertSummary(store Store, opts ServerOptions) (*AlertSummary, error) {
	staleMinutes := opts.NodeStaleMinutes
	if staleMinutes <= 0 {
		staleMinutes = 10
	}
	failedThreshold := opts.FailedTaskAlertThreshold
	if failedThreshold <= 0 {
		failedThreshold = 1
	}
	pendingThreshold := opts.PendingTaskAlertThreshold
	if pendingThreshold <= 0 {
		pendingThreshold = 20
	}

	summary := &AlertSummary{
		Status:    "ok",
		CheckedAt: time.Now().UTC(),
		Thresholds: map[string]interface{}{
			"nodeStaleMinutes":          staleMinutes,
			"failedTaskAlertThreshold":  failedThreshold,
			"pendingTaskAlertThreshold": pendingThreshold,
		},
		Issues: []AlertIssue{},
	}

	nodes, err := store.ListNodes()
	if err != nil {
		return nil, err
	}
	staleCutoff := summary.CheckedAt.Add(-time.Duration(staleMinutes) * time.Minute)
	staleNodes := make([]string, 0)
	for _, node := range nodes {
		if node.LastSeenAt.IsZero() || node.LastSeenAt.Before(staleCutoff) {
			staleNodes = append(staleNodes, node.NodeKey)
		}
	}
	if len(staleNodes) > 0 {
		summary.Issues = append(summary.Issues, AlertIssue{
			Severity: "critical",
			Kind:     "stale_nodes",
			Message:  fmt.Sprintf("%d 个节点超过 %d 分钟未上报心跳", len(staleNodes), staleMinutes),
			Count:    len(staleNodes),
			Details: map[string]interface{}{
				"nodeKeys": staleNodes,
			},
		})
	}

	metrics, err := controlMetricsSnapshot(store)
	if err != nil {
		return nil, err
	}
	if metrics.TaskFailedCount >= int64(failedThreshold) {
		recentFailed, err := store.ListTasks(TaskQuery{Limit: 20, Status: "failed"})
		if err != nil {
			return nil, err
		}
		failedIDs := make([]uint64, 0, len(recentFailed))
		for _, task := range recentFailed {
			failedIDs = append(failedIDs, task.ID)
		}
		summary.Issues = append(summary.Issues, AlertIssue{
			Severity: "warning",
			Kind:     "failed_tasks",
			Message:  fmt.Sprintf("当前失败任务数达到 %d", metrics.TaskFailedCount),
			Count:    int(metrics.TaskFailedCount),
			Details: map[string]interface{}{
				"recentTaskIDs": failedIDs,
			},
		})
	}
	if metrics.TaskPendingCount >= int64(pendingThreshold) {
		summary.Issues = append(summary.Issues, AlertIssue{
			Severity: "warning",
			Kind:     "pending_backlog",
			Message:  fmt.Sprintf("待处理任务积压达到 %d", metrics.TaskPendingCount),
			Count:    int(metrics.TaskPendingCount),
		})
	}
	if len(summary.Issues) > 0 {
		summary.Status = "alert"
	}
	return summary, nil
}
