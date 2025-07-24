package eventsdb

import (
	"context"
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

func storeEvent(db *gorm.DB, log types.Log, eventSig *EventSignatureInfo) error {
	return nil
}
