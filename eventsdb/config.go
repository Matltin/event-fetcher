package eventsdb

import (
	"math/big"
	"os"
	"strings"
	"time"
)

// Constants to avoid magic numbers
const (
	DefaultConnectionTimeout = 30 * time.Second
	DefaultPollingInterval   = 2 * time.Second
	DefaultMaxRetries        = 100
	DefaultRetryDelay        = 5 * time.Second
	DefaultMaxBlockRange     = 10_000
	DefaultFinalityBlock     = 10
)

// Configuration for the application
type Config struct {
	RPC            string
	ContractAddr   string
	AbiDir         string
	StartBlock     int64
	FinalityBlock  int64
	PgHost         string
	PgPort         string
	PgUser         string
	PgPassword     string
	PgDbName       string
	MaxRetries     int
	MaxBlockRange  int64
	RetryDelay     time.Duration
	EnableGormLogs bool
}

func LoadConfig() Config {
	config := Config{
		RPC:            "https://0xrpc.io/base",
		ContractAddr:   "0x91Cf2D8Ed503EC52768999aA6D8DBeA6e52dbe43", // SYMMIO on BASE
		AbiDir:         "./abi",
		StartBlock:     8443806, // first block
		FinalityBlock:  DefaultFinalityBlock,
		PgHost:         "127.0.0.1",
		PgPort:         "15432",
		PgUser:         "postgres",
		PgPassword:     "postgres",
		PgDbName:       "postgres",
		MaxRetries:     DefaultMaxRetries,
		RetryDelay:     DefaultRetryDelay,
		MaxBlockRange:  DefaultMaxBlockRange,
		EnableGormLogs: false,
	}

	if rpc := os.Getenv("RPC_URL"); rpc != "" {
		config.RPC = rpc
	}
	if logFlag := os.Getenv("ENABLE_GORM_LOGS"); strings.ToLower(logFlag) == "true" {
		config.EnableGormLogs = true
	}
	if contractAddr := os.Getenv("CONTRACT_ADDRESS"); contractAddr != "" {
		config.ContractAddr = contractAddr
	}
	if abiDir := os.Getenv("ABI_DIR"); abiDir != "" {
		config.AbiDir = abiDir
	}
	if blocksStr := os.Getenv("START_BLOCK"); blocksStr != "" {
		if blocks, ok := big.NewInt(0).SetString(blocksStr, 10); ok {
			config.StartBlock = blocks.Int64()
			if config.StartBlock < 1 {
				config.StartBlock = 1
			}
		}
	}
	if finalityBlockStr := os.Getenv("FINALITY_BLOCK"); finalityBlockStr != "" {
		if finality, ok := big.NewInt(0).SetString(finalityBlockStr, 10); ok {
			config.FinalityBlock = finality.Int64()
		}
	}
	if pgHost := os.Getenv("PG_HOST"); pgHost != "" {
		config.PgHost = pgHost
	}
	if pgPort := os.Getenv("PG_PORT"); pgPort != "" {
		config.PgPort = pgPort
	}
	if pgUser := os.Getenv("PG_USER"); pgUser != "" {
		config.PgUser = pgUser
	}
	if pgPassword := os.Getenv("PG_PASSWORD"); pgPassword != "" {
		config.PgPassword = pgPassword
	}
	if pgDbName := os.Getenv("PG_DBNAME"); pgDbName != "" {
		config.PgDbName = pgDbName
	}
	if maxRetries := os.Getenv("MAX_RETRIES"); maxRetries != "" {
		if retries, ok := big.NewInt(0).SetString(maxRetries, 10); ok {
			config.MaxRetries = int(retries.Int64())
		}
	}
	if blockRange := os.Getenv("MAX_BLOCK_RANGE"); blockRange != "" {
		if maxRange, ok := big.NewInt(0).SetString(blockRange, 10); ok {
			if maxRange.Int64() > 0 {
				config.MaxBlockRange = maxRange.Int64()
			}
		}
	}
	if retryDelay := os.Getenv("RETRY_DELAY_SECONDS"); retryDelay != "" {
		if delay, ok := big.NewInt(0).SetString(retryDelay, 10); ok {
			config.RetryDelay = time.Duration(delay.Int64()) * time.Second
		}
	}

	return config
}
