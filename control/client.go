package control

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is the HTTP client used by node agents.
type Client struct {
	baseURL    string
	token      string
	nodeKey    string
	nodeSecret string
	httpClient *http.Client
}

// NewClient creates a control-plane client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// WithToken sets the bearer token used for requests.
func (c *Client) WithToken(token string) *Client {
	c.token = token
	return c
}

// WithNodeAuth configures per-node request signing.
func (c *Client) WithNodeAuth(nodeKey string, nodeSecret string) *Client {
	c.nodeKey = nodeKey
	c.nodeSecret = nodeSecret
	return c
}

// RegisterNode registers or refreshes a node record.
func (c *Client) RegisterNode(req RegisterNodeRequest) (*Node, error) {
	var body Node
	if err := c.doJSON(http.MethodPost, "/api/agent/register", req, &body); err != nil {
		return nil, err
	}
	return &body, nil
}

// Heartbeat sends the current node state to the control plane.
func (c *Client) Heartbeat(req HeartbeatRequest) (*Node, error) {
	var body Node
	if err := c.doJSON(http.MethodPost, "/api/agent/heartbeat", req, &body); err != nil {
		return nil, err
	}
	return &body, nil
}

// PendingTasks fetches pending tasks for a node.
func (c *Client) PendingTasks(nodeKey string, limit int) ([]Task, error) {
	values := url.Values{}
	values.Set("nodeKey", nodeKey)
	if limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", limit))
	}
	path := "/api/agent/tasks/pending?" + values.Encode()
	var body struct {
		NodeKey string `json:"nodeKey"`
		Tasks   []Task `json:"tasks"`
	}
	if err := c.doJSON(http.MethodGet, path, nil, &body); err != nil {
		return nil, err
	}
	return body.Tasks, nil
}

// StartTask marks a task as running.
func (c *Client) StartTask(taskID uint64, req StartTaskRequest) (*Task, error) {
	var body Task
	if err := c.doJSON(http.MethodPost, fmt.Sprintf("/api/agent/tasks/%d/start", taskID), req, &body); err != nil {
		return nil, err
	}
	return &body, nil
}

// FinishTask reports the final task result.
func (c *Client) FinishTask(taskID uint64, req FinishTaskRequest) (*Task, error) {
	var body Task
	if err := c.doJSON(http.MethodPost, fmt.Sprintf("/api/agent/tasks/%d/result", taskID), req, &body); err != nil {
		return nil, err
	}
	return &body, nil
}

// ReportUsage sends current user usage snapshots to the control plane.
func (c *Client) ReportUsage(req UsageReportRequest) error {
	return c.doJSON(http.MethodPost, "/api/agent/usage", req, nil)
}

func (c *Client) doJSON(method, path string, reqBody interface{}, responseBody interface{}) error {
	var bodyReader io.Reader
	if reqBody != nil {
		payload, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return err
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.nodeKey != "" && c.nodeSecret != "" {
		timestamp := time.Now().UTC().Format(time.RFC3339)
		body := []byte{}
		if reqBody != nil {
			payload, err := json.Marshal(reqBody)
			if err != nil {
				return err
			}
			body = payload
		}
		req.Header.Set(agentNodeKeyHeader, c.nodeKey)
		req.Header.Set(agentTimestampHeader, timestamp)
		signingKey, err := hashAgentSecret(c.nodeSecret)
		if err != nil {
			return err
		}
		req.Header.Set(agentSignatureHeader, signAgentRequest(method, req.URL.RequestURI(), c.nodeKey, timestamp, body, signingKey))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var envelope ResponseBody
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		if envelope.Message != "" {
			return fmt.Errorf(envelope.Message)
		}
		return fmt.Errorf("request failed: %s", resp.Status)
	}
	if responseBody == nil || envelope.Data == nil {
		return nil
	}

	payload, err := json.Marshal(envelope.Data)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, responseBody)
}
