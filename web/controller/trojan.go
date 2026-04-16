package controller

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"github.com/gin-gonic/gin"
	ws "github.com/gorilla/websocket"
	"io"
	"path/filepath"
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

func parseCSVInt64(value string, lineNo int, field string) (int64, error) {
	result, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("CSV第%d行字段%s不是有效整数: %w", lineNo, field, err)
	}
	return result, nil
}

func parseCSVUint64(value string, lineNo int, field string) (uint64, error) {
	result, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("CSV第%d行字段%s不是有效无符号整数: %w", lineNo, field, err)
	}
	return result, nil
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
	if !strings.EqualFold(filepath.Ext(filename), ".csv") {
		responseBody.Msg = "仅支持导入csv格式的文件"
		return &responseBody
	}
	reader := csv.NewReader(bufio.NewReader(file))
	var userList []*core.User
	lineNo := 0
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
		lineNo++
		quota, err := parseCSVInt64(line[4], lineNo, "quota")
		if err != nil {
			responseBody.Msg = err.Error()
			return &responseBody
		}
		download, err := parseCSVUint64(line[5], lineNo, "download")
		if err != nil {
			responseBody.Msg = err.Error()
			return &responseBody
		}
		upload, err := parseCSVUint64(line[6], lineNo, "upload")
		if err != nil {
			responseBody.Msg = err.Error()
			return &responseBody
		}
		useDays, err := strconv.Atoi(line[7])
		if err != nil {
			responseBody.Msg = fmt.Sprintf("CSV第%d行字段useDays不是有效整数: %v", lineNo, err)
			return &responseBody
		}
		if useDays < 0 {
			responseBody.Msg = fmt.Sprintf("CSV第%d行字段useDays不能为负数", lineNo)
			return &responseBody
		}
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
	if db == nil {
		responseBody.Msg = "can't connect mysql"
		return &responseBody
	}
	tx, err := db.Begin()
	if err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DROP TABLE IF EXISTS users;"); err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	if _, err = tx.Exec(core.CreateTableSql); err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	for _, user := range userList {
		if _, err = tx.Exec(
			"INSERT INTO users(username, password, passwordShow, quota, download, upload, useDays, expiryDate) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			user.Username, user.EncryptPass, user.Password, user.Quota, user.Download, user.Upload, user.UseDays, user.ExpiryDate); err != nil {
			responseBody.Msg = err.Error()
			return &responseBody
		}
	}
	if err := tx.Commit(); err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
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
		if err := wr.Write(singleUser); err != nil {
			responseBody.Msg = err.Error()
			return &responseBody
		}
	}
	wr.Flush()
	if err := wr.Error(); err != nil {
		responseBody.Msg = err.Error()
		return &responseBody
	}
	c.Writer.Header().Set("Content-type", "application/octet-stream")
	c.Writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment;filename=%s", fmt.Sprintf("%s.csv", mysql.Database)))
	c.String(200, dataBytes.String())
	return nil
}
