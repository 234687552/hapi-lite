package api

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type FileHandler struct {
	*BaseHandler
}

type fileSearchItem struct {
	FileName string `json:"fileName"`
	FilePath string `json:"filePath"`
	FullPath string `json:"fullPath"`
	FileType string `json:"fileType"`
}

const maxUploadBytes = 50 * 1024 * 1024

func parseLimit(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	if n > 1000 {
		return 1000
	}
	return n
}

func getSessionRoot(h *FileHandler, sessionID string) (string, error) {
	sess, err := h.Store.GetSession(sessionID)
	if err != nil {
		return "", err
	}
	if sess == nil || sess.Metadata == nil || strings.TrimSpace(sess.Metadata.Path) == "" {
		return "", os.ErrNotExist
	}
	return sess.Metadata.Path, nil
}

func safeJoin(root string, relPath string) (string, error) {
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target := filepath.Join(cleanRoot, relPath)
	cleanTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	prefix := cleanRoot + string(os.PathSeparator)
	if cleanTarget != cleanRoot && !strings.HasPrefix(cleanTarget, prefix) {
		return "", os.ErrPermission
	}
	return cleanTarget, nil
}

func toSearchItem(root string, fullPath string, fileType string) fileSearchItem {
	rel, _ := filepath.Rel(root, fullPath)
	rel = filepath.ToSlash(rel)
	rel = strings.TrimPrefix(rel, "./")
	name := filepath.Base(fullPath)
	dir := filepath.ToSlash(filepath.Dir(rel))
	if dir == "." {
		dir = ""
	}
	return fileSearchItem{
		FileName: name,
		FilePath: dir,
		FullPath: rel,
		FileType: fileType,
	}
}

func shouldSkipDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor"
}

func (h *FileHandler) ListFiles(c *gin.Context) {
	sessionID := c.Param("id")
	query := strings.TrimSpace(c.Query("query"))
	limit := parseLimit(c.Query("limit"), 200)

	root, err := getSessionRoot(h, sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	items := make([]fileSearchItem, 0, limit)
	lowerQuery := strings.ToLower(query)
	if lowerQuery != "" {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			name := info.Name()
			if info.IsDir() && shouldSkipDir(name) {
				return filepath.SkipDir
			}
			if info.IsDir() {
				return nil
			}
			if strings.Contains(strings.ToLower(name), lowerQuery) {
				items = append(items, toSearchItem(root, path, "file"))
			}
			if len(items) >= limit {
				return filepath.SkipAll
			}
			return nil
		})
	} else {
		dirEntries, _ := os.ReadDir(root)
		for _, entry := range dirEntries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			full := filepath.Join(root, entry.Name())
			fileType := "file"
			if entry.IsDir() {
				fileType = "folder"
			}
			items = append(items, toSearchItem(root, full, fileType))
			if len(items) >= limit {
				break
			}
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].FullPath < items[j].FullPath
	})

	c.JSON(http.StatusOK, gin.H{"success": true, "files": items})
}

func (h *FileHandler) GetFile(c *gin.Context) {
	sessionID := c.Param("id")
	relPath := c.Query("path")
	if relPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}

	root, err := getSessionRoot(h, sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	fullPath, err := safeJoin(root, relPath)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": "File not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"content": base64.StdEncoding.EncodeToString(data),
	})
}

func (h *FileHandler) ListDirectory(c *gin.Context) {
	sessionID := c.Param("id")
	relPath := strings.TrimSpace(c.Query("path"))

	root, err := getSessionRoot(h, sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Session not found"})
		return
	}

	target := root
	if relPath != "" {
		target, err = safeJoin(root, relPath)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Access denied"})
			return
		}
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}

	type directoryEntry struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		Size     int64  `json:"size,omitempty"`
		Modified int64  `json:"modified,omitempty"`
	}

	items := make([]directoryEntry, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		itemType := "file"
		if entry.IsDir() {
			itemType = "directory"
		} else if !info.Mode().IsRegular() {
			itemType = "other"
		}
		item := directoryEntry{
			Name:     entry.Name(),
			Type:     itemType,
			Modified: info.ModTime().UnixMilli(),
		}
		if info.Mode().IsRegular() {
			item.Size = info.Size()
		}
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Type != items[j].Type {
			return items[i].Type == "directory"
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	c.JSON(http.StatusOK, gin.H{"success": true, "entries": items})
}

func (h *FileHandler) UploadFile(c *gin.Context) {
	sessionID := c.Param("id")
	root, err := getSessionRoot(h, sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Session not found"})
		return
	}

	var body struct {
		Filename string `json:"filename" binding:"required"`
		Content  string `json:"content" binding:"required"`
		MimeType string `json:"mimeType"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid body"})
		return
	}

	data, err := base64.StdEncoding.DecodeString(body.Content)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid base64 content"})
		return
	}
	if len(data) > maxUploadBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"success": false, "error": "File too large (max 50MB)"})
		return
	}

	uploadDir := filepath.Join(root, ".hapi-lite", "uploads")
	if err := os.MkdirAll(uploadDir, 0o700); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	filename := filepath.Base(strings.TrimSpace(body.Filename))
	if filename == "." || filename == "" || filename == string(filepath.Separator) {
		filename = "upload.bin"
	}
	target := filepath.Join(uploadDir, fmt.Sprintf("%d-%s", time.Now().UnixNano(), filename))
	if err := os.WriteFile(target, data, 0o600); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "path": target})
}

func (h *FileHandler) DeleteUploadFile(c *gin.Context) {
	sessionID := c.Param("id")
	root, err := getSessionRoot(h, sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Session not found"})
		return
	}

	var body struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Path) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid body"})
		return
	}

	allowedDir := filepath.Join(root, ".hapi-lite", "uploads")
	cleanAllowedDir, _ := filepath.Abs(allowedDir)
	target, err := filepath.Abs(body.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid path"})
		return
	}
	prefix := cleanAllowedDir + string(os.PathSeparator)
	if target != cleanAllowedDir && !strings.HasPrefix(target, prefix) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Access denied"})
		return
	}

	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
