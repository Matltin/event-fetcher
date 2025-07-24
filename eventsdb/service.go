package eventsdb

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
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

	if err := s.connectToBlockchain(); err != nil {
		return fmt.Errorf("failed to connect to blockchain: %w", err)
	}
	defer s.client.Close()

	// Get latest block and calculate starting block
	latestBlock, err := s.getLatestBlock()
	if err != nil {
		return fmt.Errorf("failed to get latest block: %w", err)
	}

	fromBlock := s.calculateStartingBlock(latestBlock)
	contractAddress := common.HexToAddress(s.config.ContractAddr)


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

func (s *IndexerService) connectToBlockchain() error {
	// Validate RPC URL format
	if !strings.HasPrefix(s.config.RPC, "http://") && !strings.HasPrefix(s.config.RPC, "https://") &&
		!strings.HasPrefix(s.config.RPC, "ws://") && !strings.HasPrefix(s.config.RPC, "wss://") {
		return fmt.Errorf("invalid RPC URL format: %s. Must start with http://, https://, ws://, or wss://", s.config.RPC)
	}

	// Connect to node with retry logic
	fmt.Println("Attempting to connect to RPC endpoint...")
	client, err := connectWithRetry(s.config.RPC, s.config.MaxRetries, s.config.RetryDelay)
	if err != nil {
		return err
	}

	s.client = client
	return nil
}

func (s *IndexerService) getLatestBlock() (*big.Int, error) {
	var header *types.Header
	var err error

	for i := 0; i < s.config.MaxRetries; i++ {
		fmt.Printf("Getting latest block (attempt %d)...\n", i+1)
		ctx, cancel := context.WithTimeout(context.Background(), DefaultConnectionTimeout)
		header, err = s.client.HeaderByNumber(ctx, nil)
		cancel()

		if err == nil {
			break
		}

		if i < s.config.MaxRetries-1 {
			fmt.Printf("Failed to get latest header (attempt %d): %v. Retrying...\n", i+1, err)
			time.Sleep(s.config.RetryDelay)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get latest header after %d attempts: %w", s.config.MaxRetries, err)
	}

	return header.Number, nil
}

func (s *IndexerService) calculateStartingBlock(latestBlock *big.Int) *big.Int {
	var fromBlock *big.Int

	if s.config.StartBlock == 0 {
		fromBlock = new(big.Int).Set(latestBlock)
		fmt.Printf("Starting from the latest block: %s\n", fromBlock.String())
	} else if s.config.StartBlock < 0 {
		fromBlock = new(big.Int).Add(latestBlock, big.NewInt(s.config.StartBlock))
		if fromBlock.Cmp(big.NewInt(0)) < 0 {
			fromBlock = big.NewInt(0)
		}
		fmt.Printf("Starting from %d blocks before latest (%s)\n", -s.config.StartBlock, fromBlock.String())
	} else {
		fromBlock = big.NewInt(s.config.StartBlock)
		fmt.Printf("Starting from block number: %s\n", fromBlock.String())
	}

	return fromBlock
}
