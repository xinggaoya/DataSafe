package config

type DatabaseConfig struct {
    Host     string
    Port     int
    User     string
    Password string
    Databases []string
}

type BackupConfig struct {
    BackupDir    string
    Compression  bool
    MaxBackups   int
}

type Config struct {
    Database DatabaseConfig
    Backup   BackupConfig
} 