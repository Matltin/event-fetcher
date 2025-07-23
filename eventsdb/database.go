package eventsdb

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func initDB(config Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		config.PgHost, config.PgPort, config.PgUser, config.PgPassword, config.PgDbName)

	logLevel := logger.Silent
	if config.EnableGormLogs {
		logLevel = logger.Info
	}

	gormLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logLevel,
			IgnoreRecordNotFoundError: true,
			Colorful:                  config.EnableGormLogs,
		},
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// AutoMigrate
	err = db.AutoMigrate(&BlockchainEvent{}, &ABIEventRecord{}, &Cursor{})
	if err != nil {
		return nil, fmt.Errorf("failed to migrate schema: %w", err)
	}

	return db, nil
}
