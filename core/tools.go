package core

import (
	"bufio"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"trojan/util"
)

func escapeSQLString(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	return strings.ReplaceAll(value, "'", "''")
}

// UpgradeDB 升级数据库表结构以及迁移数据
func (mysql *Mysql) UpgradeDB() error {
	db := mysql.GetDB()
	if db == nil {
		return errors.New("can't connect mysql")
	}
	// SHOW COLUMNS 返回多列，只取第一列 Field
	var field, colType, colNull, colKey sql.NullString
	var colDefault, colExtra sql.NullString
	err := db.QueryRow("SHOW COLUMNS FROM users LIKE 'passwordShow'").Scan(&field, &colType, &colNull, &colKey, &colDefault, &colExtra)
	if err == sql.ErrNoRows {
		fmt.Println(util.Yellow("正在进行数据库升级, 请稍等.."))
		if _, err := db.Exec("ALTER TABLE users ADD COLUMN passwordShow VARCHAR(255) NOT NULL AFTER password;"); err != nil {
			fmt.Println(err)
			return err
		}
		userList, err := mysql.GetData()
		if err != nil {
			fmt.Println(err)
			return err
		}
		for _, user := range userList {
			pass, _ := GetValue(fmt.Sprintf("%s_pass", user.Username))
			if pass != "" {
				base64Pass := base64.StdEncoding.EncodeToString([]byte(pass))
				if _, err := db.Exec("UPDATE users SET passwordShow=? WHERE id=?", base64Pass, user.ID); err != nil {
					fmt.Println(err)
					return err
				}
				DelValue(fmt.Sprintf("%s_pass", user.Username))
			}
		}
	} else if err != nil {
		return err
	}
	err = db.QueryRow("SHOW COLUMNS FROM users LIKE 'useDays'").Scan(&field, &colType, &colNull, &colKey, &colDefault, &colExtra)
	if err == sql.ErrNoRows {
		fmt.Println(util.Yellow("正在进行数据库升级, 请稍等.."))
		if _, err := db.Exec(`
ALTER TABLE users
ADD COLUMN useDays int(10) DEFAULT 0,
ADD COLUMN expiryDate char(10) DEFAULT '';
`); err != nil {
			fmt.Println(err)
			return err
		}
	} else if err != nil {
		return err
	}
	var tableName sql.NullString
	err = db.QueryRow("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_NAME = 'users' AND TABLE_SCHEMA = ? AND TABLE_COLLATION LIKE 'utf8%'",
		mysql.Database).Scan(&tableName)
	if err == sql.ErrNoRows {
		tempFile, createErr := os.CreateTemp("", "trojan-upgrade-*.sql")
		if createErr != nil {
			return createErr
		}
		tempPath := tempFile.Name()
		if closeErr := tempFile.Close(); closeErr != nil {
			_ = os.Remove(tempPath)
			return closeErr
		}
		defer os.Remove(tempPath)

		if err := mysql.DumpSql(tempPath); err != nil {
			return fmt.Errorf("dump sql for utf8 migration: %w", err)
		}
		if err := mysql.ExecSql(tempPath); err != nil {
			return fmt.Errorf("replay sql for utf8 migration: %w", err)
		}
	} else if err != nil {
		return err
	}
	return nil
}

// DumpSql 导出sql
func (mysql *Mysql) DumpSql(filePath string) error {
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	writer.WriteString("DROP TABLE IF EXISTS users;")
	writer.WriteString(CreateTableSql)
	db := mysql.GetDB()
	if db == nil {
		return errors.New("can't connect mysql")
	}
	userList, err := queryUserListParams(db, "SELECT * FROM users")
	if err != nil {
		return err
	}
	for _, user := range userList {
		writer.WriteString(fmt.Sprintf(`
INSERT INTO users(username, password, passwordShow, quota, download, upload, useDays, expiryDate) VALUES ('%s','%s','%s', %d, %d, %d, %d, '%s');`,
			escapeSQLString(user.Username),
			escapeSQLString(user.EncryptPass),
			escapeSQLString(user.Password),
			user.Quota,
			user.Download,
			user.Upload,
			user.UseDays,
			escapeSQLString(user.ExpiryDate)))
	}
	writer.WriteString("\n")
	return writer.Flush()
}

// ExecSql 执行sql
func (mysql *Mysql) ExecSql(filePath string) error {
	db := mysql.GetDB()
	if db == nil {
		return errors.New("can't connect mysql")
	}
	fileByte, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	sqlStr := string(fileByte)
	sqls := strings.Split(strings.Replace(sqlStr, "\r\n", "\n", -1), ";\n")
	for _, s := range sqls {
		s = strings.TrimSpace(s)
		if s != "" {
			if _, err = db.Exec(s); err != nil {
				return err
			}
		}
	}
	return nil
}
