package eventsdb

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// BlockchainEvent model stores all blockchain event data
type BlockchainEvent struct {
	ID                 uint            `gorm:"primaryKey"`
	TxHash             string          `gorm:"not null;type:varchar(66);uniqueIndex:idx_tx_log"` // Keccak hash of the transaction
	TxIndex            uint            `gorm:"not null"`                                         // Transaction index in the block
	BlockNumber        uint64          `gorm:"not null;index"`                                   // Block number
	BlockHash          string          `gorm:"not null;type:varchar(66);index"`                  // Hash of the block
	LogIndex           uint            `gorm:"not null;uniqueIndex:idx_tx_log"`                  // Index in the block's log array
	Removed            bool            `gorm:"not null;default:false"`                           // True if log was removed due to chain reorg
	ContractAddress    string          `gorm:"not null;type:varchar(42);index"`                  // Address of the contract
	EventSignature     string          `gorm:"not null;type:varchar(66);index"`                  // Keccak of the event signature
	EventName          *string         `gorm:"type:varchar(255);index;default:NULL"`             // Human-readable event name (NULL if unknown)
	EventFullSignature *string         `gorm:"type:text;default:NULL"`                           // Full event signature (NULL if unknown)
	OtherTopics        StringArray     `gorm:"type:text[]"`                                      // Additional event topics
	RawData            string          `gorm:"type:text"`                                        // Hex-encoded unindexed log data
	DecodedParams      json.RawMessage `gorm:"type:jsonb"`                                       // Decoded event parameters
	InsertTime         time.Time       `gorm:"not null;default:now()"`                           // When this record was inserted
}

// StringArray handles PostgreSQL string arrays
type StringArray []string

func (sa *StringArray) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("scan source is not []byte")
	}

	str := string(bytes)
	str = strings.Trim(str, "{}")

	if str == "" {
		*sa = []string{}
		return nil
	}

	*sa = strings.Split(str, ",")
	return nil
}

func (sa StringArray) Value() (driver.Value, error) {
	if sa == nil {
		return "{}", nil
	}
	return fmt.Sprintf("{%s}", strings.Join(sa, ",")), nil
}

// ABIEventRecord model stores ABI events json format
type ABIEventRecord struct {
	ID                 uint   `gorm:"primaryKey"`
	EventSignatureHash string `gorm:"uniqueIndex"`
	EventName          string
	ABIEventJSON       string
}

// Coursor count Number of processed block
type Cursor struct {
	ID    uint `gorm:"primaryKey"`
	Count int
}
