// 全量数据包加载：versions.json + <ver>.zip 下载、解码、持久化、运行时热重载。
package dob

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	// ManifestFile 包内清单文件名。
	ManifestFile = "manifest.json"
	// ImgsManifestFile 镜像清单文件名。
	ImgsManifestFile = "imgs.json"
	// ModulesPrefix 模块前缀。
	ModulesPrefix = "modules/"
	// DefaultVersionsFile 版本列表文件名。
	DefaultVersionsFile = "versions.json"
)

// RevivePackedValue 还原打包时的特殊类型（对齐前端 revivePackedValue，递归还原）。
func RevivePackedValue(value any) any {
	switch t := value.(type) {
	case []any:
		for i, item := range t {
			t[i] = RevivePackedValue(item)
		}
		return t
	case map[string]any:
		kind, _ := t["__dnaPackType"].(string)
		switch kind {
		case "Undefined":
			return nil
		case "Date":
			if s, ok := t["value"].(string); ok {
				if tm, err := time.Parse(time.RFC3339, s); err == nil {
					return tm
				}
				return s
			}
			return t["value"]
		case "Set":
			if l, ok := t["value"].([]any); ok {
				out := make([]any, 0, len(l))
				for _, v := range l {
					out = append(out, RevivePackedValue(v))
				}
				return out
			}
			return t["value"]
		case "Map":
			if l, ok := t["value"].([]any); ok {
				out := make([]any, 0, len(l))
				for _, pair := range l {
					if pl, ok := pair.([]any); ok && len(pl) == 2 {
						out = append(out, [2]any{RevivePackedValue(pl[0]), RevivePackedValue(pl[1])})
					} else {
						out = append(out, RevivePackedValue(pair))
					}
				}
				return out
			}
			return t["value"]
		}
		for k, item := range t {
			t[k] = RevivePackedValue(item)
		}
		return t
	default:
		return value
	}
}

func defaultPackCacheDir() string {
	if override := os.Getenv("DNA_BUILDER_CACHE"); override != "" {
		return filepath.Join(override, "data-pack")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".cache", "dna-builder", "data-pack")
}

// ProgressFunc 下载进度回调（received 已收字节，total 总数，未知为 -1）。
type ProgressFunc func(received, total int64)

// DataPackStore 是全量包存储：磁盘 <cache>/<version>/package.zip + 内存缓存。
type DataPackStore struct {
	CacheDir     string
	DataPackBase string
	BackupBase   string
	HTTPTimeout  time.Duration
	client       *http.Client

	manifest      map[string]any
	activeVersion string
	moduleCache   map[string]map[string]any
	manifestMtime float64
}

// NewDataPackStore 构造（空值用默认；backupBase 传 "-" 禁用备用源）。
func NewDataPackStore(cacheDir, dataPackBase, backupBase string) *DataPackStore {
	if cacheDir == "" {
		cacheDir = defaultPackCacheDir()
	}
	if dataPackBase == "" {
		dataPackBase = "https://cdn.dna-builder.cn/data-pack"
	}
	backup := backupBase
	if backup == "" {
		backup = "https://cdn.dobapp.cc/data-pack"
	} else if backup == "-" {
		backup = ""
	}
	return &DataPackStore{
		CacheDir:     cacheDir,
		DataPackBase: dataPackBase,
		BackupBase:   backup,
		HTTPTimeout:  120 * time.Second,
		client:       &http.Client{Timeout: 120 * time.Second},
		moduleCache:  map[string]map[string]any{},
	}
}

// VersionsURL 版本列表地址。
func (s *DataPackStore) VersionsURL() string { return s.DataPackBase + "/" + DefaultVersionsFile }

func (s *DataPackStore) bases() []string {
	var out []string
	if s.DataPackBase != "" {
		out = append(out, s.DataPackBase)
	}
	if s.BackupBase != "" {
		out = append(out, s.BackupBase)
	}
	return out
}

// RemoteVersions 读远端 versions.json（主备依次尝试）。
func (s *DataPackStore) RemoteVersions() ([]map[string]any, error) {
	var last error
	for _, base := range s.bases() {
		raw, err := s.getBytes(base+"/"+DefaultVersionsFile, 30*time.Second)
		if err != nil {
			last = err
			continue
		}
		var payload any
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&payload); err != nil {
			last = err
			continue
		}
		var versions []map[string]any
		switch t := payload.(type) {
		case []any:
			for _, v := range t {
				if m, ok := v.(map[string]any); ok {
					versions = append(versions, m)
				}
			}
		case map[string]any:
			for _, v := range AsList(t["versions"]) {
				if m, ok := v.(map[string]any); ok {
					versions = append(versions, m)
				}
			}
		}
		sort.Slice(versions, func(i, j int) bool { return S(versions[i], "version") > S(versions[j], "version") })
		return versions, nil
	}
	if last == nil {
		last = fmt.Errorf("no base")
	}
	return nil, httpErrorf(0, "http_error", "读取版本列表失败: %s (%v)", s.VersionsURL(), last)
}

func (s *DataPackStore) getBytes(url string, timeout time.Duration) ([]byte, error) {
	client := s.client
	if timeout != s.HTTPTimeout {
		client = &http.Client{Timeout: timeout}
	}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// PackageURL 数据包地址（主源）。
func (s *DataPackStore) PackageURL(version string) string {
	return fmt.Sprintf("%s/%s.zip", s.DataPackBase, version)
}

// Download 下载整包并激活。version 为空取远端最新；返回 manifest。
func (s *DataPackStore) Download(version string, progress ProgressFunc) (map[string]any, error) {
	if version == "" {
		versions, err := s.RemoteVersions()
		if err != nil {
			return nil, err
		}
		if len(versions) == 0 {
			return nil, httpErrorf(0, "http_error", "远端版本列表为空")
		}
		version = S(versions[0], "version")
	}
	destDir := filepath.Join(s.CacheDir, version)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}
	dest := filepath.Join(destDir, "package.zip")
	var last error
	ok := false
	for _, base := range s.bases() {
		if err := s.fetchToFile(fmt.Sprintf("%s/%s.zip", base, version), dest, progress); err != nil {
			last = err
			continue
		}
		ok = true
		break
	}
	if !ok {
		if he, ok := last.(*DobHttpError); ok {
			return nil, he
		}
		if last == nil {
			last = fmt.Errorf("no base")
		}
		return nil, httpErrorf(0, "http_error", "下载数据包失败: %s (%v)", version, last)
	}
	return s.Activate(version)
}

func (s *DataPackStore) fetchToFile(url, dest string, progress ProgressFunc) error {
	resp, err := s.client.Get(url)
	if err != nil {
		return httpErrorf(0, "http_error", "下载数据包失败: %s (%v)", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return httpErrorf(resp.StatusCode, "http_error", "下载数据包失败: %s", url)
	}
	fh, err := os.Create(dest)
	if err != nil {
		return httpErrorf(0, "http_error", "下载数据包失败: %s (%v)", url, err)
	}
	defer fh.Close()
	total := resp.ContentLength
	var received int64
	buf := make([]byte, 256*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := fh.Write(buf[:n]); werr != nil {
				return httpErrorf(0, "http_error", "下载数据包失败: %s (%v)", url, werr)
			}
			received += int64(n)
			if progress != nil {
				progress(received, total)
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return httpErrorf(0, "http_error", "下载数据包失败: %s (%v)", url, err)
		}
	}
	return nil
}

// Activate 切换激活版本：重读磁盘 manifest 并清空模块缓存。
func (s *DataPackStore) Activate(version string) (map[string]any, error) {
	manifest, err := s.readManifestFromZip(version)
	if err != nil {
		return nil, err
	}
	s.manifest = manifest
	s.activeVersion = version
	s.moduleCache = map[string]map[string]any{}
	s.manifestMtime = s.zipMtime(version)
	return manifest, nil
}

// Reload 重读当前激活版本的磁盘包。
func (s *DataPackStore) Reload() (map[string]any, error) {
	if s.activeVersion == "" {
		return nil, nil
	}
	return s.Activate(s.activeVersion)
}

// RefreshIfChanged zip mtime 变化时自动重载。
func (s *DataPackStore) RefreshIfChanged() bool {
	if s.activeVersion == "" {
		return false
	}
	if s.zipMtime(s.activeVersion) != s.manifestMtime {
		if _, err := s.Reload(); err != nil {
			return false
		}
		return true
	}
	return false
}

// ActiveVersion 当前激活版本。
func (s *DataPackStore) ActiveVersion() string { return s.activeVersion }

// Manifest 当前 manifest。
func (s *DataPackStore) Manifest() map[string]any { return s.manifest }

// InstalledVersions 已安装版本（升序）。
func (s *DataPackStore) InstalledVersions() []string {
	entries, err := os.ReadDir(s.CacheDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(s.CacheDir, e.Name(), "package.zip")); err == nil {
				out = append(out, e.Name())
			}
		}
	}
	sort.Strings(out)
	return out
}

// LoadModule 读单个模块（内存 → zip），返回 {exportName: value}。
func (s *DataPackStore) LoadModule(moduleKey string) (map[string]any, error) {
	if m, ok := s.moduleCache[moduleKey]; ok {
		return m, nil
	}
	if s.activeVersion == "" {
		return nil, &DobApiError{Message: "尚未激活数据包版本，先调用 Download()/Activate()", Code: "no_active_pack"}
	}
	raw, err := s.readZipEntry(s.activeVersion, ModulesPrefix+moduleKey+".msgpack")
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return map[string]any{}, nil
	}
	decoded, err := MsgpackDecode(raw)
	if err != nil {
		return nil, err
	}
	record, _ := decoded.(map[string]any)
	if record == nil {
		record = map[string]any{}
	}
	record = RevivePackedValue(record).(map[string]any)
	s.moduleCache[moduleKey] = record
	return record, nil
}

// LoadExport 读模块的单个导出。
func (s *DataPackStore) LoadExport(moduleKey, exportName string) (any, error) {
	m, err := s.LoadModule(moduleKey)
	if err != nil {
		return nil, err
	}
	return m[exportName], nil
}

// Ensure 保证有可用版本：已安装则激活最新，否则下载（缺省远端最新）。返回 manifest。
func (s *DataPackStore) Ensure(version string) (map[string]any, error) {
	if version != "" {
		return s.Activate(version)
	}
	if installed := s.InstalledVersions(); len(installed) > 0 {
		return s.Activate(installed[len(installed)-1])
	}
	return s.Download("", nil)
}

// LoadTables 数据源抽象接口实现：转换为可直接给 Engine 用的 GameDataTables。
func (s *DataPackStore) LoadTables() (*GameDataTables, error) {
	if s.activeVersion == "" {
		if _, err := s.Ensure(""); err != nil {
			return nil, err
		}
	}
	raw := map[string]any{}
	for _, table := range []string{"chars", "mods", "buffs", "effects", "weapons", "pets", "pet_entries", "monsters"} {
		record, err := s.LoadModule(ModuleDefaults[table])
		if err != nil {
			return nil, err
		}
		if exp, ok := PetExports[table]; ok {
			raw[table] = record[exp]
		} else {
			raw[table] = record["default"]
		}
		if raw[table] == nil {
			raw[table] = []any{}
		}
	}
	raw["curves"] = map[string]any{}
	return NewGameDataTables(raw), nil
}

// RAGFingerprints 取 manifest 里打包时算好的 RAG 指纹。
func (s *DataPackStore) RAGFingerprints(lang string) map[string]any {
	rag, _ := s.manifest["rag"].(map[string]any)
	l, _ := rag[lang].(map[string]any)
	kinds, _ := l["kinds"].(map[string]any)
	if kinds == nil {
		return nil
	}
	return CloneMap(kinds)
}

func (s *DataPackStore) zipPath(version string) string {
	return filepath.Join(s.CacheDir, version, "package.zip")
}

func (s *DataPackStore) zipMtime(version string) float64 {
	fi, err := os.Stat(s.zipPath(version))
	if err != nil {
		return 0
	}
	return float64(fi.ModTime().UnixNano()) / 1e9
}

func (s *DataPackStore) readZipEntry(version, name string) ([]byte, error) {
	zr, err := zip.OpenReader(s.zipPath(version))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, nil
}

func (s *DataPackStore) readManifestFromZip(version string) (map[string]any, error) {
	raw, err := s.readZipEntry(version, ManifestFile)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, &DobApiError{Message: "数据包缺 manifest.json: " + version, Code: "bad_pack"}
	}
	var manifest map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&manifest); err != nil {
		return nil, &DobApiError{Message: "数据包缺 manifest.json: " + version, Code: "bad_pack"}
	}
	if _, ok := manifest["_loadedAt"]; !ok {
		manifest["_loadedAt"] = time.Now().UTC().Format(time.RFC3339)
	}
	if _, ok := manifest["_version"]; !ok {
		manifest["_version"] = version
	}
	zr, err := zip.OpenReader(s.zipPath(version))
	if err != nil {
		return nil, &DobApiError{Message: "数据包 zip 损坏: " + version, Code: "bad_pack"}
	}
	defer zr.Close()
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, &DobApiError{Message: "数据包 zip 损坏: " + version, Code: "bad_pack"}
		}
		if _, err := io.Copy(io.Discard, rc); err != nil {
			rc.Close()
			return nil, &DobApiError{Message: "数据包 zip 损坏: " + version, Code: "bad_pack"}
		}
		rc.Close()
	}
	return manifest, nil
}
