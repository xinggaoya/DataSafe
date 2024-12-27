package handlers

import (
	"fmt"
	"mysql-backup/models"
	"mysql-backup/services"
	"mysql-backup/storage"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron"
)

type BackupHandler struct {
	backup   *services.BackupService
	schedule *services.ScheduleService
	store    storage.Store
}

// BackupRequest 修改备份请求结构
type BackupRequest struct {
	SettingID int    `json:"settingId"`
	Database  string `json:"database"`
	Schedule  string `json:"schedule,omitempty"`
}

// BackupResponse 备份记录响应结构
type BackupResponse struct {
	ID          int    `json:"id"`
	SettingName string `json:"settingName"`
	DBName      string `json:"dbName"`
	FileName    string `json:"fileName"`
	CreatedAt   string `json:"createdAt"`
	Status      string `json:"status"`
}

// ScheduleResponse 定时任务响应结构
type ScheduleResponse struct {
	ID          int    `json:"id"`
	SettingID   int    `json:"settingId"`
	SettingName string `json:"settingName"`
	Database    string `json:"database"`
	Schedule    string `json:"schedule"`
}

func NewBackupHandler(backup *services.BackupService, schedule *services.ScheduleService, store storage.Store) *BackupHandler {
	return &BackupHandler{
		backup:   backup,
		schedule: schedule,
		store:    store,
	}
}

// GetDatabases 获取指定配置的数据库列表
func (h *BackupHandler) GetDatabases(c *gin.Context) {
	settingIDStr := c.Query("settingId")
	settingID, err := strconv.Atoi(settingIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid setting id"})
		return
	}

	// 获取数据库配置
	setting, err := h.store.GetSettingByID(settingID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get setting"})
		return
	}

	// 使用配置获取数据库列表
	databases, err := h.backup.GetDatabasesWithConfig(setting)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, databases)
}

// CreateBackup 创建备份
func (h *BackupHandler) CreateBackup(c *gin.Context) {
	var req BackupRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取数据库配置
	setting, err := h.store.GetSettingByID(req.SettingID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
		return
	}

	// 使用配置创建备份
	err = h.backup.BackupDatabaseWithConfig(setting, req.Database, h.store)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "备份任务已创建"})
}

// GetBackups 获取备份历史
func (h *BackupHandler) GetBackups(c *gin.Context) {
	var page models.PageRequest
	if err := c.ShouldBindQuery(&page); err != nil {
		// 如果没有传分页参数，使用默认值
		page = models.PageRequest{Page: 1, PageSize: 10}
	}

	// 获取总记录数和分页数据
	total, records, err := h.store.GetBackupRecordsWithPage(page.Page, page.PageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 转换为响应格式
	var responses []BackupResponse
	for _, record := range records {
		setting, _ := h.store.GetSettingByID(record.SettingID)
		settingName := "未知配置"
		if setting != nil {
			settingName = setting.Name
		}

		responses = append(responses, BackupResponse{
			ID:          record.ID,
			SettingName: settingName,
			DBName:      record.DBName,
			FileName:    record.FileName,
			CreatedAt:   record.CreatedAt,
			Status:      record.Status,
		})
	}

	c.JSON(http.StatusOK, models.PageResponse{
		Total: total,
		Data:  responses,
	})
}

// ScheduleBackup 创建定时备份任务
func (h *BackupHandler) ScheduleBackup(c *gin.Context) {
	var req BackupRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	// 验证必要参数
	if req.SettingID == 0 || req.Database == "" || req.Schedule == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少必要参数"})
		return
	}

	// 验证 Cron 表达式
	if _, err := cron.ParseStandard(req.Schedule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 Cron 表达式"})
		return
	}

	// 获取数据库配置
	setting, err := h.store.GetSettingByID(req.SettingID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取数据库配置失败"})
		return
	}

	// 检查是否已存在相同的任务
	tasks := h.schedule.ListTasks()
	for _, task := range tasks {
		if task.SettingID == req.SettingID &&
			task.Database == req.Database &&
			task.Schedule == req.Schedule {
			// 如果已存在完全相同的任务，直接返回成功
			c.JSON(http.StatusOK, gin.H{
				"id":      task.ID,
				"message": "任务已存在",
			})
			return
		}
	}

	// 添加定时任务
	id, err := h.schedule.AddTaskWithConfig(setting, req.Database, req.Schedule)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":      id,
		"message": "定时任务添加成功",
	})
}

// ListSchedules 获取定时任务列表
func (h *BackupHandler) ListSchedules(c *gin.Context) {
	var page models.PageRequest
	if err := c.ShouldBindQuery(&page); err != nil {
		// 如果没有传分页参数，使用默认值
		page = models.PageRequest{Page: 1, PageSize: 10}
	}

	// 获取总记录数和分页数据
	total, tasks := h.schedule.ListTasksWithPage(page.Page, page.PageSize)
	var responses []ScheduleResponse

	for _, task := range tasks {
		setting, _ := h.store.GetSettingByID(task.SettingID)
		settingName := "未知配置"
		if setting != nil {
			settingName = setting.Name
		}

		responses = append(responses, ScheduleResponse{
			ID:          task.ID,
			SettingID:   task.SettingID,
			SettingName: settingName,
			Database:    task.Database,
			Schedule:    task.Schedule,
		})
	}

	c.JSON(http.StatusOK, models.PageResponse{
		Total: total,
		Data:  responses,
	})
}

func (h *BackupHandler) DeleteSchedule(c *gin.Context) {
	id := c.Param("id")
	taskID := 0
	fmt.Sscanf(id, "%d", &taskID)

	if err := h.schedule.RemoveTask(taskID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Schedule deleted successfully"})
}

func (h *BackupHandler) SaveSettings(c *gin.Context) {
	var settings models.DBSettings
	if err := c.BindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.store.SaveSettings(&settings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, settings)
}

func (h *BackupHandler) GetSettings(c *gin.Context) {
	settings, err := h.store.GetAllSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, settings)
}

// TestConnection 测试数据库连接
func (h *BackupHandler) TestConnection(c *gin.Context) {
	var settings models.DBSettings
	if err := c.BindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.backup.TestConnection(&settings)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("连接失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "连接成功",
	})
}

// ListBackupFiles 获取备份文件列表
func (h *BackupHandler) ListBackupFiles(c *gin.Context) {
	setting, err := h.store.GetSettingByID(1) // TODO: 从请求参数获取配置ID
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
		return
	}

	files, err := h.backup.ListBackupFiles(setting.BackupDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, files)
}

// DeleteBackupFile 删除备份文件
func (h *BackupHandler) DeleteBackupFile(c *gin.Context) {
	filename := c.Param("filename")
	setting, err := h.store.GetSettingByID(1) // TODO: 从请求参数获取配置ID
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
		return
	}

	if err := h.backup.DeleteBackupFile(setting.BackupDir, filename); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "文件删除成功"})
}

// DownloadBackupFile 下载备份文件
func (h *BackupHandler) DownloadBackupFile(c *gin.Context) {
	filename := c.Param("filename")
	setting, err := h.store.GetSettingByID(1) // TODO: 从请求参数获取配置ID
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取配置失败"})
		return
	}

	filePath := filepath.Join(setting.BackupDir, filename)
	// 验证文件路径
	if !strings.HasPrefix(filePath, setting.BackupDir) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的文件路径"})
		return
	}

	c.File(filePath)
}
