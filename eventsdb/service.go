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

	// Load event on database
	if err := s.loadEventSignaturesOnDB(); err != nil {
		return fmt.Errorf("failed to store event on db : %w", err)
	}

	// Load event signatures from database
	if err := s.loadEventSignatures(); err != nil {
		log.Printf("Warning: Failed to load event signatures: %v\n", err)
		log.Println("Continuing without event signature decoding...")
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

	fromBlock, savedBlock := s.calculateStartingBlock(latestBlock)
	contractAddress := common.HexToAddress(s.config.ContractAddr)

	if fromBlock != nil {
		fmt.Printf("Fetching events from block %s to %s\n", fromBlock.String(), latestBlock.String())

		fmt.Printf("Processing block range %s to %s\n", fromBlock, latestBlock)
		err = processBlockRange(s.client, s.db, contractAddress, fromBlock, latestBlock, s.eventSigs, s.config.MaxRetries, s.config.RetryDelay)
		if err != nil {
			return fmt.Errorf("failed to process block range %s to %s: %w", fromBlock, latestBlock, err)
		}

	} else {
		latestBlock = savedBlock
	}
	// Start continuous monitoring
	return s.startContinuousMonitoring(contractAddress, latestBlock)
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
	log.Println("Successfully connected to PostgreSQL database")
	return nil
}

func (s *IndexerService) loadEventSignaturesOnDB() error {
	if _, err := os.Stat(s.config.AbiDir); os.IsNotExist(err) {
		return fmt.Errorf("ABI directory %s does not exist, continuing without event signature decoding... ", s.config.AbiDir)
	}

	if err := loadEventSignaturesOnDB(s.db, s.config.AbiDir); err != nil {
		return fmt.Errorf("faild to store event on database: %w", err)
	}

	return nil
}

func (s *IndexerService) loadEventSignatures() error {
	s.eventSigs = make(map[string]EventSignatureInfo)

	loadedSigs, err := loadEventSignatures(s.db)
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
	log.Println("Attempting to connect to RPC endpoint...")
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
		log.Printf("Getting latest block (attempt %d)...\n", i+1)
		ctx, cancel := context.WithTimeout(context.Background(), DefaultConnectionTimeout)
		header, err = s.client.HeaderByNumber(ctx, nil)
		cancel()

		if err == nil {
			break
		}

		if i < s.config.MaxRetries-1 {
			log.Printf("Failed to get latest header (attempt %d): %v. Retrying...\n", i+1, err)
			time.Sleep(s.config.RetryDelay)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get latest header after %d attempts: %w", s.config.MaxRetries, err)
	}

	return header.Number, nil
}

func (s *IndexerService) calculateStartingBlock(latestBlock *big.Int) (*big.Int, *big.Int) {
	var fromBlock *big.Int
	var latestBlockSaved *big.Int
	var counter Cursor
	err := s.db.First(&counter).Error

	if err == nil {
		block := big.NewInt(int64(counter.Count))
		if latestBlock.Cmp(block) < 1 {
			fromBlock = nil
			latestBlockSaved = block
		} else {
			fromBlock = block
			latestBlockSaved = nil
		}
	} else {
		block := big.NewInt(s.config.StartBlock)
		if latestBlock.Cmp(block) < 1 {
			fromBlock = nil
			latestBlockSaved = block
		} else {
			fromBlock = block
			latestBlockSaved = nil
		}
	}

	return fromBlock, latestBlockSaved
}

func (s *IndexerService) startContinuousMonitoring(contractAddress common.Address, lastProcessedBlock *big.Int) error {
	log.Println("----------------------------------------")
	log.Println("Starting continuous event monitoring...")

	for {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultConnectionTimeout)
		header, err := s.client.HeaderByNumber(ctx, nil)
		cancel()

		if err != nil {
			log.Printf("Error getting latest block: %v. Retrying in %v...\n", err, s.config.RetryDelay)
			time.Sleep(s.config.RetryDelay)

			// Try to reconnect
			if reconnectErr := s.reconnectToBlockchain(); reconnectErr != nil {
				log.Printf("Failed to reconnect: %v\n", reconnectErr)
				continue
			}
			continue
		}

		currentBlock := header.Number

		if currentBlock.Cmp(lastProcessedBlock) > 0 {
			fromBlock := new(big.Int).Add(lastProcessedBlock, big.NewInt(1))
			log.Printf("New block(s) detected! Checking for events from block %s to %s\n",
				fromBlock.String(), currentBlock.String())

			processBlockRange(s.client, s.db, contractAddress, fromBlock, currentBlock, s.eventSigs, s.config.MaxRetries, s.config.RetryDelay)
			lastProcessedBlock = currentBlock
		}

		time.Sleep(DefaultPollingInterval)
	}
}

func (s *IndexerService) reconnectToBlockchain() error {
	newClient, err := connectWithRetry(s.config.RPC, s.config.MaxRetries, s.config.RetryDelay)
	if err != nil {
		return err
	}

	s.client.Close()
	s.client = newClient
	return nil
}
