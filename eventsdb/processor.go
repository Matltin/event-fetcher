package eventsdb

import (
	"context"
	"encoding/json"
	"fmt"
	logger "log"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func processBlockRange(client *ethclient.Client, db *gorm.DB, contractAddress common.Address, fromBlock, toBlock *big.Int, eventSigs map[string]EventSignatureInfo, maxRetries int, retryDelay time.Duration) error {
	if client == nil {
		return fmt.Errorf("client is nil")
	}
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	if fromBlock == nil || toBlock == nil {
		return fmt.Errorf("block numbers cannot be nil")
	}

	query := ethereum.FilterQuery{
		FromBlock: fromBlock,
		ToBlock:   toBlock,
		Addresses: []common.Address{contractAddress},
	}

	var logs []types.Log
	var err error

	for i := 0; i < maxRetries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultConnectionTimeout)
		logs, err = client.FilterLogs(ctx, query)
		cancel()

		if err == nil {
			break
		}

		if i < maxRetries-1 {
			logger.Printf("Failed to filter logs (attempt %d): %v. Retrying...\n", i+1, err)
			time.Sleep(retryDelay)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to filter logs after %d attempts: %v", maxRetries, err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to start transaction: %v", tx.Error)
	}

	if len(logs) == 0 {
		logger.Println("No event found")
		err = storeCursor(tx, toBlock)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to store Cursor: %v", err)
		}
		if err := tx.Commit().Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to commit transaction: %v", err)
		}
		return nil
	}

	logger.Printf("Found %d events\n", len(logs))
	for _, log := range logs {
		var eventSig *EventSignatureInfo
		if len(log.Topics) > 0 {
			topicHex := log.Topics[0].Hex()
			if sig, exists := eventSigs[topicHex]; exists {
				eventSig = &sig
			}
		}

		err = storeEvent(tx, log, eventSig)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to store event: %v", err)
		}

		logger.Printf("Event stored in database successfully (BlockNumber: %d, TxHash: %s, LogIndex: %d)\n",
			log.BlockNumber, log.TxHash.Hex(), log.Index)
	}

	err = storeCursor(tx, toBlock)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to store Cursor: %v", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	return nil
}

func printEventLog(log types.Log, eventSigs map[string]EventSignatureInfo) {
	logger.Println("----------------------------------------")
	logger.Printf("TxHash: %s\n", log.TxHash.Hex())
	logger.Printf("TxIndex: %d\n", log.TxIndex)
	logger.Printf("Block Number: %d\n", log.BlockNumber)
	logger.Printf("Block Hash: %s\n", log.BlockHash.Hex())
	logger.Printf("Log Index: %d\n", log.Index)
	logger.Printf("Removed: %t\n", log.Removed)

	if len(log.Topics) == 0 {
		logger.Println("No topics in log")
		return
	}

	topicHex := log.Topics[0].Hex()
	logger.Printf("Event Signature: %s\n", topicHex)

	eventSig, exists := eventSigs[topicHex]
	if !exists {
		logger.Println("Event: Unknown (signature not found in loaded ABIs)")

		for i, topic := range log.Topics {
			if i == 0 {
				logger.Printf("Topic 0 (Event Signature): %s\n", topic.Hex())
			} else {
				logger.Printf("Topic %d: %s\n", i, topic.Hex())
			}
		}

		if len(log.Data) > 0 {
			logger.Printf("Raw Data: %x\n", log.Data)
		}
		return
	}

	logger.Printf("Event: %s\n", eventSig.Signature)

	var indexedInputs []abi.Argument
	var nonIndexedInputs []abi.Argument
	for _, input := range eventSig.Inputs {
		if input.Indexed {
			indexedInputs = append(indexedInputs, input)
		} else {
			nonIndexedInputs = append(nonIndexedInputs, input)
		}
	}

	// Process indexed parameters
	logger.Println("Indexed Parameters:")
	for i, input := range indexedInputs {
		topicIndex := i + 1
		if topicIndex < len(log.Topics) {
			topic := log.Topics[topicIndex]
			fmt.Printf("  %s (%s): %s\n", input.Name, input.Type.String(), topic.Hex())

			switch input.Type.T {
			case abi.AddressTy:
				logger.Printf("    Decoded: %s\n", common.HexToAddress(topic.Hex()).Hex())
			case abi.IntTy, abi.UintTy:
				val := big.NewInt(0).SetBytes(topic.Bytes())
				logger.Printf("    Decoded: %s\n", val.String())
			}
		}
	}

	// Process non-indexed parameters
	if len(log.Data) > 0 && len(nonIndexedInputs) > 0 {
		logger.Println("Non-Indexed Parameters:")
		logger.Printf("  Raw Data: %x\n", log.Data)
		logger.Println("  Decoded:")

		method := abi.NewMethod(eventSig.Name, eventSig.Name, abi.Function, "", false, false, nonIndexedInputs, nil)

		v, err := method.Inputs.UnpackValues(log.Data)
		if err != nil {
			logger.Printf("    Error decoding data: %v\n", err)
		} else {
			for i, input := range nonIndexedInputs {
				if i < len(v) {
					fmt.Printf("    %s (%s): %v\n", input.Name, input.Type.String(), v[i])
				}
			}
		}
	}
}

// Modified storeEvent function with upsert and transaction support
func storeEvent(tx *gorm.DB, log types.Log, eventSig *EventSignatureInfo) error {
	var eventName *string
	var fullSignature *string

	if eventSig != nil {
		eventName = &eventSig.Name
		fullSignature = &eventSig.Signature
	}

	otherTopics := make([]string, 0, len(log.Topics)-1)
	for i := 1; i < len(log.Topics); i++ {
		otherTopics = append(otherTopics, log.Topics[i].Hex())
	}

	decodedParams := make(map[string]interface{})
	if eventSig != nil && eventSig.OriginalABI != nil {
		var indexedInputs []abi.Argument
		var nonIndexedInputs []abi.Argument
		var originalIndexedInputs []ABIInput
		var originalNonIndexedInputs []ABIInput

		// Separate indexed and non-indexed inputs
		for i, input := range eventSig.Inputs {
			if input.Indexed {
				indexedInputs = append(indexedInputs, input)
				if i < len(eventSig.OriginalABI.Inputs) {
					originalIndexedInputs = append(originalIndexedInputs, eventSig.OriginalABI.Inputs[i])
				}
			} else {
				nonIndexedInputs = append(nonIndexedInputs, input)
				if i < len(eventSig.OriginalABI.Inputs) {
					originalNonIndexedInputs = append(originalNonIndexedInputs, eventSig.OriginalABI.Inputs[i])
				}
			}
		}

		// Process indexed parameters (topics)
		for i, input := range indexedInputs {
			topicIndex := i + 1
			if topicIndex < len(log.Topics) {
				topic := log.Topics[topicIndex]

				var decodedValue interface{}
				switch input.Type.T {
				case abi.AddressTy:
					decodedValue = common.HexToAddress(topic.Hex())
				case abi.IntTy, abi.UintTy:
					decodedValue = big.NewInt(0).SetBytes(topic.Bytes())
				case abi.BoolTy:
					decodedValue = topic.Bytes()[31] == 1
				case abi.StringTy:
					decodedValue = string(topic.Bytes())
				case abi.FixedBytesTy, abi.BytesTy:
					decodedValue = fmt.Sprintf("%x", topic.Bytes())
				default:
					decodedValue = topic.Bytes()
				}

				// Use simplified decoding
				if i < len(originalIndexedInputs) {
					decodedParams[input.Name] = decodeParameterWithComponents(decodedValue, originalIndexedInputs[i], input)
				} else {
					decodedParams[input.Name] = decodedValue
				}
			}
		}

		// Process non-indexed parameters from data
		if len(log.Data) > 0 && len(nonIndexedInputs) > 0 {
			method := abi.NewMethod(eventSig.Name, eventSig.Name, abi.Function, "", false, false, nonIndexedInputs, nil)

			v, err := method.Inputs.UnpackValues(log.Data)
			if err == nil {
				for i, input := range nonIndexedInputs {
					if i < len(v) {
						// Use simplified decoding
						if i < len(originalNonIndexedInputs) {
							decodedParams[input.Name] = decodeParameterWithComponents(v[i], originalNonIndexedInputs[i], input)
						} else {
							decodedParams[input.Name] = v[i]
						}
					}
				}
			}
		}
	}

	decodedParamsJSON, err := json.Marshal(decodedParams)
	if err != nil {
		return fmt.Errorf("failed to marshal decoded parameters: %w", err)
	}

	rawData := fmt.Sprintf("%x", log.Data)

	logTopic := ""
	if len(log.Topics) != 0 {
		logTopic = log.Topics[0].Hex()
	}

	event := BlockchainEvent{
		TxHash:             log.TxHash.Hex(),
		TxIndex:            uint(log.TxIndex),
		BlockNumber:        log.BlockNumber,
		BlockHash:          log.BlockHash.Hex(),
		LogIndex:           uint(log.Index),
		Removed:            log.Removed,
		ContractAddress:    log.Address.Hex(),
		EventSignature:     logTopic,
		EventName:          eventName,
		EventFullSignature: fullSignature,
		OtherTopics:        otherTopics,
		RawData:            rawData,
		DecodedParams:      decodedParamsJSON,
	}

	// Use upsert (OnConflict) to avoid duplicate key errors
	result := tx.Clauses(
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "tx_hash"}, {Name: "log_index"}},
			DoUpdates: clause.AssignmentColumns([]string{"tx_index", "block_number", "block_hash", "removed", "contract_address", "event_signature", "event_name", "event_full_signature", "other_topics", "raw_data", "decoded_params"}),
		},
	).Create(&event)

	if result.Error != nil {
		return fmt.Errorf("failed to store event: %w", result.Error)
	}

	return nil
}

// Update storeCursor to use transaction
func storeCursor(tx *gorm.DB, c *big.Int) error {
	var counter Cursor
	if err := tx.First(&counter, 1).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create the counter if not exists
			counter = Cursor{
				ID:    1,
				Count: int(c.Int64()),
			}
			if err := tx.Create(&counter).Error; err != nil {
				return fmt.Errorf("failed to create counter: %w", err)
			}
		} else {
			return fmt.Errorf("failed to query counter: %w", err)
		}
	} else {
		// Update the existing counter
		if err := tx.Model(&Cursor{}).
			Where("id = ?", 1).
			Update("count", int(c.Int64())).Error; err != nil {
			return fmt.Errorf("failed to update counter: %w", err)
		}
	}

	return nil
}
