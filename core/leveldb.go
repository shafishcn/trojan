package core

import (
	"os"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
)

var (
	dbInstance *leveldb.DB
	dbOnce     sync.Once
	dbErr      error
)

func dbPath() string {
	path := os.Getenv("TROJAN_MANAGER_DB_PATH")
	if path != "" {
		return path
	}
	return "/var/lib/trojan-manager"
}

// getDB 获取全局LevelDB实例(单例)
func getDB() (*leveldb.DB, error) {
	dbOnce.Do(func() {
		dbInstance, dbErr = leveldb.OpenFile(dbPath(), nil)
	})
	return dbInstance, dbErr
}

// GetValue 获取leveldb值
func GetValue(key string) (string, error) {
	db, err := getDB()
	if err != nil {
		return "", err
	}
	result, err := db.Get([]byte(key), nil)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// SetValue 设置leveldb值
func SetValue(key string, value string) error {
	db, err := getDB()
	if err != nil {
		return err
	}
	return db.Put([]byte(key), []byte(value), nil)
}

// DelValue 删除值
func DelValue(key string) error {
	db, err := getDB()
	if err != nil {
		return err
	}
	return db.Delete([]byte(key), nil)
}
