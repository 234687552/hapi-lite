package api

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/liangzd/hapi-lite/internal/auth"
	"github.com/liangzd/hapi-lite/internal/session"
	"github.com/liangzd/hapi-lite/internal/sse"
	"github.com/liangzd/hapi-lite/internal/store"
)

func SetupRouter(st *store.Store, broker *sse.Broker, mgr *session.Manager) *gin.Engine {
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		AllowCredentials: false,
	}))

	api := r.Group("/api")
	api.POST("/auth", AuthHandler)
	api.POST("/bind", BindHandler)

	protected := api.Group("")
	protected.Use(auth.Middleware())

	base := &BaseHandler{Store: st, Broker: broker, Mgr: mgr}

	sh := &SessionHandler{BaseHandler: base}
	protected.GET("/sessions", sh.List)
	protected.POST("/sessions", sh.Create)
	protected.GET("/sessions/:id", sh.Get)
	protected.DELETE("/sessions/:id", sh.Delete)
	protected.POST("/sessions/:id/resume", sh.Resume)
	protected.POST("/sessions/:id/abort", sh.Abort)
	protected.POST("/sessions/:id/archive", sh.Archive)
	protected.POST("/sessions/:id/switch", sh.Switch)
	protected.POST("/sessions/:id/permission-mode", sh.SetPermissionMode)
	protected.POST("/sessions/:id/model", sh.SetModel)
	protected.PATCH("/sessions/:id", sh.Rename)
	protected.GET("/sessions/:id/slash-commands", sh.ListSlashCommands)
	protected.GET("/sessions/:id/skills", sh.ListSkills)

	mh := &MessageHandler{BaseHandler: base}
	protected.GET("/sessions/:id/messages", mh.List)
	protected.POST("/sessions/:id/messages", mh.Send)

	ph := &PermissionHandler{BaseHandler: base}
	protected.POST("/sessions/:id/permissions/:requestId/approve", ph.Approve)
	protected.POST("/sessions/:id/permissions/:requestId/deny", ph.Deny)

	fh := &FileHandler{BaseHandler: base}
	protected.GET("/sessions/:id/files", fh.ListFiles)
	protected.GET("/sessions/:id/file", fh.GetFile)
	protected.GET("/sessions/:id/directory", fh.ListDirectory)
	protected.POST("/sessions/:id/upload", fh.UploadFile)
	protected.POST("/sessions/:id/upload/delete", fh.DeleteUploadFile)

	gh := &GitHandler{BaseHandler: base}
	protected.GET("/sessions/:id/git-status", gh.GitStatus)
	protected.GET("/sessions/:id/git-diff-numstat", gh.GitDiffNumstat)
	protected.GET("/sessions/:id/git-diff-file", gh.GitDiffFile)

	machineH := &MachineHandler{BaseHandler: base}
	protected.GET("/machines", machineH.List)
	protected.POST("/machines/:id/spawn", machineH.Spawn)
	protected.POST("/machines/:id/paths/exists", machineH.PathsExists)

	miscH := &MiscHandler{}
	protected.POST("/visibility", miscH.Visibility)
	protected.GET("/push/vapid-public-key", miscH.PushVapidPublicKey)
	protected.POST("/push/subscribe", miscH.PushSubscribe)
	protected.DELETE("/push/subscribe", miscH.PushUnsubscribe)

	sseH := &SSEHandler{Broker: broker}
	protected.GET("/events", sseH.Events)

	// Serve frontend static files with SPA fallback
	distDir := "web/dist"
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/socket.io") {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}
		fp := filepath.Join(distDir, p)
		if info, err := os.Stat(fp); err == nil && !info.IsDir() {
			c.File(fp)
			return
		}
		c.File(filepath.Join(distDir, "index.html"))
	})

	return r
}
