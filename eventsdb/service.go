package eventsdb

import (
	"fmt"
	"log"
	"os"

	"github.com/ethereum/go-ethereum/ethclient"
	"gorm.io/gorm"
)

// IndexerService handles the main application logic
type IndexerService struct {
	config    Config
	db        *gorm.DB
	client    *ethclient.Client
	eventSigs map[string]EventSignatureInfo
}

// NewIndexerService creates a new indexer service
func NewIndexerService(config Config) *IndexerService {
	return &IndexerService{
		config: config,
	}
}

func (s *IndexerService) Start() error {
	// Print confuguration
	s.printConfiguration()

	// Initialize database
	if err := s.initializeDatabase(); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// Load event signatures
	if err := s.loadEventSignatures(); err != nil {
		fmt.Printf("Warning: Failed to load event signatures: %v\n", err)
		fmt.Println("Continuing without event signature decoding...")
	}

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

func (s *IndexerService) initializeDatabase() error {
	db, err := initDB(s.config)
	if err != nil {
		return err
	}
	s.db = db
	fmt.Println("Successfully connected to PostgreSQL database")
	return nil
}

func (s *IndexerService) loadEventSignatures() error {
	s.eventSigs = make(map[string]EventSignatureInfo)

	if _, err := os.Stat(s.config.AbiDir); os.IsNotExist(err) {
		fmt.Printf("ABI directory %s does not exist, continuing without event signature decoding...\n", s.config.AbiDir)
		return nil
	}

	loadedSigs, err := loadEventSignatures(s.config.AbiDir)
	if err != nil {
		return err
	}

	s.eventSigs = loadedSigs
	return nil
}
