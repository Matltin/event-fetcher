package eventsdb

import (
	"fmt"
	"math/big"
	"reflect"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// Simplified decoding with component information
func decodeParameterWithComponents(value interface{}, input ABIInput, abiInput abi.Argument) interface{} {
	rv := reflect.ValueOf(value)

	// Handle single [4]uint8 → bytes4
	if rv.Kind() == reflect.Array && rv.Len() == 4 && rv.Type().Elem().Kind() == reflect.Uint8 {
		var b4 [4]byte
		for i := 0; i < 4; i++ {
			b4[i] = byte(rv.Index(i).Uint())
		}
		return fmt.Sprintf("0x%x", b4)
	}

	// Handle slices of [4]uint8 → []bytes4 → []hex
	if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Array &&
		rv.Type().Elem().Len() == 4 && rv.Type().Elem().Elem().Kind() == reflect.Uint8 {
		var result []string
		for i := 0; i < rv.Len(); i++ {
			item := rv.Index(i)
			var b4 [4]byte
			for j := 0; j < 4; j++ {
				b4[j] = byte(item.Index(j).Uint())
			}
			result = append(result, fmt.Sprintf("0x%x", b4))
		}
		return result
	}

	// Handle array of structs using reflection
	if rv.Kind() == reflect.Slice && len(input.Components) > 0 {
		var result []map[string]interface{}
		for i := 0; i < rv.Len(); i++ {
			item := rv.Index(i)
			if item.Kind() == reflect.Struct {
				itemResult := make(map[string]interface{})
				for j, comp := range input.Components {
					if j < item.NumField() {
						field := item.Field(j)
						decoded := decodeParameterWithComponents(field.Interface(), comp, abiInput)
						itemResult[comp.Name] = decoded
					}
				}
				if len(itemResult) > 0 {
					result = append(result, itemResult)
				}
			}
		}
		if len(result) == 1 {
			return result[0] // Flatten single-item arrays
		}
		return result
	}

	// Handle basic Go types
	switch val := value.(type) {
	case *big.Int:
		return val.String()
	case *big.Float:
		return val.String()
	case common.Address:
		return val.Hex()
	case []byte:
		return fmt.Sprintf("0x%x", val)
	case [4]byte:
		return fmt.Sprintf("0x%x", val[:])
	case [32]byte:
		return fmt.Sprintf("0x%x", val[:])
	default:
		return val
	}
}
