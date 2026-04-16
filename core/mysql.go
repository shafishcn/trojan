package core

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"strconv"

	// mysql sql驱动
	_ "github.com/go-sql-driver/mysql"
	mysqlDriver "github.com/go-sql-driver/mysql"
)

// Mysql 结构体
type Mysql struct {
	Enabled    bool   `json:"enabled"`
	ServerAddr string `json:"server_addr"`
	ServerPort int    `json:"server_port"`
	Database   string `json:"database"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Cafile     string `json:"cafile"`
}

// User 用户表记录结构体
type User struct {
	ID          uint
	Username    string
	Password    string
	EncryptPass string
	Quota       int64
	Download    uint64
	Upload      uint64
	UseDays     uint
	ExpiryDate  string
}

// PageQuery 分页查询的结构体
type PageQuery struct {
	PageNum  int
	CurPage  int
	Total    int
	PageSize int
	DataList []*User
}

// CreateTableSql 创表sql
var CreateTableSql = `
CREATE TABLE IF NOT EXISTS users (
    id INT UNSIGNED NOT NULL AUTO_INCREMENT,
    username VARCHAR(64) NOT NULL,
    password CHAR(56) NOT NULL,
    passwordShow VARCHAR(255) NOT NULL,
    quota BIGINT NOT NULL DEFAULT 0,
    download BIGINT UNSIGNED NOT NULL DEFAULT 0,
    upload BIGINT UNSIGNED NOT NULL DEFAULT 0,
    useDays int(10) DEFAULT 0,
    expiryDate char(10) DEFAULT '',
    PRIMARY KEY (id),
    INDEX (password)
) DEFAULT CHARSET=utf8mb4;
`

var (
	mysqlDB   *sql.DB
	mysqlOnce sync.Once
	mysqlDSN  string
	mysqlMu   sync.Mutex
)

// GetDB 获取mysql数据库连接(连接池复用)
func (mysql *Mysql) GetDB() *sql.DB {
	mysqlMu.Lock()
	defer mysqlMu.Unlock()
	// 屏蔽mysql驱动包的日志输出
	mysqlDriver.SetLogger(log.New(io.Discard, "", 0))
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", mysql.Username, mysql.Password, mysql.ServerAddr, mysql.ServerPort, mysql.Database)

	// 如果 DSN 变了(比如重新配置)，重置连接池
	if mysqlDB != nil && dsn != mysqlDSN {
		mysqlDB.Close()
		mysqlDB = nil
		mysqlOnce = sync.Once{}
	}

	mysqlOnce.Do(func() {
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(5 * time.Minute)
		mysqlDB = db
		mysqlDSN = dsn
	})
	return mysqlDB
}

// SetTestDB 设置测试用的 Mock 数据库连接
func SetTestDB(db *sql.DB) {
	mysqlMu.Lock()
	defer mysqlMu.Unlock()
	mysqlDB = db
	mysqlDSN = ":@tcp(:0)/"
	mysqlOnce = sync.Once{}
	mysqlOnce.Do(func() {}) // 占位标记已初始化，防止被重写
}

// ResetTestDB 重置测试注入的数据库连接
func ResetTestDB() {
	mysqlMu.Lock()
	defer mysqlMu.Unlock()
	if mysqlDB != nil {
		_ = mysqlDB.Close()
	}
	mysqlDB = nil
	mysqlDSN = ""
	mysqlOnce = sync.Once{}
}

// CreateTable 不存在trojan user表则自动创建
func (mysql *Mysql) CreateTable() {
	db := mysql.GetDB()
	if db == nil {
		return
	}
	if _, err := db.Exec(CreateTableSql); err != nil {
		fmt.Println(err)
	}
}

func scanUser(scanner interface{ Scan(...interface{}) error }) (*User, error) {
	var (
		username    string
		encryptPass string
		passShow    string
		download    uint64
		upload      uint64
		quota       int64
		id          uint
		useDays     uint
		expiryDate  string
	)
	if err := scanner.Scan(&id, &username, &encryptPass, &passShow, &quota, &download, &upload, &useDays, &expiryDate); err != nil {
		return nil, err
	}
	return &User{
		ID:          id,
		Username:    username,
		Password:    passShow,
		EncryptPass: encryptPass,
		Download:    download,
		Upload:      upload,
		Quota:       quota,
		UseDays:     useDays,
		ExpiryDate:  expiryDate,
	}, nil
}

func queryUserListParams(db *sql.DB, query string, args ...interface{}) ([]*User, error) {
	var userList []*User
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		userList = append(userList, user)
	}
	return userList, nil
}

func queryUserParams(db *sql.DB, query string, args ...interface{}) (*User, error) {
	row := db.QueryRow(query, args...)
	return scanUser(row)
}

// CreateUser 创建Trojan用户
func (mysql *Mysql) CreateUser(username string, base64Pass string, originPass string) error {
	db := mysql.GetDB()
	if db == nil {
		return errors.New("can't connect mysql")
	}
	encryPass := sha256.Sum224([]byte(originPass))
	if _, err := db.Exec("INSERT INTO users(username, password, passwordShow, quota) VALUES (?, ?, ?, -1)",
		username, fmt.Sprintf("%x", encryPass), base64Pass); err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

// UpdateUser 更新Trojan用户名和密码
func (mysql *Mysql) UpdateUser(id uint, username string, base64Pass string, originPass string) error {
	db := mysql.GetDB()
	if db == nil {
		return errors.New("can't connect mysql")
	}
	encryPass := sha256.Sum224([]byte(originPass))
	if _, err := db.Exec("UPDATE users SET username=?, password=?, passwordShow=? WHERE id=?",
		username, fmt.Sprintf("%x", encryPass), base64Pass, id); err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

// DeleteUser 删除用户
func (mysql *Mysql) DeleteUser(id uint) error {
	db := mysql.GetDB()
	if db == nil {
		return errors.New("can't connect mysql")
	}
	result, err := db.Exec("DELETE FROM users WHERE id=?", id)
	if err != nil {
		fmt.Println(err)
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("不存在id为%d的用户", id)
	}
	return nil
}

// MonthlyResetData 设置了过期时间的用户，每月定时清空使用流量
func (mysql *Mysql) MonthlyResetData() error {
	db := mysql.GetDB()
	if db == nil {
		return errors.New("can't connect mysql")
	}
	if _, err := db.Exec("UPDATE users SET download=0, upload=0 WHERE useDays != 0 AND quota != 0"); err != nil {
		return err
	}
	return nil
}

// DailyCheckExpire 检查是否有过期，过期了设置流量上限为0
func (mysql *Mysql) DailyCheckExpire() (bool, error) {
	needRestart := false
	now := time.Now()
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return false, err
	}
	yesterday := now.Add(-24 * time.Hour).In(loc)
	yesterdayStr := yesterday.Format("2006-01-02")

	db := mysql.GetDB()
	if db == nil {
		return false, errors.New("can't connect mysql")
	}
	userList, err := queryUserListParams(db, "SELECT * FROM users WHERE quota != 0")
	if err != nil {
		return false, err
	}
	for _, user := range userList {
		if user.ExpiryDate == "" {
			continue
		}
		if yesterdayStr >= user.ExpiryDate {
			if _, err := db.Exec("UPDATE users SET quota=0 WHERE id=?", user.ID); err != nil {
				return false, err
			}
			needRestart = true
		}
	}
	return needRestart, nil
}

// CancelExpire 取消过期时间
func (mysql *Mysql) CancelExpire(id uint) error {
	db := mysql.GetDB()
	if db == nil {
		return errors.New("can't connect mysql")
	}
	if _, err := db.Exec("UPDATE users SET useDays=0, expiryDate='' WHERE id=?", id); err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

// SetExpire 设置过期时间
func (mysql *Mysql) SetExpire(id uint, useDays uint) error {
	now := time.Now()
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		fmt.Println(err)
		return err
	}
	expiryDate := now.Add(time.Duration(useDays) * 24 * time.Hour).In(loc).Format("2006-01-02")

	db := mysql.GetDB()
	if db == nil {
		return errors.New("can't connect mysql")
	}
	if _, err := db.Exec("UPDATE users SET useDays=?, expiryDate=? WHERE id=?", useDays, expiryDate, id); err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

// SetQuota 限制流量
func (mysql *Mysql) SetQuota(id uint, quota int) error {
	db := mysql.GetDB()
	if db == nil {
		return errors.New("can't connect mysql")
	}
	if _, err := db.Exec("UPDATE users SET quota=? WHERE id=?", quota, id); err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

// CleanData 清空流量统计
func (mysql *Mysql) CleanData(id uint) error {
	db := mysql.GetDB()
	if db == nil {
		return errors.New("can't connect mysql")
	}
	if _, err := db.Exec("UPDATE users SET download=0, upload=0 WHERE id=?", id); err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

// CleanDataByName 清空指定用户名流量统计数据
func (mysql *Mysql) CleanDataByName(usernames []string) error {
	if len(usernames) == 0 {
		return nil
	}
	db := mysql.GetDB()
	if db == nil {
		return errors.New("can't connect mysql")
	}
	// 构建参数化的 IN 子句
	placeholders := make([]string, len(usernames))
	args := make([]interface{}, len(usernames))
	for i, name := range usernames {
		placeholders[i] = "?"
		args[i] = name
	}
	query := "UPDATE users SET download=0, upload=0 WHERE BINARY username IN (" + joinStrings(placeholders, ",") + ")"
	if _, err := db.Exec(query, args...); err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

// GetUserByName 通过用户名来获取用户
func (mysql *Mysql) GetUserByName(name string) *User {
	db := mysql.GetDB()
	if db == nil {
		return nil
	}
	user, err := queryUserParams(db, "SELECT * FROM users WHERE BINARY username=?", name)
	if err != nil {
		return nil
	}
	return user
}

// GetUserByPass 通过密码来获取用户
func (mysql *Mysql) GetUserByPass(pass string) *User {
	db := mysql.GetDB()
	if db == nil {
		return nil
	}
	user, err := queryUserParams(db, "SELECT * FROM users WHERE BINARY passwordShow=?", pass)
	if err != nil {
		return nil
	}
	return user
}

// PageList 通过分页获取用户记录
func (mysql *Mysql) PageList(curPage int, pageSize int) (*PageQuery, error) {
	var total int

	db := mysql.GetDB()
	if db == nil {
		return nil, errors.New("连接mysql失败")
	}
	offset := (curPage - 1) * pageSize
	userList, err := queryUserListParams(db, "SELECT * FROM users LIMIT ?, ?", offset, pageSize)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	db.QueryRow("SELECT COUNT(id) FROM users").Scan(&total)
	return &PageQuery{
		CurPage:  curPage,
		PageSize: pageSize,
		Total:    total,
		DataList: userList,
		PageNum:  (total + pageSize - 1) / pageSize,
	}, nil
}

// GetData 获取用户记录
func (mysql *Mysql) GetData(ids ...string) ([]*User, error) {
	db := mysql.GetDB()
	if db == nil {
		return nil, errors.New("连接mysql失败")
	}
	if len(ids) > 0 {
		// 构建参数化 IN 子句
		placeholders := make([]string, len(ids))
		args := make([]interface{}, len(ids))
		for i, id := range ids {
			placeholders[i] = "?"
			idVal, err := strconv.Atoi(id)
			if err != nil {
				return nil, fmt.Errorf("invalid id: %s", id)
			}
			args[i] = idVal
		}
		query := "SELECT * FROM users WHERE id IN (" + joinStrings(placeholders, ",") + ")"
		userList, err := queryUserListParams(db, query, args...)
		if err != nil {
			fmt.Println(err)
			return nil, err
		}
		return userList, nil
	}
	userList, err := queryUserListParams(db, "SELECT * FROM users")
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	return userList, nil
}
