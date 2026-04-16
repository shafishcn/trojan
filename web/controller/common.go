package controller

import (
	"github.com/robfig/cron/v3"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/net"
	"sync"
	"time"
	"trojan/asset"
	"trojan/core"
	"trojan/trojan"
)

// ResponseBody 结构体
type ResponseBody struct {
	Duration string
	Data     interface{}
	Msg      string
}

type speedInfo struct {
	Up   uint64
	Down uint64
}

var si *speedInfo
var siMu sync.RWMutex

// TimeCost web函数执行用时统计方法
func TimeCost(start time.Time, body *ResponseBody) {
	body.Duration = time.Since(start).String()
}

func clashRules() string {
	rules, err := core.GetValue("clash-rules")
	if err == nil && rules != "" {
		return rules
	}
	return string(asset.GetAsset("clash-rules.yaml"))
}

func getSpeedInfo() *speedInfo {
	siMu.RLock()
	defer siMu.RUnlock()
	if si == nil {
		return &speedInfo{}
	}
	result := *si
	return &result
}

// Version 获取版本信息
func Version() *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)
	responseBody.Data = map[string]string{
		"version":       trojan.MVersion,
		"buildDate":     trojan.BuildDate,
		"goVersion":     trojan.GoVersion,
		"gitVersion":    trojan.GitVersion,
		"trojanVersion": trojan.Version(),
		"trojanUptime":  trojan.UpTime(),
		"trojanType":    trojan.Type(),
	}
	return &responseBody
}

// SetLoginInfo 设置登录页信息
func SetLoginInfo(title string) *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)
	err := core.SetValue("login_title", title)
	if err != nil {
		responseBody.Msg = err.Error()
	}
	return &responseBody
}

// SetDomain 设置域名
func SetDomain(domain string) *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)
	trojan.SetDomain(domain)
	return &responseBody
}

// SetClashRules 设置clash规则
func SetClashRules(rules string) *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)
	if err := core.SetValue("clash-rules", rules); err != nil {
		responseBody.Msg = err.Error()
	}
	return &responseBody
}

// ResetClashRules 重置clash规则
func ResetClashRules() *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)
	if err := core.DelValue("clash-rules"); err != nil {
		responseBody.Msg = err.Error()
	}
	responseBody.Data = clashRules()
	return &responseBody
}

// GetClashRules 获取clash规则
func GetClashRules() *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)
	responseBody.Data = clashRules()
	return &responseBody
}

// SetTrojanType 设置trojan类型
func SetTrojanType(tType string) *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)
	err := trojan.SwitchType(tType)
	if err != nil {
		responseBody.Msg = err.Error()
	}
	return &responseBody
}

// CollectTask 启动收集主机信息任务
func CollectTask() {
	var recvCount, sentCount uint64
	c := cron.New()
	lastIO, err := net.IOCounters(true)
	var lastRecvCount, lastSentCount uint64
	if err == nil {
		for _, k := range lastIO {
			lastRecvCount = lastRecvCount + k.BytesRecv
			lastSentCount = lastSentCount + k.BytesSent
		}
	}
	siMu.Lock()
	si = &speedInfo{}
	siMu.Unlock()
	_, err = c.AddFunc("@every 2s", func() {
		result, err := net.IOCounters(true)
		if err != nil {
			return
		}
		recvCount, sentCount = 0, 0
		for _, k := range result {
			recvCount = recvCount + k.BytesRecv
			sentCount = sentCount + k.BytesSent
		}
		siMu.Lock()
		si.Up = (sentCount - lastSentCount) / 2
		si.Down = (recvCount - lastRecvCount) / 2
		siMu.Unlock()
		lastSentCount = sentCount
		lastRecvCount = recvCount
		lastIO = result
	})
	if err != nil {
		return
	}
	c.Start()
}

// ServerInfo 获取服务器信息
func ServerInfo() *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)
	data := map[string]interface{}{
		"cpu":      []float64{},
		"speed":    getSpeedInfo(),
		"netCount": map[string]int{"tcp": 0, "udp": 0},
	}
	var warnings []string
	if cpuPercent, err := cpu.Percent(0, false); err == nil {
		data["cpu"] = cpuPercent
	} else {
		warnings = append(warnings, "cpu: "+err.Error())
	}
	if vmInfo, err := mem.VirtualMemory(); err == nil {
		data["memory"] = vmInfo
	} else {
		warnings = append(warnings, "memory: "+err.Error())
	}
	if smInfo, err := mem.SwapMemory(); err == nil {
		data["swap"] = smInfo
	} else {
		warnings = append(warnings, "swap: "+err.Error())
	}
	if diskInfo, err := disk.Usage("/"); err == nil {
		data["disk"] = diskInfo
	} else {
		warnings = append(warnings, "disk: "+err.Error())
	}
	if loadInfo, err := load.Avg(); err == nil {
		data["load"] = loadInfo
	} else {
		warnings = append(warnings, "load: "+err.Error())
	}
	netCount := map[string]int{"tcp": 0, "udp": 0}
	if tcpCon, err := net.Connections("tcp"); err == nil {
		netCount["tcp"] = len(tcpCon)
	} else {
		warnings = append(warnings, "tcp connections: "+err.Error())
	}
	if udpCon, err := net.Connections("udp"); err == nil {
		netCount["udp"] = len(udpCon)
	} else {
		warnings = append(warnings, "udp connections: "+err.Error())
	}
	data["netCount"] = netCount
	if len(warnings) > 0 {
		data["warnings"] = warnings
	}
	responseBody.Data = data
	return &responseBody
}
