package api

import (
	"bytes"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

type GitHandler struct {
	*BaseHandler
}

type gitCommandResult struct {
	Success  bool   `json:"success"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exitCode,omitempty"`
	Error    string `json:"error,omitempty"`
}

func runGitCommand(root string, args ...string) gitCommandResult {
	cmd := exec.Command("git", args...)
	cmd.Dir = root

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return gitCommandResult{
			Success: true,
			Stdout:  stdout.String(),
			Stderr:  stderr.String(),
		}
	}

	exitCode := 1
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	}
	return gitCommandResult{
		Success:  false,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Error:    err.Error(),
	}
}

func (h *GitHandler) GitStatus(c *gin.Context) {
	root, err := getSessionRoot(&FileHandler{BaseHandler: h.BaseHandler}, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gitCommandResult{Success: false, Error: "Session not found"})
		return
	}
	c.JSON(http.StatusOK, runGitCommand(root, "status", "--porcelain=v2", "--branch"))
}

func (h *GitHandler) GitDiffNumstat(c *gin.Context) {
	root, err := getSessionRoot(&FileHandler{BaseHandler: h.BaseHandler}, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gitCommandResult{Success: false, Error: "Session not found"})
		return
	}

	staged := strings.EqualFold(c.Query("staged"), "true") || c.Query("staged") == "1"
	args := []string{"diff", "--numstat"}
	if staged {
		args = []string{"diff", "--cached", "--numstat"}
	}
	c.JSON(http.StatusOK, runGitCommand(root, args...))
}

func (h *GitHandler) GitDiffFile(c *gin.Context) {
	root, err := getSessionRoot(&FileHandler{BaseHandler: h.BaseHandler}, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gitCommandResult{Success: false, Error: "Session not found"})
		return
	}

	reqPath := strings.TrimSpace(c.Query("path"))
	if reqPath == "" {
		c.JSON(http.StatusBadRequest, gitCommandResult{Success: false, Error: "path is required"})
		return
	}
	fullPath, err := safeJoin(root, reqPath)
	if err != nil {
		c.JSON(http.StatusForbidden, gitCommandResult{Success: false, Error: "Access denied"})
		return
	}
	relPath, err := filepath.Rel(root, fullPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gitCommandResult{Success: false, Error: "Invalid path"})
		return
	}
	relPath = filepath.ToSlash(relPath)

	staged := strings.EqualFold(c.Query("staged"), "true") || c.Query("staged") == "1"
	args := []string{"diff"}
	if staged {
		args = []string{"diff", "--cached"}
	}
	args = append(args, "--", relPath)
	c.JSON(http.StatusOK, runGitCommand(root, args...))
}
