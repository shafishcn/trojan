package util

import (
	"bufio"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var publicIPProviders = []string{
	"http://api.ipify.org",
	"http://icanhazip.com",
}

func fetchPublicIP(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("unexpected status: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

// PortIsUse 判断端口是否占用
func PortIsUse(port int) bool {
	_, tcpError := net.DialTimeout("tcp", fmt.Sprintf(":%d", port), time.Millisecond*50)
	udpAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf(":%d", port))
	if err != nil {
		return tcpError == nil
	}
	udpConn, udpError := net.ListenUDP("udp", udpAddr)
	if udpConn != nil {
		defer udpConn.Close()
	}
	return tcpError == nil || udpError != nil
}

// RandomPort 获取没占用的随机端口
func RandomPort() int {
	for {
		newPort := rand.Intn(65536-1024) + 1024
		if !PortIsUse(newPort) {
			return newPort
		}
	}
}

// IsExists 检测指定路径文件或者文件夹是否存在
func IsExists(path string) bool {
	_, err := os.Stat(path) //os.Stat获取文件信息
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}

// GetLocalIP 获取本机ipv4地址
func GetLocalIP() string {
	for _, provider := range publicIPProviders {
		ip, err := fetchPublicIP(provider)
		if err == nil && ip != "" {
			return ip
		}
	}
	return ""
}

// InstallPack 安装指定名字软件
func InstallPack(name string) {
	if !CheckCommandExists(name) {
		if CheckCommandExists("yum") {
			ExecCommand("yum install -y " + name)
		} else if CheckCommandExists("apt-get") {
			ExecCommand("apt-get update")
			ExecCommand("apt-get install -y " + name)
		}
	}
}

// OpenPort 开通指定端口
func OpenPort(port int) {
	if CheckCommandExists("firewall-cmd") {
		ExecCommand(fmt.Sprintf("firewall-cmd --zone=public --add-port=%d/tcp --add-port=%d/udp --permanent >/dev/null 2>&1", port, port))
		ExecCommand("firewall-cmd --reload >/dev/null 2>&1")
	} else {
		if len(ExecCommandWithResult(fmt.Sprintf(`iptables -nvL --line-number|grep -w "%d"`, port))) > 0 {
			return
		}
		ExecCommand(fmt.Sprintf("iptables -I INPUT -p tcp --dport %d -j ACCEPT", port))
		ExecCommand(fmt.Sprintf("iptables -I INPUT -p udp --dport %d -j ACCEPT", port))
		ExecCommand(fmt.Sprintf("iptables -I OUTPUT -p udp --sport %d -j ACCEPT", port))
		ExecCommand(fmt.Sprintf("iptables -I OUTPUT -p tcp --sport %d -j ACCEPT", port))
	}
}

// Log 实时打印指定服务日志
func Log(serviceName string, line int) {
	result, err := LogChan(serviceName, line, make(chan byte))
	if err != nil {
		fmt.Println("Error:The command is err: ", err.Error())
		return
	}
	for line := range result {
		fmt.Println(line)
	}
}

func journalctlArgs(serviceName string, line int) []string {
	args := []string{"-f", "-u", serviceName, "-o", "cat"}
	if line < 0 {
		return append(args, "--no-tail")
	}
	return append(args, "-n", strconv.Itoa(line))
}

// LogChan 指定服务实时日志, 返回chan
func LogChan(serviceName string, line int, closeChan chan byte) (chan string, error) {
	cmd := exec.Command("journalctl", journalctlArgs(serviceName, line)...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		fmt.Println("Error:The command is err: ", err.Error())
		return nil, err
	}
	ch := make(chan string, 100)
	stdoutScan := bufio.NewScanner(stdout)
	go func() {
		defer close(ch)
		defer cmd.Wait()
		for stdoutScan.Scan() {
			select {
			case <-closeChan:
				_ = stdout.Close()
				return
			default:
				ch <- stdoutScan.Text()
			}
		}
		if err := stdoutScan.Err(); err != nil {
			fmt.Println("Error:The command is err: ", err.Error())
		}
	}()
	return ch, nil
}
