package main

import (
	"embed"
	"html/template"
	"log"
	"mysql-backup/handlers"
	"mysql-backup/services"
	"mysql-backup/storage"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
)

//go:embed static/*.html
var staticFiles embed.FS

func main() {
	r := gin.Default()
	c := cron.New(cron.WithSeconds())

	// 初始化 BoltDB 存储
	store, err := storage.NewBoltStore("./data.db")
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer store.Close()

	// 初始化服务和处理器
	backupService := services.NewBackupService()
	scheduleService := services.NewScheduleService(c, backupService, store)
	backupHandler := handlers.NewBackupHandler(backupService, scheduleService, store)

	// 设置 HTML 模板，修改分隔符以避免与 Vue 冲突
	t := template.New("").Delims("[[", "]]")
	t, err = t.ParseFS(staticFiles, "static/*.html")
	if err != nil {
		log.Fatal("Failed to parse templates:", err)
	}
	r.SetHTMLTemplate(t)

	// 首页路由
	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	// 设置页面路由
	r.GET("/backup", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index1.html", nil)
	})

	// API路由
	api := r.Group("/api")
	{
		api.GET("/databases", backupHandler.GetDatabases)
		api.GET("/backups", backupHandler.GetBackups)
		api.POST("/backup", backupHandler.CreateBackup)
		api.GET("/schedules", backupHandler.ListSchedules)
		api.POST("/schedules", backupHandler.ScheduleBackup)
		api.DELETE("/schedules/:id", backupHandler.DeleteSchedule)
		api.GET("/settings", backupHandler.GetSettings)
		api.POST("/settings", backupHandler.SaveSettings)
		api.POST("/test-connection", backupHandler.TestConnection)
		api.GET("/backup-files", backupHandler.ListBackupFiles)
		api.DELETE("/backup-files/:filename", backupHandler.DeleteBackupFile)
		api.GET("/backup-files/:filename", backupHandler.DownloadBackupFile)
	}

	// 启动定时任务
	c.Start()

	// 启动服务器
	log.Fatal(r.Run(":6680"))
}
