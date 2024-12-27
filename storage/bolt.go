package storage

import (
	"encoding/json"
	"fmt"
	"mysql-backup/models"
	"sort"
	"time"

	"go.etcd.io/bbolt"
)

type BoltStore struct {
	db *bbolt.DB
}

var (
	settingsBucket  = []byte("settings")
	backupsBucket   = []byte("backups")
	schedulesBucket = []byte("schedules")
)

func NewBoltStore(dbPath string) (*BoltStore, error) {
	db, err := bbolt.Open(dbPath, 0600, &bbolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open bolt db: %v", err)
	}

	// 创建 buckets
	err = db.Update(func(tx *bbolt.Tx) error {
		for _, bucket := range [][]byte{settingsBucket, backupsBucket, schedulesBucket} {
			_, err := tx.CreateBucketIfNotExists(bucket)
			if err != nil {
				return fmt.Errorf("create bucket %s: %v", bucket, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("create buckets: %v", err)
	}

	return &BoltStore{db: db}, nil
}

func (s *BoltStore) SaveSettings(settings *models.DBSettings) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(settingsBucket)

		if settings.ID == 0 {
			id, _ := b.NextSequence()
			settings.ID = int(id)
		}

		key := []byte(fmt.Sprintf("%d", settings.ID))
		value, err := json.Marshal(settings)
		if err != nil {
			return fmt.Errorf("marshal settings: %v", err)
		}

		return b.Put(key, value)
	})
}

func (s *BoltStore) GetAllSettings() ([]models.DBSettings, error) {
	var settings []models.DBSettings

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(settingsBucket)
		return b.ForEach(func(k, v []byte) error {
			var setting models.DBSettings
			if err := json.Unmarshal(v, &setting); err != nil {
				return fmt.Errorf("unmarshal settings: %v", err)
			}
			settings = append(settings, setting)
			return nil
		})
	})

	if err != nil {
		return nil, fmt.Errorf("get settings: %v", err)
	}

	return settings, nil
}

func (s *BoltStore) SaveBackupRecord(record *models.BackupRecord) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(backupsBucket)

		if record.ID == 0 {
			id, _ := b.NextSequence()
			record.ID = int(id)
		}

		key := []byte(fmt.Sprintf("%d", record.ID))
		value, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("marshal backup record: %v", err)
		}

		return b.Put(key, value)
	})
}

func (s *BoltStore) GetBackupRecords() ([]*models.BackupRecord, error) {
	var records []*models.BackupRecord

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(backupsBucket)
		return b.ForEach(func(k, v []byte) error {
			var record models.BackupRecord
			if err := json.Unmarshal(v, &record); err != nil {
				return fmt.Errorf("unmarshal backup record: %v", err)
			}
			records = append(records, &record)
			return nil
		})
	})

	if err != nil {
		return nil, fmt.Errorf("get backup records: %v", err)
	}

	// 按时间倒序排序
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt > records[j].CreatedAt
	})

	return records, nil
}

func (s *BoltStore) GetBackupRecordsBySettingID(settingID int) ([]*models.BackupRecord, error) {
	var records []*models.BackupRecord

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(backupsBucket)
		return b.ForEach(func(k, v []byte) error {
			var record models.BackupRecord
			if err := json.Unmarshal(v, &record); err != nil {
				return fmt.Errorf("unmarshal backup record: %v", err)
			}
			if record.SettingID == settingID {
				records = append(records, &record)
			}
			return nil
		})
	})

	if err != nil {
		return nil, fmt.Errorf("get backup records by setting id: %v", err)
	}

	return records, nil
}

func (s *BoltStore) DeleteBackupRecord(id int) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(backupsBucket)
		return b.Delete([]byte(fmt.Sprintf("%d", id)))
	})
}

func (s *BoltStore) Close() error {
	return s.db.Close()
}

func (s *BoltStore) GetSettingByID(id int) (*models.DBSettings, error) {
	var setting *models.DBSettings

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(settingsBucket)
		v := b.Get([]byte(fmt.Sprintf("%d", id)))
		if v == nil {
			return fmt.Errorf("setting not found: %d", id)
		}

		setting = &models.DBSettings{}
		return json.Unmarshal(v, setting)
	})

	if err != nil {
		return nil, fmt.Errorf("get setting by id: %v", err)
	}

	return setting, nil
}

func (s *BoltStore) UpdateBackupRecord(record *models.BackupRecord) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(backupsBucket)
		if b == nil {
			return fmt.Errorf("backup bucket not found")
		}

		data, err := json.Marshal(record)
		if err != nil {
			return err
		}

		return b.Put([]byte(fmt.Sprintf("%d", record.ID)), data)
	})
}

func (s *BoltStore) SaveSchedule(task *models.ScheduledTask) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(schedulesBucket)

		if task.ID == 0 {
			id, _ := b.NextSequence()
			task.ID = int(id)
		}

		value, err := json.Marshal(task)
		if err != nil {
			return fmt.Errorf("marshal schedule: %v", err)
		}

		return b.Put([]byte(fmt.Sprintf("%d", task.ID)), value)
	})
}

func (s *BoltStore) GetAllSchedules() ([]*models.ScheduledTask, error) {
	var tasks []*models.ScheduledTask

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(schedulesBucket)
		return b.ForEach(func(k, v []byte) error {
			var task models.ScheduledTask
			if err := json.Unmarshal(v, &task); err != nil {
				return fmt.Errorf("unmarshal schedule: %v", err)
			}
			tasks = append(tasks, &task)
			return nil
		})
	})

	if err != nil {
		return nil, fmt.Errorf("get schedules: %v", err)
	}

	return tasks, nil
}

func (s *BoltStore) DeleteSchedule(id int) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(schedulesBucket)
		return b.Delete([]byte(fmt.Sprintf("%d", id)))
	})
}

func (s *BoltStore) GetBackupRecordsWithPage(page, pageSize int) (int, []*models.BackupRecord, error) {
	var records []*models.BackupRecord
	var total int

	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(backupsBucket)

		// 获取总记录数
		total = b.Stats().KeyN

		// 计算分页范围
		start := (page - 1) * pageSize
		end := start + pageSize
		current := 0

		return b.ForEach(func(k, v []byte) error {
			// 跳过不在当前页的记录
			if current < start {
				current++
				return nil
			}
			if current >= end {
				return nil
			}

			var record models.BackupRecord
			if err := json.Unmarshal(v, &record); err != nil {
				return fmt.Errorf("unmarshal backup record: %v", err)
			}
			records = append(records, &record)
			current++
			return nil
		})
	})

	if err != nil {
		return 0, nil, fmt.Errorf("get backup records: %v", err)
	}

	// 按时间倒序排序
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt > records[j].CreatedAt
	})

	return total, records, nil
}
