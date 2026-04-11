package controller

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"github.com/gin-gonic/gin"
	ws "github.com/gorilla/websocket"
	"io"
	"strconv"
	"strings"
	"time"
	"trojan/core"
	"trojan/trojan"
	"trojan/util"
)

// Start 启动trojan
func Start() *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)
	trojan.Start()
	return &responseBody
}

// Stop 停止trojan
func Stop() *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)
	trojan.Stop()
	return &responseBody
}

// Restart 重启trojan
func Restart() *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)
	trojan.Restart()
	return &responseBody
}

// Update trojan更新
func Update() *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)
	trojan.InstallTrojan("")
	return &responseBody
}

// SetLogLevel 修改trojan日志等级
func SetLogLevel(level int) *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)
	core.WriteLogLevel(level)
	trojan.Restart()
	return &responseBody
}

// GetLogLevel 获取trojan日志等级
func GetLogLevel() *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)
	config := core.GetConfig()
	responseBody.Data = map[string]interface{}{
		"loglevel": &config.LogLevel,
	}
	return &responseBody
}

// Log 通过ws查看trojan实时日志
func Log(c *gin.Context) {
	var (
		wsConn *util.WsConnection
		err    error
	)
	if wsConn, err = util.InitWebsocket(c.Writer, c.Request); err != nil {
		fmt.Println(err)
		return
	}
	defer wsConn.WsClose()
	param := c.DefaultQuery("line", "300")
	if !util.IsInteger(param) {
		fmt.Println("invalid param: " + param)
		return
	}
	line, _ := strconv.Atoi(param)
	result, err := util.LogChan("trojan", line, wsConn.CloseChan)
	if err != nil {
		fmt.Println(err)
		return
	}
	for line := range result {
		if err := wsConn.WsWrite(ws.TextMessage, []byte(line+"\n")); err != nil {
			fmt.Println("can't send: ", line)
			break
		}
	}
}

// ImportCsv 导入csv文件到trojan数据库
func ImportCsv(c *gin.Context) *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	defer file.Close()
	filename := header.Filename
	if !strings.Contains(filename, ".csv") {
		responseBody.Msg = "仅支持导入csv格式的文件"
		return &responseBody
	}
	reader := csv.NewReader(bufio.NewReader(file))
	var userList []*core.User
	for {
		line, readErr := reader.Read()
		if readErr == io.EOF {
			break
		} else if readErr != nil {
			responseBody.Msg = readErr.Error()
			return &responseBody
		}
		if len(line) < 9 {
			responseBody.Msg = fmt.Sprintf("CSV格式错误: 期望至少9列, 实际%d列", len(line))
			return &responseBody
		}
		quota, _ := strconv.ParseInt(line[4], 10, 64)
		download, _ := strconv.ParseUint(line[5], 10, 64)
		upload, _ := strconv.ParseUint(line[6], 10, 64)
		useDays, _ := strconv.Atoi(line[7])
		userList = append(userList, &core.User{
			Username:    line[1],
			Password:    line[2],
			EncryptPass: line[3],
			Quota:       quota,
			Download:    download,
			Upload:      upload,
			UseDays:     uint(useDays),
			ExpiryDate:  line[8],
		})
	}
	mysql := core.GetMysql()
	db := mysql.GetDB()
	if _, err = db.Exec("DROP TABLE IF EXISTS users;"); err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	if _, err = db.Exec(core.CreateTableSql); err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	for _, user := range userList {
		if _, err = db.Exec(
			"INSERT INTO users(username, password, passwordShow, quota, download, upload, useDays, expiryDate) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			user.Username, user.EncryptPass, user.Password, user.Quota, user.Download, user.Upload, user.UseDays, user.ExpiryDate); err != nil {
			responseBody.Msg = err.Error()
			return &responseBody
		}
	}
	return &responseBody
}

// ExportCsv 导出trojan表数据到csv文件
func ExportCsv(c *gin.Context) *ResponseBody {
	responseBody := ResponseBody{Msg: "success"}
	defer TimeCost(time.Now(), &responseBody)
	var dataBytes = new(bytes.Buffer)
	//设置UTF-8 BOM, 防止中文乱码
	dataBytes.WriteString("\xEF\xBB\xBF")
	mysql := core.GetMysql()
	userList, err := mysql.GetData()
	if err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	wr := csv.NewWriter(dataBytes)
	for _, user := range userList {
		singleUser := []string{
			strconv.FormatUint(uint64(user.ID), 10),
			user.Username,
			user.Password,
			user.EncryptPass,
			strconv.FormatInt(user.Quota, 10),
			strconv.FormatUint(user.Download, 10),
			strconv.FormatUint(user.Upload, 10),
			strconv.FormatUint(uint64(user.UseDays), 10),
			user.ExpiryDate,
		}
		wr.Write(singleUser)
	}
	wr.Flush()
	c.Writer.Header().Set("Content-type", "application/octet-stream")
	c.Writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment;filename=%s", fmt.Sprintf("%s.csv", mysql.Database)))
	c.String(200, dataBytes.String())
	return nil
}

