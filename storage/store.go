package storage

import "mysql-backup/models"

// Store 定义了存储接口
type Store interface {
	// 设置相关
	SaveSettings(settings *models.DBSettings) error
	GetAllSettings() ([]models.DBSettings, error)
	GetSettingByID(id int) (*models.DBSettings, error)

	// 备份记录相关
	SaveBackupRecord(record *models.BackupRecord) error
	UpdateBackupRecord(record *models.BackupRecord) error
	GetBackupRecords() ([]*models.BackupRecord, error)
	GetBackupRecordsBySettingID(settingID int) ([]*models.BackupRecord, error)
	DeleteBackupRecord(id int) error

	// 定时任务相关
	SaveSchedule(task *models.ScheduledTask) error
	GetAllSchedules() ([]*models.ScheduledTask, error)
	DeleteSchedule(id int) error

	// 关闭存储
	Close() error

	// 分页相关
	GetBackupRecordsWithPage(page, pageSize int) (int, []*models.BackupRecord, error)
}
