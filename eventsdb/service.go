package eventsdb

import (
	"log"

	"github.com/ethereum/go-ethereum/ethclient"
	"gorm.io/gorm"
)

// IndexerService handles the main application logic
type IndexerService struct {
	config Config
	db     *gorm.DB
	client *ethclient.Client
}

// NewIndexerService creates a new indexer service
func NewIndexerService(config Config) *IndexerService {
	return &IndexerService{
		config: config,
	}
}

func (s *IndexerService) Start() error {
	s.printConfiguration()

	return nil
}

func (s *IndexerService) printConfiguration() {
	log.Println("Configuration:")
	log.Printf("  RPC Endpoint: %s\n", s.config.RPC)
	log.Printf("  Contract: %s\n", s.config.ContractAddr)
	log.Printf("  ABI Directory: %s\n", s.config.AbiDir)
	log.Printf("  Start Block: %d\n", s.config.StartBlock)
	log.Printf("  Max Retries: %d\n", s.config.MaxRetries)
	log.Printf("  Retry Delay: %v\n", s.config.RetryDelay)
	log.Printf("  GORM Logs: %t\n", s.config.EnableGormLogs)
	log.Printf("  Postgres: %s:%s@%s:%s/%s\n", s.config.PgUser, "******", s.config.PgHost, s.config.PgPort, s.config.PgDbName)
}
