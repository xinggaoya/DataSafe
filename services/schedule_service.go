package services

import (
	"fmt"
	"log"
	"mysql-backup/models"
	"mysql-backup/storage"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

type ScheduledTask struct {
	ID        int    `json:"id"`
	SettingID int    `json:"settingId"`
	Database  string `json:"database"`
	Schedule  string `json:"schedule"`
	EntryID   cron.EntryID
}

type ScheduleService struct {
	cron      *cron.Cron
	tasks     map[int]*ScheduledTask
	backup    *BackupService
	store     storage.Store
	lastID    int
	taskMutex sync.RWMutex
}

func NewScheduleService(c *cron.Cron, backup *BackupService, store storage.Store) *ScheduleService {
	s := &ScheduleService{
		cron:   c,
		tasks:  make(map[int]*ScheduledTask),
		backup: backup,
		store:  store,
	}

	// 从存储中恢复定时任务
	if err := s.restoreSchedules(); err != nil {
		log.Printf("恢复定时任务失败: %v", err)
	}

	// 确保 cron 已经启动
	s.cron.Start()

	return s
}

// 添加恢复定时任务的方法
func (s *ScheduleService) restoreSchedules() error {
	tasks, err := s.store.GetAllSchedules()
	if err != nil {
		return fmt.Errorf("获取定时任务失败: %v", err)
	}

	for _, task := range tasks {
		setting, err := s.store.GetSettingByID(task.SettingID)
		if err != nil {
			log.Printf("获取任务 %d 的数据库配置失败: %v", task.ID, err)
			continue
		}

		// 恢复时也需要添加秒字段
		cronExpr := "0 " + task.Schedule
		entryID, err := s.cron.AddFunc(cronExpr, func() {
			if err := s.backup.BackupDatabaseWithConfig(setting, task.Database, s.store); err != nil {
				log.Printf("定时备份失败 [%s]: %v\n", task.Database, err)
				return
			}

			record := &models.BackupRecord{
				SettingID: setting.ID,
				DBName:    task.Database,
				FileName:  fmt.Sprintf("%s_%s.sql", task.Database, time.Now().Format("20060102150405")),
				CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
				Status:    "completed",
			}
			if err := s.store.SaveBackupRecord(record); err != nil {
				log.Printf("保存备份记录失败: %v", err)
			}
			log.Printf("定时备份完成: %s", task.Database)
		})

		if err != nil {
			log.Printf("恢复任务 %d 失败: %v", task.ID, err)
			continue
		}

		s.tasks[task.ID] = &ScheduledTask{
			ID:        task.ID,
			SettingID: task.SettingID,
			Database:  task.Database,
			Schedule:  task.Schedule,
			EntryID:   entryID,
		}
		log.Printf("成功恢复定时任务: [ID=%d] %s (%s)", task.ID, task.Database, task.Schedule)
	}

	return nil
}

func (s *ScheduleService) AddTaskWithConfig(setting *models.DBSettings, database, schedule string) (int, error) {
	s.taskMutex.Lock()
	defer s.taskMutex.Unlock()

	// 验证 Cron 表达式 (使用标准的5字段格式)
	if _, err := cron.ParseStandard(schedule); err != nil {
		return 0, fmt.Errorf("无效的 Cron 表达式: %v", err)
	}

	// 创建任务记录 (存储时使用5字段格式)
	task := &models.ScheduledTask{
		SettingID: setting.ID,
		Database:  database,
		Schedule:  schedule,
	}

	// 保存到存储
	if err := s.store.SaveSchedule(task); err != nil {
		return 0, fmt.Errorf("保存定时任务失败: %v", err)
	}

	// 添加到 cron (添加秒字段)
	cronExpr := "0 " + schedule // 添加秒字段
	entryID, err := s.cron.AddFunc(cronExpr, func() {
		log.Printf("开始执行定时备份任务: %s", database)
		if err := s.backup.BackupDatabaseWithConfig(setting, database, s.store); err != nil {
			log.Printf("定时备份失败 [%s]: %v\n", database, err)
			return
		}

		// 记录备份历史
		record := &models.BackupRecord{
			SettingID: setting.ID,
			DBName:    database,
			FileName:  fmt.Sprintf("%s_%s.sql", database, time.Now().Format("20060102150405")),
			CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
			Status:    "completed",
		}
		if err := s.store.SaveBackupRecord(record); err != nil {
			log.Printf("保存备份记录失败: %v", err)
		}
		log.Printf("定时备份完成: %s", database)
	})

	if err != nil {
		// 如果添加到 cron 失败，删除存储的任务
		s.store.DeleteSchedule(task.ID)
		return 0, fmt.Errorf("添加定时任务失败: %v", err)
	}

	// 保存到内存
	s.tasks[task.ID] = &ScheduledTask{
		ID:        task.ID,
		SettingID: setting.ID,
		Database:  database,
		Schedule:  schedule,
		EntryID:   entryID,
	}

	log.Printf("成功添加定时任务: [ID=%d] %s (%s)", task.ID, database, schedule)
	return task.ID, nil
}

func (s *ScheduleService) RemoveTask(id int) error {
	s.taskMutex.Lock()
	defer s.taskMutex.Unlock()

	task, exists := s.tasks[id]
	if !exists {
		return fmt.Errorf("task not found: %d", id)
	}

	s.cron.Remove(task.EntryID)
	delete(s.tasks, id)

	// 从存储中删除
	return s.store.DeleteSchedule(id)
}

func (s *ScheduleService) ListTasks() []ScheduledTask {
	s.taskMutex.RLock()
	defer s.taskMutex.RUnlock()

	tasks := make([]ScheduledTask, 0, len(s.tasks))
	for _, task := range s.tasks {
		tasks = append(tasks, *task)
	}
	return tasks
}

// ListTasksWithPage 分页获取定时任务列表
func (s *ScheduleService) ListTasksWithPage(page, pageSize int) (int, []ScheduledTask) {
	s.taskMutex.RLock()
	defer s.taskMutex.RUnlock()

	tasks := make([]ScheduledTask, 0, len(s.tasks))
	for _, task := range s.tasks {
		tasks = append(tasks, *task)
	}

	// 计算总记录数
	total := len(tasks)

	// 计算分页范围
	start := (page - 1) * pageSize
	if start >= total {
		return total, []ScheduledTask{}
	}

	end := start + pageSize
	if end > total {
		end = total
	}

	return total, tasks[start:end]
}
