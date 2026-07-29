package main

import (
	"embed"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed template.html
var templateFS embed.FS

var tmpl = template.Must(template.ParseFS(templateFS, "template.html"))

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
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, info.Name()))
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

func main() {
	port := "8080"
	root := "."

	for i := 1; i < len(os.Args); i++ {
		switch os.Args[i] {
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
			port = os.Args[i]
		}
	}

	absRoot, _ := filepath.Abs(root)

	mux := http.NewServeMux()

	mux.Handle("/dl/", http.StripPrefix("/dl/", http.FileServer(http.Dir(absRoot))))
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
