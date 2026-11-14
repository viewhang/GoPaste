package store

// openTestStore 使用固定测试密钥打开隔离数据目录
func openTestStore(dir string) (*Store, error) { return OpenWithKey(dir, make([]byte, 32)) }
