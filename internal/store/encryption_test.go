package store

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"gopaste/internal/types"
)

// TestHistoryKey 验证仓库与系统凭据的绑定及密钥丢失时保留原数据
func TestHistoryKey(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "vault")
	secrets := map[string][]byte{}
	get := func(id string) ([]byte, error) {
		key, ok := secrets[id]
		if !ok {
			return nil, fs.ErrNotExist
		}
		return key, nil
	}
	set := func(id string, key []byte) error {
		secrets[id] = bytes.Clone(key)
		return nil
	}
	refused := errors.New("凭据库已锁定")
	if _, err := historyKey(dir, get, func(string, []byte) error { return refused }); !errors.Is(err, refused) {
		t.Fatal(err)
	}
	key, err := historyKey(dir, get, set)
	if err != nil || len(key) != 32 {
		t.Fatalf("生成密钥失败：%v", err)
	}
	id := readTestFile(t, filepath.Join(dir, vaultIDFile))
	reloaded, err := historyKey(dir, get, set)
	if err != nil || !bytes.Equal(key, reloaded) || !bytes.Equal(key, secrets["history-"+string(id)]) {
		t.Fatalf("凭据未复用：%v", err)
	}
	s, err := OpenWithKey(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	dbFile := filepath.Join(dir, databaseFile)
	before := readTestFile(t, dbFile)
	clear(secrets)
	if _, err := historyKey(dir, get, set); !errors.Is(err, fs.ErrNotExist) || len(secrets) != 0 {
		t.Fatalf("已有仓库生成了替代密钥：%v", err)
	}
	if err := os.Remove(filepath.Join(dir, vaultIDFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := historyKey(dir, get, set); err == nil || len(secrets) != 0 || !bytes.Equal(before, readTestFile(t, dbFile)) {
		t.Fatal("标识丢失后改变了已有仓库")
	}
}

// readTestFile 读取测试文件并报告读取错误
func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestEncryptedHistory 验证落盘密文、搜索、资源身份认证与错误密钥保护
func TestEncryptedHistory(t *testing.T) {
	dir := t.TempDir()
	s, err := openTestStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	marker := "private-clipboard-record-45678"
	image := []byte("private-image-bytes-45678")
	rec := captureTest(t, s, &types.Record{Kind: types.KindImage, Title: marker}, image, []byte("private-icon"))
	ids, err := s.IDs(types.Tab{ID: "all"}, marker)
	if err != nil || len(ids) != 1 || ids[0].ID != rec.ID {
		t.Fatalf("加密搜索失败：%v", err)
	}
	for _, name := range []string{databaseFile, databaseFile + "-wal"} {
		data := readTestFile(t, filepath.Join(dir, name))
		if len(data) == 0 || bytes.Contains(data, []byte(marker)) || bytes.HasPrefix(data, []byte("SQLite format 3")) {
			t.Fatalf("%s 未加密", name)
		}
	}
	imagePath := s.BlobPath(rec.Image)
	sealed := readTestFile(t, imagePath)
	if filepath.Base(imagePath) == rec.Image || bytes.Contains(sealed, image) {
		t.Fatal("图片内容或文件名仍为明文")
	}
	icon := readTestFile(t, s.BlobPath(rec.Source.Icon))
	tampered := bytes.Clone(sealed)
	tampered[len(tampered)-1] ^= 1
	for _, invalid := range [][]byte{icon, tampered, sealed[:len(sealed)-1], {}} {
		if err := os.WriteFile(imagePath, invalid, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ReadBlob(rec.Image); err == nil {
			t.Fatal("资源身份认证未拒绝替换或截断")
		}
	}
	if err := os.WriteFile(imagePath, sealed, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, databaseFile)
	before := readTestFile(t, dbPath)
	if bytes.Contains(before, []byte(marker)) {
		t.Fatal("检查点写入了明文")
	}
	if bad, err := OpenWithKey(dir, bytes.Repeat([]byte{1}, 32)); err == nil {
		bad.Close()
		t.Fatal("错误密钥打开了数据库")
	}
	if !bytes.Equal(before, readTestFile(t, dbPath)) {
		t.Fatal("错误密钥改变了原数据库")
	}
	s, err = openTestStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.ReadBlob(rec.Image)
	if err != nil || !bytes.Equal(got, image) {
		t.Fatalf("重启后图片解密失败：%v", err)
	}
}

// TestPlaintextWALMigration 验证旧数据库的未归并事务、设置与图片在迁移中断后可恢复
func TestPlaintextWALMigration(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(t.TempDir(), legacyDatabaseFile)
	db := connectDatabase(source, nil)
	defer db.Close()
	settings, err := settingsStatement(types.DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction(db, []statement{
		{SQL: "CREATE TABLE records (id TEXT PRIMARY KEY, position INTEGER, kind TEXT, list_id TEXT, meta TEXT, summary TEXT, payload TEXT, search TEXT, extensions TEXT)"},
		{SQL: "CREATE TABLE settings (id INTEGER PRIMARY KEY, data TEXT)"},
		settings, {SQL: "PRAGMA user_version=2"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatal(err)
	}
	image := []byte("original-wal-picture")
	name := blobName(image, "png")
	r := &types.Record{ID: "wal", Kind: types.KindImage, Title: "only-in-legacy-wal", Image: name, ListID: "saved"}
	stmt, err := recordStatement(r, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction(db, []statement{stmt}); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal"} {
		data := readTestFile(t, source+suffix)
		if bytes.Contains(data, []byte(r.Title)) != (suffix == "-wal") {
			t.Fatal("测试记录应仅存在于旧 WAL")
		}
		if err := os.WriteFile(filepath.Join(dir, legacyDatabaseFile+suffix), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	oldBlobs := filepath.Join(dir, "blobs")
	if err := os.Mkdir(oldBlobs, 0o700); err != nil {
		t.Fatal(err)
	}
	oldImage := filepath.Join(oldBlobs, name)
	if err := os.WriteFile(oldImage, image, 0o600); err != nil {
		t.Fatal(err)
	}
	v, err := newVaultCipher(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	newBlobs := filepath.Join(dir, "blobs.enc")
	if err := os.Mkdir(newBlobs, 0o700); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(newBlobs, v.diskName(name)+".tmp")
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	if s, err := openTestStore(dir); err == nil {
		s.Close()
		t.Fatal("资源未能写入时迁移应中止")
	}
	if _, err := os.Stat(filepath.Join(dir, legacyDatabaseFile)); err != nil || !bytes.Equal(readTestFile(t, oldImage), image) {
		t.Fatal("迁移失败丢失了旧库或原图")
	}
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	s, err := openTestStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, ok := s.Get(r.ID)
	if !ok || got.Title != r.Title || got.ListID != r.ListID || s.Settings().Hotkey != types.DefaultSettings().Hotkey {
		t.Fatal("未恢复旧 WAL 数据或设置")
	}
	data, err := s.ReadBlob(got.Image)
	if err != nil || !bytes.Equal(data, image) {
		t.Fatalf("迁移图片无效：%v", err)
	}
	for _, path := range []string{oldImage, filepath.Join(dir, legacyDatabaseFile), filepath.Join(dir, legacyDatabaseFile+"-wal")} {
		if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("迁移后保留了明文 %s", path)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	newLegacy := filepath.Join(dir, legacyDatabaseFile)
	if err := os.WriteFile(newLegacy, readTestFile(t, source), 0o600); err != nil {
		t.Fatal(err)
	}
	if reopened, err := openTestStore(dir); err == nil {
		reopened.Close()
		t.Fatal("应提示用户保留回退旧版本后写入的历史")
	}
	if _, err := os.Stat(newLegacy); err != nil {
		t.Fatal("删除了旧版本重新写入的历史")
	}
}
