package eventsdb

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"gorm.io/gorm"
)

func processBlockRange(client *ethclient.Client, db *gorm.DB, contractAddress common.Address, fromBlock, toBlock *big.Int, eventSigs map[string]EventSignatureInfo, maxRetries int, retryDelay time.Duration) {
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
			fmt.Printf("Failed to filter logs (attempt %d): %v. Retrying...\n", i+1, err)
			time.Sleep(retryDelay)
		}
	}

	if err != nil {
		fmt.Printf("Failed to filter logs after %d attempts: %v\n", maxRetries, err)
		return
	}

	if len(logs) > 0 {
		fmt.Printf("Found %d events\n", len(logs))
		for _, log := range logs {
			printEventLog(log, eventSigs)

			var eventSig *EventSignatureInfo
			if len(log.Topics) > 0 {
				topicHex := log.Topics[0].Hex()
				if sig, exists := eventSigs[topicHex]; exists {
					eventSig = &sig
				}
			}

			err = storeEvent(db, log, eventSig)
			if err != nil {
				fmt.Printf("Failed to store event: %v\n", err)
			} else {
				fmt.Printf("Event stored in database successfully (TxHash: %s, LogIndex: %d)\n",
					log.TxHash.Hex(), log.Index)
			}
		}
	} else {
		fmt.Println("No new events found")
	}
}

func printEventLog(log types.Log, eventSigs map[string]EventSignatureInfo) {
	fmt.Println("----------------------------------------")
	fmt.Printf("TxHash: %s\n", log.TxHash.Hex())
	fmt.Printf("TxIndex: %d\n", log.TxIndex)
	fmt.Printf("Block Number: %d\n", log.BlockNumber)
	fmt.Printf("Block Hash: %s\n", log.BlockHash.Hex())
	fmt.Printf("Log Index: %d\n", log.Index)
	fmt.Printf("Removed: %t\n", log.Removed)

	if len(log.Topics) == 0 {
		fmt.Println("No topics in log")
		return
	}

	topicHex := log.Topics[0].Hex()
	fmt.Printf("Event Signature: %s\n", topicHex)

	eventSig, exists := eventSigs[topicHex]
	if !exists {
		fmt.Println("Event: Unknown (signature not found in loaded ABIs)")

		for i, topic := range log.Topics {
			if i == 0 {
				fmt.Printf("Topic 0 (Event Signature): %s\n", topic.Hex())
			} else {
				fmt.Printf("Topic %d: %s\n", i, topic.Hex())
			}
		}

		if len(log.Data) > 0 {
			fmt.Printf("Raw Data: %x\n", log.Data)
		}
		return
	}

	fmt.Printf("Event: %s\n", eventSig.Signature)

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
	fmt.Println("Indexed Parameters:")
	for i, input := range indexedInputs {
		topicIndex := i + 1
		if topicIndex < len(log.Topics) {
			topic := log.Topics[topicIndex]
			fmt.Printf("  %s (%s): %s\n", input.Name, input.Type.String(), topic.Hex())

			switch input.Type.T {
			case abi.AddressTy:
				fmt.Printf("    Decoded: %s\n", common.HexToAddress(topic.Hex()).Hex())
			case abi.IntTy, abi.UintTy:
				val := big.NewInt(0).SetBytes(topic.Bytes())
				fmt.Printf("    Decoded: %s\n", val.String())
			}
		}
	}

	// Process non-indexed parameters
	if len(log.Data) > 0 && len(nonIndexedInputs) > 0 {
		fmt.Println("Non-Indexed Parameters:")
		fmt.Printf("  Raw Data: %x\n", log.Data)
		fmt.Println("  Decoded:")

		method := abi.NewMethod(eventSig.Name, eventSig.Name, abi.Function, "", false, false, nonIndexedInputs, nil)

		v, err := method.Inputs.UnpackValues(log.Data)
		if err != nil {
			fmt.Printf("    Error decoding data: %v\n", err)
		} else {
			for i, input := range nonIndexedInputs {
				if i < len(v) {
					fmt.Printf("    %s (%s): %v\n", input.Name, input.Type.String(), v[i])
				}
			}
		}
	}
}

// Modified storeEvent function with simplified parameter handling
func storeEvent(db *gorm.DB, log types.Log, eventSig *EventSignatureInfo) error {
	eventName := "Unknown"
	fullSignature := "Unknown"

	if eventSig != nil {
		eventName = eventSig.Name
		fullSignature = eventSig.Signature
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

	event := BlockchainEvent{
		TxHash:             log.TxHash.Hex(),
		TxIndex:            uint(log.TxIndex),
		BlockNumber:        log.BlockNumber,
		BlockHash:          log.BlockHash.Hex(),
		LogIndex:           uint(log.Index),
		Removed:            log.Removed,
		ContractAddress:    log.Address.Hex(),
		EventSignature:     log.Topics[0].Hex(),
		EventName:          eventName,
		EventFullSignature: fullSignature,
		OtherTopics:        otherTopics,
		RawData:            rawData,
		DecodedParams:      decodedParamsJSON,
	}

	result := db.FirstOrCreate(&event, BlockchainEvent{
		TxHash:   log.TxHash.Hex(),
		LogIndex: uint(log.Index),
	})

	if result.Error != nil {
		return fmt.Errorf("failed to store event: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		result = db.Model(&BlockchainEvent{}).
			Where("tx_hash = ? AND log_index = ?", log.TxHash.Hex(), log.Index).
			Updates(map[string]interface{}{
				"tx_index":             uint(log.TxIndex),
				"block_number":         log.BlockNumber,
				"block_hash":           log.BlockHash.Hex(),
				"removed":              log.Removed,
				"contract_address":     log.Address.Hex(),
				"event_signature":      log.Topics[0].Hex(),
				"event_name":           eventName,
				"event_full_signature": fullSignature,
				"other_topics":         otherTopics,
				"raw_data":             rawData,
				"decoded_params":       decodedParamsJSON,
			})
		if result.Error != nil {
			return fmt.Errorf("failed to update event: %w", result.Error)
		}
	}

	return nil
}
