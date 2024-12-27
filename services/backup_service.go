package services

import (
	"database/sql"
	"fmt"
	"mysql-backup/config"
	"mysql-backup/models"
	"mysql-backup/storage"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type BackupService struct {
	config *config.Config
}

func NewBackupService() *BackupService {
	return &BackupService{}
}

func (s *BackupService) BackupDatabase(dbName string) error {
	timestamp := time.Now().Format("20060102150405")
	filename := fmt.Sprintf("%s/%s_%s.sql", s.config.Backup.BackupDir, dbName, timestamp)

	cmd := exec.Command("mysqldump",
		"-h", s.config.Database.Host,
		"-P", fmt.Sprintf("%d", s.config.Database.Port),
		"-u", s.config.Database.User,
		fmt.Sprintf("-p%s", s.config.Database.Password),
		dbName,
	)

	if s.config.Backup.Compression {
		filename += ".gz"
		gzip := exec.Command("gzip")
		pipe, _ := gzip.StdinPipe()

		cmd.Stdout = pipe
		gzip.Stdout, _ = os.Create(filename)

		gzip.Start()
		err := cmd.Run()
		pipe.Close()
		gzip.Wait()

		return err
	}

	outFile, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer outFile.Close()

	cmd.Stdout = outFile
	return cmd.Run()
}

func (s *BackupService) GetDatabases() ([]string, error) {
	// 执行 MySQL 命令列出所有数据库
	cmd := exec.Command("mysql",
		"-h", s.config.Database.Host,
		"-P", fmt.Sprintf("%d", s.config.Database.Port),
		"-u", s.config.Database.User,
		fmt.Sprintf("-p%s", s.config.Database.Password),
		"-e", "SHOW DATABASES",
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// 解析输出，跳过第一行（标题行）
	databases := strings.Split(string(output), "\n")[1:]
	// 过滤掉空行和系统数据库
	var result []string
	for _, db := range databases {
		db = strings.TrimSpace(db)
		if db != "" && !strings.HasPrefix(db, "information_schema") &&
			!strings.HasPrefix(db, "performance_schema") &&
			!strings.HasPrefix(db, "mysql") &&
			!strings.HasPrefix(db, "sys") {
			result = append(result, db)
		}
	}

	return result, nil
}

func (s *BackupService) ScheduleBackup(database, schedule string) (int, error) {
	// 这里应该实现定时任务的添加逻辑
	// 返回任务ID和可能的错误
	// TODO: 实现实际的定时任务调度逻辑
	return 1, nil
}

func (s *BackupService) GetDatabasesWithConfig(setting *models.DBSettings) ([]string, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/",
		setting.User,
		setting.Password,
		setting.Host,
		setting.Port,
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %v", err)
	}
	defer db.Close()

	// 测试连接
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("连接测试失败: %v", err)
	}

	// 查询所有数据库
	rows, err := db.Query("SHOW DATABASES")
	if err != nil {
		return nil, fmt.Errorf("查询数据库列表失败: %v", err)
	}
	defer rows.Close()

	var databases []string
	for rows.Next() {
		var dbName string
		if err := rows.Scan(&dbName); err != nil {
			return nil, fmt.Errorf("读取数据库名称失败: %v", err)
		}
		databases = append(databases, dbName)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历数据库列表失败: %v", err)
	}

	// 过滤系统数据库
	return filterSystemDatabases(databases), nil
}

func (s *BackupService) BackupDatabaseWithConfig(setting *models.DBSettings, dbName string, store storage.Store) error {
	// 创建备份记录
	record := &models.BackupRecord{
		DBName:    dbName,
		FileName:  fmt.Sprintf("%s_%s.sql", dbName, time.Now().Format("20060102150405")),
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
		Status:    "in_progress",
		SettingID: setting.ID,
	}

	// 保存初始记录
	if err := store.SaveBackupRecord(record); err != nil {
		return fmt.Errorf("保存备份记录失败: %v", err)
	}

	// 执行备份
	err := s.performBackup(setting, dbName, record.FileName)

	// 更新备份状态
	if err != nil {
		record.Status = "failed"
		record.Error = err.Error()
	} else {
		record.Status = "completed"
	}

	// 更新记录
	if updateErr := store.UpdateBackupRecord(record); updateErr != nil {
		// 如果更新记录失败，但备份成功，返回更新错误
		if err == nil {
			return fmt.Errorf("备份成功但更新记录失败: %v", updateErr)
		}
	}

	return err
}

// performBackup 执行实际的备份操作
func (s *BackupService) performBackup(setting *models.DBSettings, dbName, fileName string) error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
		setting.User,
		setting.Password,
		setting.Host,
		setting.Port,
		dbName,
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("连接数据库失败: %v", err)
	}
	defer db.Close()

	// 确保备份目录存在
	if err := os.MkdirAll(setting.BackupDir, 0755); err != nil {
		return fmt.Errorf("创建备份目录失败: %v", err)
	}

	// 获取所有表
	rows, err := db.Query("SHOW TABLES")
	if err != nil {
		return fmt.Errorf("获取表列表失败: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return fmt.Errorf("读取表名失败: %v", err)
		}
		tables = append(tables, tableName)
	}

	// 创建备份文件
	timestamp := time.Now().Format("20060102150405")
	filename := filepath.Join(setting.BackupDir, fmt.Sprintf("%s_%s.sql", dbName, timestamp))
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建备份文件失败: %v", err)
	}
	defer file.Close()

	// 写入数据库创建语句
	fmt.Fprintf(file, "CREATE DATABASE IF NOT EXISTS `%s`;\n", dbName)
	fmt.Fprintf(file, "USE `%s`;\n\n", dbName)

	// 备份每个表的结构和数据
	for _, table := range tables {
		// 获取表结构
		var createTable string
		err := db.QueryRow("SHOW CREATE TABLE `"+table+"`").Scan(&table, &createTable)
		if err != nil {
			return fmt.Errorf("获取表 %s 的结构失败: %v", table, err)
		}
		fmt.Fprintln(file, createTable+";\n")

		// 获取表数据
		rows, err := db.Query("SELECT * FROM `" + table + "`")
		if err != nil {
			return fmt.Errorf("读取表 %s 的数据失败: %v", table, err)
		}

		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			return fmt.Errorf("获取表 %s 的列信息失败: %v", table, err)
		}

		// 准备数据
		values := make([]interface{}, len(columns))
		scanArgs := make([]interface{}, len(columns))
		for i := range values {
			scanArgs[i] = &values[i]
		}

		// 写入数据
		for rows.Next() {
			err := rows.Scan(scanArgs...)
			if err != nil {
				rows.Close()
				return fmt.Errorf("读取行数据失败: %v", err)
			}

			fmt.Fprintf(file, "INSERT INTO `%s` VALUES (", table)
			for i, value := range values {
				if i > 0 {
					fmt.Fprint(file, ",")
				}
				fmt.Fprint(file, formatValue(value))
			}
			fmt.Fprintln(file, ");")
		}
		rows.Close()
		fmt.Fprintln(file)
	}

	// 在备份完成后清理旧文件
	if err := s.cleanOldBackups(setting, dbName); err != nil {
		return fmt.Errorf("清理旧备份失败: %v", err)
	}

	return nil
}

// 格式化值为 SQL 字符串
func formatValue(value interface{}) string {
	if value == nil {
		return "NULL"
	}
	switch v := value.(type) {
	case []byte:
		return fmt.Sprintf("'%s'", escapeString(string(v)))
	case string:
		return fmt.Sprintf("'%s'", escapeString(v))
	case time.Time:
		return fmt.Sprintf("'%s'", v.Format("2006-01-02 15:04:05"))
	default:
		return fmt.Sprintf("%v", v)
	}
}

// 转义 SQL 字符串
func escapeString(s string) string {
	return strings.NewReplacer(
		"'", "\\'",
		"\"", "\\\"",
		"\n", "\\n",
		"\r", "\\r",
		"\x00", "\\0",
		"\x1a", "\\Z",
	).Replace(s)
}

// 辅助函数
func parseDatabases(output string) []string {
	// 解析输出，跳过第一行（标题行）
	databases := strings.Split(string(output), "\n")[1:]
	var result []string
	for _, db := range databases {
		db = strings.TrimSpace(db)
		if db != "" {
			result = append(result, db)
		}
	}
	return result
}

func filterSystemDatabases(databases []string) []string {
	var result []string
	systemDBs := map[string]bool{
		"information_schema": true,
		"performance_schema": true,
		"mysql":              true,
		"sys":                true,
	}

	for _, db := range databases {
		if !systemDBs[db] {
			result = append(result, db)
		}
	}
	return result
}

// TestConnection 测试数据库连接
func (s *BackupService) TestConnection(settings *models.DBSettings) error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/?timeout=5s",
		settings.User,
		settings.Password,
		settings.Host,
		settings.Port,
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("初始化连接失败: %v", err)
	}
	defer db.Close()

	// 测试连接
	if err := db.Ping(); err != nil {
		return fmt.Errorf("连接测试失败: %v", err)
	}

	return nil
}

// ValidateSettings 验证配置
func (s *BackupService) ValidateSettings(settings *models.DBSettings) error {
	if settings.Host == "" {
		return fmt.Errorf("主机地址不能为空")
	}
	if settings.Port <= 0 || settings.Port > 65535 {
		return fmt.Errorf("无效的端口号")
	}
	if settings.User == "" {
		return fmt.Errorf("用户名不能为空")
	}
	if settings.BackupDir == "" {
		return fmt.Errorf("备份目录不能为空")
	}
	return nil
}

// 添加备份文件管理功能
func (s *BackupService) ListBackupFiles(backupDir string) ([]BackupFile, error) {
	files, err := os.ReadDir(backupDir)
	if err != nil {
		return nil, fmt.Errorf("读取备份目录失败: %v", err)
	}

	var backupFiles []BackupFile
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".sql") {
			info, err := file.Info()
			if err != nil {
				continue
			}
			backupFiles = append(backupFiles, BackupFile{
				Name:      file.Name(),
				Size:      info.Size(),
				CreatedAt: info.ModTime().Format("2006-01-02 15:04:05"),
			})
		}
	}
	return backupFiles, nil
}

// DeleteBackupFile 删除备份文件
func (s *BackupService) DeleteBackupFile(backupDir, filename string) error {
	fullPath := filepath.Join(backupDir, filename)
	// 验证文件路径
	if !strings.HasPrefix(fullPath, backupDir) {
		return fmt.Errorf("无效的文件路径")
	}
	if err := os.Remove(fullPath); err != nil {
		return fmt.Errorf("删除文件失败: %v", err)
	}
	return nil
}

// BackupFile 备份文件信息
type BackupFile struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"createdAt"`
}

// cleanOldBackups 清理旧的备份文件
func (s *BackupService) cleanOldBackups(setting *models.DBSettings, dbName string) error {
	if setting.MaxBackups <= 0 {
		return nil // 不限制备份数量
	}

	files, err := os.ReadDir(setting.BackupDir)
	if err != nil {
		return fmt.Errorf("读取备份目录失败: %v", err)
	}

	// 获取指定数据库的备份文件
	var backupFiles []os.DirEntry
	for _, file := range files {
		if !file.IsDir() && strings.HasPrefix(file.Name(), dbName+"_") {
			backupFiles = append(backupFiles, file)
		}
	}

	// 如果备份文件数量超过限制，删除最旧的文件
	if len(backupFiles) > setting.MaxBackups {
		// 按修改时间排序
		sort.Slice(backupFiles, func(i, j int) bool {
			iInfo, _ := backupFiles[i].Info()
			jInfo, _ := backupFiles[j].Info()
			return iInfo.ModTime().After(jInfo.ModTime())
		})

		// 删除多余的文件
		for i := setting.MaxBackups; i < len(backupFiles); i++ {
			filename := backupFiles[i].Name()
			if err := s.DeleteBackupFile(setting.BackupDir, filename); err != nil {
				return fmt.Errorf("删除旧备份文件失败: %v", err)
			}
		}
	}

	return nil
}
