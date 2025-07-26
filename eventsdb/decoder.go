package eventsdb

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// Simplified decoding with component information
func decodeParameterWithComponents(value interface{}, input ABIInput, abiInput abi.Argument) interface{} {
	// Handle arrays of structs
	if val, ok := value.([]interface{}); ok && len(input.Components) > 0 {
		var result []map[string]interface{}
		for _, item := range val {
			if itemSlice, ok := item.([]interface{}); ok {
				itemResult := make(map[string]interface{})
				for i, comp := range input.Components {
					if i < len(itemSlice) {
						itemResult[comp.Name] = decodeParameterWithComponents(itemSlice[i], comp, abiInput)
					}
				}
				if len(itemResult) > 0 {
					result = append(result, itemResult)
				}
			}
		}
		if len(result) == 1 {
			return result[0] // Return single struct as map
		}
		return result
	}

	// Handle basic types
	switch val := value.(type) {
	case *big.Int:
		return val.String()
	case *big.Float:
		return val.String()
	case common.Address:
		return val.Hex()
	case []byte:
		return fmt.Sprintf("%x", val)
	case [32]byte:
		return fmt.Sprintf("%x", val[:])
	default:
		// For simple values, just return them directly
		return val
	}
}
