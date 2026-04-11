package control

import (
	"context"
	"fmt"
	"time"
)

// AgentConfig configures the node agent loop.
type AgentConfig struct {
	ControlURL         string
	AgentToken         string
	NodeSecret         string
	NodeKey            string
	Name               string
	Region             string
	Provider           string
	Endpoint           string
	Port               int
	TrojanType         string
	TrojanVersion      string
	ManagerVersion     string
	PublicIP           string
	DomainName         string
	Tags               []string
	HeartbeatInterval  time.Duration
	TaskPollInterval   time.Duration
	UsageInterval      time.Duration
	InitialTaskBackoff time.Duration
}

// AgentRuntime coordinates register/heartbeat/task execution for a node.
type AgentRuntime struct {
	client   *Client
	executor *AgentExecutor
	config   AgentConfig
}

// NewAgentRuntime creates a runnable agent instance.
func NewAgentRuntime(config AgentConfig, executor *AgentExecutor) *AgentRuntime {
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = 30 * time.Second
	}
	if config.TaskPollInterval <= 0 {
		config.TaskPollInterval = 15 * time.Second
	}
	if config.UsageInterval <= 0 {
		config.UsageInterval = 60 * time.Second
	}
	if config.InitialTaskBackoff <= 0 {
		config.InitialTaskBackoff = 2 * time.Second
	}
	if executor == nil {
		executor = NewAgentExecutor()
	}
	return &AgentRuntime{
		client:   NewClient(config.ControlURL).WithToken(config.AgentToken).WithNodeAuth(config.NodeKey, config.NodeSecret),
		executor: executor,
		config:   config,
	}
}

// Run starts the agent loop until the context is canceled.
func (a *AgentRuntime) Run(ctx context.Context) error {
	if err := a.register(); err != nil {
		return err
	}
	if err := a.sendHeartbeat(); err != nil {
		return err
	}
	if err := a.processPendingTasks(); err != nil {
		return err
	}
	if err := a.reportUsage(); err != nil {
		fmt.Println("agent usage report error:", err)
	}

	heartbeatTicker := time.NewTicker(a.config.HeartbeatInterval)
	defer heartbeatTicker.Stop()
	taskTicker := time.NewTicker(a.config.TaskPollInterval)
	defer taskTicker.Stop()
	usageTicker := time.NewTicker(a.config.UsageInterval)
	defer usageTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-heartbeatTicker.C:
			if err := a.sendHeartbeat(); err != nil {
				fmt.Println("agent heartbeat error:", err)
			}
		case <-taskTicker.C:
			if err := a.processPendingTasks(); err != nil {
				fmt.Println("agent task poll error:", err)
			}
		case <-usageTicker.C:
			if err := a.reportUsage(); err != nil {
				fmt.Println("agent usage report error:", err)
			}
		}
	}
}

func (a *AgentRuntime) register() error {
	req := RegisterNodeRequest{
		NodeKey:        a.config.NodeKey,
		Name:           a.config.Name,
		Region:         a.config.Region,
		Provider:       a.config.Provider,
		Endpoint:       a.config.Endpoint,
		Port:           a.config.Port,
		TrojanType:     a.config.TrojanType,
		TrojanVersion:  a.config.TrojanVersion,
		ManagerVersion: a.config.ManagerVersion,
		PublicIP:       a.config.PublicIP,
		DomainName:     a.config.DomainName,
		Tags:           a.config.Tags,
		AgentSecret:    a.config.NodeSecret,
	}
	_, err := a.client.RegisterNode(req)
	return err
}

func (a *AgentRuntime) sendHeartbeat() error {
	req := CollectHeartbeat(a.config.NodeKey)
	_, err := a.client.Heartbeat(req)
	return err
}

func (a *AgentRuntime) processPendingTasks() error {
	tasks, err := a.client.PendingTasks(a.config.NodeKey, 20)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		executionToken := GenerateAgentSecret()
		startedTask, err := a.client.StartTask(task.ID, StartTaskRequest{
			NodeKey:        a.config.NodeKey,
			ExecutionToken: executionToken,
		})
		if err != nil {
			fmt.Println("start task error:", err)
			continue
		}
		executionToken = startedTask.ExecutionToken
		result := a.executor.Execute(task)
		if _, err := a.client.FinishTask(task.ID, FinishTaskRequest{
			NodeKey:        a.config.NodeKey,
			ExecutionToken: executionToken,
			Success:        result.Success,
			Message:        result.Message,
			Details:        result.Details,
		}); err != nil {
			fmt.Println("finish task error:", err)
		}
	}
	return nil
}

func (a *AgentRuntime) reportUsage() error {
	req, err := CollectUsage(a.config.NodeKey)
	if err != nil {
		return err
	}
	return a.client.ReportUsage(req)
}
