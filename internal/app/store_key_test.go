package app

import "gopaste/internal/store"

// openTestStore 使用固定测试密钥打开隔离数据目录
func openTestStore(dir string) (*store.Store, error) { return store.OpenWithKey(dir, make([]byte, 32)) }
