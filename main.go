package main

import (
	"archive/zip"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed template.html
var templateFS embed.FS

var tmpl = template.Must(template.ParseFS(templateFS, "template.html"))

type Message struct {
	ID        int64  `json:"id"`
	Content   string `json:"content"`
	IP        string `json:"ip"`
	OS        string `json:"os"`
	CreatedAt string `json:"created_at"`
}

type MessageStore struct {
	mu       sync.RWMutex
	path     string
	messages []Message
}

var msgStore *MessageStore

func newMessageStore(filePath string) *MessageStore {
	ms := &MessageStore{path: filePath}
	os.MkdirAll(filepath.Dir(filePath), 0700)
	ms.load()
	return ms
}

func (ms *MessageStore) load() {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	data, err := os.ReadFile(ms.path)
	if err != nil {
		ms.messages = []Message{}
		return
	}
	json.Unmarshal(data, &ms.messages)
}

func (ms *MessageStore) save() {
	data, _ := json.Marshal(ms.messages)
	os.WriteFile(ms.path, data, 0600)
}

func (ms *MessageStore) add(content, ip, osName string) Message {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	msg := Message{
		ID:        time.Now().UnixMilli(),
		Content:   content,
		IP:        ip,
		OS:        osName,
		CreatedAt: time.Now().Format("01-02 15:04:05"),
	}
	ms.messages = append(ms.messages, msg)
	ms.save()
	return msg
}

func (ms *MessageStore) find(id int64) (Message, bool) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	for _, m := range ms.messages {
		if m.ID == id {
			return m, true
		}
	}
	return Message{}, false
}

func (ms *MessageStore) delete(id int64) bool {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	for i, m := range ms.messages {
		if m.ID == id {
			ms.messages = append(ms.messages[:i], ms.messages[i+1:]...)
			ms.save()
			return true
		}
	}
	return false
}

func (ms *MessageStore) getAll() []Message {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	result := make([]Message, len(ms.messages))
	for i := len(ms.messages) - 1; i >= 0; i-- {
		result[len(ms.messages)-1-i] = ms.messages[i]
	}
	return result
}

type FileInfo struct {
	Name    string
	Size    string
	ModTime string
	IsDir   bool
}

type PageData struct {
	Path  string
	Files []FileInfo
}

func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

func dirHandler(root string) http.HandlerFunc {
	absRoot, _ := filepath.Abs(root)

	return func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, "/")
		rel = filepath.Clean(rel)
		if strings.HasPrefix(rel, "..") {
			http.NotFound(w, r)
			return
		}

		abs := filepath.Join(absRoot, rel)

		info, err := os.Stat(abs)
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		if !info.IsDir() {
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": info.Name()}))
			http.ServeFile(w, r, abs)
			return
		}

		entries, err := os.ReadDir(abs)
		if err != nil {
			http.Error(w, "Read directory error", http.StatusInternalServerError)
			return
		}

		files := make([]FileInfo, 0, len(entries))
		for _, e := range entries {
			if e.Name() == ".messages.json" {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			files = append(files, FileInfo{
				Name:    e.Name(),
				Size:    formatSize(info.Size()),
				ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
				IsDir:   e.IsDir(),
			})
		}

		sort.Slice(files, func(i, j int) bool {
			if files[i].IsDir != files[j].IsDir {
				return files[i].IsDir
			}
			return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
		})

		tmpl.Execute(w, PageData{Path: rel, Files: files})
	}
}

func extractIP(r *http.Request) string {
	ip := r.RemoteAddr
	if strings.HasPrefix(ip, "[") {
		if idx := strings.LastIndex(ip, "]:"); idx != -1 {
			ip = ip[1:idx]
		}
	} else if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

func parseUA(ua string) string {
	switch {
	case strings.Contains(ua, "Windows NT 10"):
		return "Windows 10/11"
	case strings.Contains(ua, "Windows NT 6.3"):
		return "Windows 8.1"
	case strings.Contains(ua, "Windows NT 6.1"):
		return "Windows 7"
	case strings.Contains(ua, "Windows"):
		return "Windows"
	case strings.Contains(ua, "Android"):
		return "Android"
	case strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad"):
		return "iOS"
	case strings.Contains(ua, "Mac OS X") || strings.Contains(ua, "Macintosh"):
		return "macOS"
	case strings.Contains(ua, "Linux"):
		return "Linux"
	default:
		return "Unknown"
	}
}

func messagesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		msgs := msgStore.getAll()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(msgs)
	case http.MethodPost:
		var req struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Content == "" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		ip := extractIP(r)
		osName := parseUA(r.UserAgent())
		msg := msgStore.add(req.Content, ip, osName)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(msg)
	case http.MethodDelete:
		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		msg, found := msgStore.find(id)
		if !found {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if msg.IP != extractIP(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		msgStore.delete(id)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func zipDir(w *zip.Writer, absPath, zipPath string) error {
	info, err := os.Stat(absPath)
	if err != nil {
		return err
	}

	if info.IsDir() {
		entries, err := os.ReadDir(absPath)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.Name() == ".messages.json" {
				continue
			}
			if err := zipDir(w, filepath.Join(absPath, e.Name()), filepath.Join(zipPath, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}

	file, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer file.Close()

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(zipPath)
	header.Method = zip.Deflate
	writer, err := w.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, file)
	return err
}

// zipHandler 将目录实时打包为 zip 并流式下载，路径经 /zip/ 前缀剥离
func zipHandler(root string) http.HandlerFunc {
	absRoot, _ := filepath.Abs(root)

	return func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, "/zip/")
		rel = filepath.Clean(rel)
		if strings.HasPrefix(rel, "..") {
			http.NotFound(w, r)
			return
		}

		abs := filepath.Join(absRoot, rel)
		info, err := os.Stat(abs)
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		name := filepath.Base(abs)
		if !info.IsDir() {
			name = strings.TrimSuffix(name, filepath.Ext(name))
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name + ".zip"}))

		zw := zip.NewWriter(w)
		defer zw.Close()
		if err := zipDir(zw, abs, name); err != nil {
			// 头部可能已写入，无法再报错，仅终止
			return
		}
	}
}

// messageStorePath 返回留言板存储路径，存于用户配置目录，独立于被服务的根目录，
// 避免泄露给 /dl/ 等文件服务。
func messageStorePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		// 兜底：退回到可执行文件所在目录，保证不落在被服务目录树内。
		exe, exeErr := os.Executable()
		if exeErr != nil {
			return filepath.Join(os.TempDir(), "fileserver", "messages.json")
		}
		dir = filepath.Dir(exe)
	}
	return filepath.Join(dir, "fileserver", "messages.json")
}

// migrateLegacyStore 将旧版存放在服务根目录里的 .messages.json 迁移到新位置一次。
func migrateLegacyStore(absRoot, newPath string) {
	legacy := filepath.Join(absRoot, ".messages.json")
	if _, err := os.Stat(newPath); err == nil {
		return // 新存储已存在，无需迁移
	}
	data, err := os.ReadFile(legacy)
	if err != nil {
		return // 无旧文件，直接使用新存储
	}
	os.MkdirAll(filepath.Dir(newPath), 0700)
	if err := os.WriteFile(newPath, data, 0600); err != nil {
		return
	}
	os.Remove(legacy)
}

// blockHiddenFile 加固文件服务，拒绝访问隐藏的消息存储文件（defense-in-depth）。
func blockHiddenFile(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/.messages.json") || r.URL.Path == "/.messages.json" {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func whoamiHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"ip": extractIP(r),
		"os": parseUA(r.UserAgent()),
	})
}

func main() {
	port := "8080"
	root := "."

	usage := func() {
		fmt.Printf(`
  📁 File Server - 内网文件浏览下载服务

  Usage:
    fileserver [options] [port]

  Options:
    -p, --port <port>   监听端口 (默认 8080)
    -d, --dir  <dir>    要服务的目录 (默认当前目录 .)
    -h, --help          显示本帮助

  Examples:
    fileserver                         服务当前目录, 端口 8080
    fileserver -p 9000                 指定端口
    fileserver -d /path/to/dir         指定目录
    fileserver -p 9000 -d /data        同时指定
    fileserver 9000                    省略 -p 直接写端口

`)
	}

	for i := 1; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "-h", "--help":
			usage()
			return
		case "-p", "--port":
			if i+1 < len(os.Args) {
				port = os.Args[i+1]
				i++
			}
		case "-d", "--dir":
			if i+1 < len(os.Args) {
				root = os.Args[i+1]
				i++
			}
		default:
			if strings.HasPrefix(os.Args[i], "-") {
				fmt.Fprintf(os.Stderr, "Unknown option: %s\n", os.Args[i])
				usage()
				os.Exit(1)
			}
			port = os.Args[i]
		}
	}

	absRoot, _ := filepath.Abs(root)

	storePath := messageStorePath()
	migrateLegacyStore(absRoot, storePath)

	mux := http.NewServeMux()

	mux.Handle("/dl/", blockHiddenFile(http.StripPrefix("/dl/", http.FileServer(http.Dir(absRoot)))))
	mux.HandleFunc("/zip/", zipHandler(root))
	msgStore = newMessageStore(storePath)
	mux.HandleFunc("/api/messages", messagesHandler)
	mux.HandleFunc("/api/whoami", whoamiHandler)
	mux.HandleFunc("/", dirHandler(root))

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to listen on port %s: %v\n", port, err)
		os.Exit(1)
	}

	addrs := getLocalIPs()
	fmt.Printf("\n")
	fmt.Printf("  📁 File Server\n")
	fmt.Printf("  Serving: %s\n", absRoot)
	fmt.Printf("  Port:    %s\n\n", port)
	for _, addr := range addrs {
		fmt.Printf("  ➜ http://%s:%s\n", addr, port)
	}
	fmt.Printf("\n  Press Ctrl+C to stop.\n\n")

	if err := http.Serve(listener, mux); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

func getLocalIPs() []string {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return []string{"localhost"}
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			ips = append(ips, ipnet.IP.String())
		}
	}
	if len(ips) == 0 {
		ips = append(ips, "localhost")
	}
	return ips
}
