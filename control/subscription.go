package control

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"trojan/asset"
)

// BuildClashSubscription renders a multi-node Clash subscription for one user.
func BuildClashSubscription(user *ControlUser, nodes []Node) (string, error) {
	if user == nil {
		return "", fmt.Errorf("user is required")
	}
	if len(nodes) == 0 {
		return "", fmt.Errorf("no bound nodes")
	}

	passwordBytes, err := base64.StdEncoding.DecodeString(user.Password)
	if err != nil {
		return "", fmt.Errorf("decode password: %w", err)
	}
	password := string(passwordBytes)

	var proxyLines []string
	var proxyNames []string
	for _, node := range nodes {
		server, port := resolveNodeAddress(node)
		name := node.Name
		if name == "" {
			name = fmt.Sprintf("%s:%d", server, port)
		}
		proxyNames = append(proxyNames, name)
		proxyLines = append(proxyLines, fmt.Sprintf("  - {name: %s, server: %s, port: %d, type: trojan, password: %s, sni: %s}", name, server, port, password, resolveSNI(node, server)))
	}

	var proxyGroupLines []string
	for _, name := range proxyNames {
		proxyGroupLines = append(proxyGroupLines, "      - "+name)
	}

	return fmt.Sprintf(`proxies:
%s

proxy-groups:
  - name: PROXY
    type: select
    proxies:
%s

%s
`, strings.Join(proxyLines, "\n"), strings.Join(proxyGroupLines, "\n"), string(asset.GetAsset("clash-rules.yaml"))), nil
}

// BuildTrojanLinks renders one trojan:// link per bound node.
func BuildTrojanLinks(user *ControlUser, nodes []Node) (string, error) {
	if user == nil {
		return "", fmt.Errorf("user is required")
	}
	if len(nodes) == 0 {
		return "", fmt.Errorf("no bound nodes")
	}

	passwordBytes, err := base64.StdEncoding.DecodeString(user.Password)
	if err != nil {
		return "", fmt.Errorf("decode password: %w", err)
	}
	password := string(passwordBytes)

	links := make([]string, 0, len(nodes))
	for _, node := range nodes {
		server, port := resolveNodeAddress(node)
		name := node.Name
		if name == "" {
			name = fmt.Sprintf("%s:%d", server, port)
		}
		remark := url.QueryEscape(name)
		links = append(links, fmt.Sprintf("trojan://%s@%s:%d#%s", password, server, port, remark))
	}
	return strings.Join(links, "\n"), nil
}

func resolveNodeAddress(node Node) (string, int) {
	if node.DomainName != "" {
		return node.DomainName, resolveNodePort(node)
	}
	if node.Endpoint != "" {
		host, port, err := net.SplitHostPort(node.Endpoint)
		if err == nil {
			parsedPort, parseErr := strconv.Atoi(port)
			if parseErr == nil {
				return host, parsedPort
			}
		}
		return node.Endpoint, resolveNodePort(node)
	}
	if node.PublicIP != "" {
		return node.PublicIP, resolveNodePort(node)
	}
	return node.NodeKey, resolveNodePort(node)
}

func resolveNodePort(node Node) int {
	if node.Port > 0 {
		return node.Port
	}
	return 443
}

func resolveSNI(node Node, fallback string) string {
	if node.DomainName != "" {
		return node.DomainName
	}
	return fallback
}
