package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	sqlcipher "github.com/0xCarbon/go-sqlite3"
)

// cipherConnector 为每个数据库连接在首次读取前设置原始密钥
type cipherConnector struct {
	path   string
	engine sqlcipher.SQLiteDriver
}

// Driver 返回负责打开加密连接的 SQLCipher 驱动
func (c *cipherConnector) Driver() driver.Driver { return &c.engine }

// Connect 使用当前仓库的数据库密钥建立连接
func (c *cipherConnector) Connect(context.Context) (driver.Conn, error) { return c.engine.Open(c.path) }

// connectDatabase 打开仅使用一个连接的 SQLCipher 数据库
func connectDatabase(path string, key []byte) *sql.DB {
	name := filepath.ToSlash(path)
	if strings.HasPrefix(name, "//") {
		name = "//" + name
	}
	uri := (&url.URL{Path: name}).EscapedPath()
	db := sql.OpenDB(&cipherConnector{path: "file:" + uri + "?_synchronous=FULL", engine: sqlcipher.SQLiteDriver{EncryptionKeyBytes: key}})
	db.SetMaxOpenConns(1)
	return db
}

// statement 保存同一事务中执行的语句与参数
type statement struct {
	SQL  string
	Args []any
}

// transaction 在同一事务内提交历史与设置变更
func transaction(db *sql.DB, statements []statement) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt.SQL, stmt.Args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// migrateDatabase 将旧 SQLite 快照连同已提交的 WAL 数据复制到新加密库
func migrateDatabase(dir string, key []byte) error {
	dest := filepath.Join(dir, databaseFile)
	if _, err := os.Stat(dest); !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	source := filepath.Join(dir, legacyDatabaseFile)
	if _, err := os.Stat(source); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	tmp := dest + ".migrating"
	// 旧数据库始终保留至新库提交，重试时从旧库重新导出
	for _, suffix := range []string{"", "-journal", "-wal", "-shm"} {
		if err := os.Remove(tmp + suffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	db := connectDatabase(tmp, key)
	defer db.Close()
	if _, err := db.Exec("ATTACH DATABASE ? AS legacy KEY ''", source); err != nil {
		return err
	}
	var version int
	if err := db.QueryRow("PRAGMA legacy.user_version").Scan(&version); err != nil {
		return err
	}
	if version > 2 {
		return fmt.Errorf("store: unsupported database version %d", version)
	}
	if _, err := db.Exec("SELECT sqlcipher_export('main', 'legacy')"); err != nil {
		return err
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version=%d", version)); err != nil {
		return err
	}
	var ok string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&ok); err != nil {
		return err
	}
	if ok != "ok" {
		return errors.New("store: invalid migrated database")
	}
	if err := db.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}
