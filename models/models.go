package models

// DBSettings 数据库配置结构
type DBSettings struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	User       string `json:"user"`
	Password   string `json:"password"`
	BackupDir  string `json:"backupDir"`
	MaxBackups int    `json:"maxBackups"` // 保留的最大备份数量，0表示不限制
}

// BackupRecord 备份记录结构
type BackupRecord struct {
	ID        int    `json:"id"`
	SettingID int    `json:"settingId"`
	DBName    string `json:"dbName"`
	FileName  string `json:"fileName"`
	CreatedAt string `json:"createdAt"`
	Status    string `json:"status"` // "completed", "failed", "in_progress"
	Error     string `json:"error"`  // 错误信息
}

// BackupRequest 备份请求结构
type BackupRequest struct {
	SettingID int    `json:"settingId"`
	Database  string `json:"database"`
	Schedule  string `json:"schedule,omitempty"`
}

// ScheduledTask 定时任务结构
type ScheduledTask struct {
	ID        int    `json:"id"`
	SettingID int    `json:"settingId"`
	Database  string `json:"database"`
	Schedule  string `json:"schedule"`
}

// PageRequest 分页请求参数
type PageRequest struct {
	Page     int `form:"page" json:"page"`         // 当前页码
	PageSize int `form:"pageSize" json:"pageSize"` // 每页数量
}

// PageResponse 分页响应结构
type PageResponse struct {
	Total int         `json:"total"` // 总记录数
	Data  interface{} `json:"data"`  // 数据列表
}
