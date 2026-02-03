package httpapi

import (
	"embed"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// FrontendHandler handles serving frontend files with fallback support
type FrontendHandler struct {
	useEmbedded bool
	fileSystem  http.FileSystem
}

// NewFrontendHandler creates a new frontend handler
// It tries to use the filesystem first, falls back to embedded files if not available
func NewFrontendHandler(frontendDir string, embeddedFS embed.FS) *FrontendHandler {
	// Check if frontend directory exists
	if _, err := os.Stat(frontendDir); err == nil {
		log.Printf("[frontend] Using filesystem: %s", frontendDir)
		return &FrontendHandler{
			useEmbedded: false,
			fileSystem:  http.Dir(frontendDir),
		}
	}

	// Fallback to embedded files
	log.Printf("[frontend] Filesystem not found, using embedded files")
	fsys, err := fs.Sub(embeddedFS, "frontend")
	if err != nil {
		log.Printf("[frontend] Warning: failed to load embedded files: %v", err)
		// Still try to use the directory even if it doesn't exist
		return &FrontendHandler{
			useEmbedded: false,
			fileSystem:  http.Dir(frontendDir),
		}
	}

	return &FrontendHandler{
		useEmbedded: true,
		fileSystem:  http.FS(fsys),
	}
}

// ServeHTTP serves frontend files
func (h *FrontendHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only allow GET and HEAD
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Security: Prevent path traversal
	path := r.URL.Path
	if strings.Contains(path, "..") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	// Normalize path separators to forward slashes
	path = filepath.ToSlash(path)

	// Handle root path
	if path == "/" || path == "." {
		path = "index.html"
	} else {
		// Remove leading slash for file system access
		path = strings.TrimPrefix(path, "/")
	}

	log.Printf("[frontend] Serving: %s (original: %s)", path, r.URL.Path)

	// Try to open the file
	file, err := h.fileSystem.Open(path)
	if err != nil {
		log.Printf("[frontend] Error opening %s: %v", path, err)
		// If file not found and it's not index.html, try index.html (SPA fallback)
		if os.IsNotExist(err) && path != "index.html" {
			// Check if it's a file with extension
			if filepath.Ext(path) == "" {
				// No extension, might be a route, serve index.html
				file, err = h.fileSystem.Open("index.html")
				if err != nil {
					http.Error(w, "Not Found", http.StatusNotFound)
					return
				}
				defer file.Close()
				h.serveFile(w, r, "index.html", file)
				return
			}
		}
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	defer file.Close()

	// Serve the file
	h.serveFile(w, r, path, file)
}

// serveFile serves a single file with proper content type
func (h *FrontendHandler) serveFile(w http.ResponseWriter, r *http.Request, name string, file http.File) {
	// Get file info
	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// If it's a directory, try to serve index.html
	if stat.IsDir() {
		indexPath := filepath.ToSlash(filepath.Join(name, "index.html"))
		indexFile, err := h.fileSystem.Open(indexPath)
		if err != nil {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		defer indexFile.Close()
		indexStat, err := indexFile.Stat()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Set content type for HTML
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		// Serve the file content
		if seeker, ok := indexFile.(io.ReadSeeker); ok {
			http.ServeContent(w, r, "index.html", indexStat.ModTime(), seeker)
		} else {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	// Set content type based on file extension
	contentType := getContentType(name)
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	// Serve the file content
	if seeker, ok := file.(io.ReadSeeker); ok {
		http.ServeContent(w, r, name, stat.ModTime(), seeker)
	} else {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// getContentType returns the content type based on file extension
func getContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".eot":
		return "application/vnd.ms-fontobject"
	default:
		return ""
	}
}
