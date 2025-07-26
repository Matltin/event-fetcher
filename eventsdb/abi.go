package eventsdb

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"golang.org/x/crypto/sha3"
	"gorm.io/gorm"
)

// Enhanced EventSignatureInfo with original ABI information
type EventSignatureInfo struct {
	Name        string
	Signature   string
	Inputs      []abi.Argument
	OriginalABI *ABIEvent // Added: Original ABI event information
}

// Struct to parse the original ABI JSON to get component names
type ABIEvent struct {
	Name      string     `json:"name"`
	Type      string     `json:"type"`
	Anonymous bool       `json:"anonymous"`
	Inputs    []ABIInput `json:"inputs"`
}

type ABIInput struct {
	Name         string     `json:"name"`
	Type         string     `json:"type"`
	Indexed      bool       `json:"indexed"`
	Components   []ABIInput `json:"components,omitempty"`
	InternalType string     `json:"internalType,omitempty"`
}

// BuildABIJSONArray constructs a valid ABI JSON array from database records
func GetABIEventBySignatureHash(db *gorm.DB, signatureHash string) (*ABIEvent, error) {
	var record ABIEventRecord
	err := db.Where("event_signature_hash = ?", signatureHash).First(&record).Error
	if err != nil {
		return nil, err
	}

	item := []byte(record.ABIEventJSON)

	var abiItem struct {
		Name      string     `json:"name"`
		Type      string     `json:"type"`
		Anonymous bool       `json:"anonymous"`
		Inputs    []ABIInput `json:"inputs"`
	}
	if err := json.Unmarshal(item, &abiItem); err != nil {
		return nil, err
	}

	return &ABIEvent{
		Name:      abiItem.Name,
		Anonymous: abiItem.Anonymous,
		Type:      abiItem.Type,
		Inputs:    abiItem.Inputs,
	}, nil
}

// BuildEventSignature constructs the canonical event signature string
func BuildEventSignature(event ABIEvent) string {
	var params []string
	for _, input := range event.Inputs {
		params = append(params, ResolveType(input))
	}
	return fmt.Sprintf("%s(%s)", event.Name, strings.Join(params, ","))
}

// ResolveType recursively resolves Solidity types to their canonical form
func ResolveType(input ABIInput) string {
	// Handle arrays: "tuple[]" or "address[]", "tuple[][]" etc.
	arraySuffix := ""
	t := input.Type
	for strings.HasSuffix(t, "[]") {
		arraySuffix += "[]"
		t = t[:len(t)-2]
	}

	if t == "tuple" {
		var componentTypes []string
		for _, comp := range input.Components {
			componentTypes = append(componentTypes, ResolveType(comp))
		}
		return fmt.Sprintf("(%s)%s", strings.Join(componentTypes, ","), arraySuffix)
	}
	return input.Type
}

// Keccak256Hash generates the keccak256 hash of the input text
func Keccak256Hash(text string) string {
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write([]byte(text))
	return fmt.Sprintf("0x%x", hasher.Sum(nil))
}

// Parse the original ABI JSON to extract component information
func parseABIJSON(abiData []byte) ([]ABIEvent, error) {
	var abiArray []json.RawMessage
	if err := json.Unmarshal(abiData, &abiArray); err != nil {
		return nil, err
	}

	var events []ABIEvent
	for _, item := range abiArray {
		var entry map[string]any
		if err := json.Unmarshal(item, &entry); err != nil {
			continue
		}

		// Only process event type entries
		if entry["type"] == "event" {
			var abiItem struct {
				Name      string     `json:"name"`
				Type      string     `json:"type"`
				Anonymous bool       `json:"anonymous"`
				Inputs    []ABIInput `json:"inputs"`
			}
			if err := json.Unmarshal(item, &abiItem); err != nil {
				continue
			}

			events = append(events, ABIEvent{
				Name:      abiItem.Name,
				Anonymous: abiItem.Anonymous,
				Type:      abiItem.Type,
				Inputs:    abiItem.Inputs,
			})
		}
	}

	return events, nil
}

// loadEventSignaturesOnDB scans ABI files and stores event signatures in the database
func loadEventSignaturesOnDB(db *gorm.DB, abiDir string) error {
	var counter int

	err := filepath.Walk(abiDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error walking path %s: %w", path, err)
		}

		if info.IsDir() || !strings.HasSuffix(strings.ToLower(info.Name()), ".json") {
			return nil
		}

		abiData, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("error reading file %s: %w", path, err)
		}

		// Parse the original ABI JSON first to get component information
		abiEvents, err := parseABIJSON(abiData)
		if err != nil {
			fmt.Printf("Warning: Could not parse ABI JSON from %s: %v\n", path, err)
			return nil
		}

		for _, e := range abiEvents {
			counter++
			eventSignature := BuildEventSignature(e)

			// hash eventSignature
			signatureHash := Keccak256Hash(eventSignature)

			var record ABIEventRecord
			err := db.Where("event_signature_hash = ?", signatureHash).First(&record).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				eventJSON, err := json.Marshal(e)
				if err != nil {
					fmt.Println("Failed to Marshal: ", err)
					continue
				}
				newRecord := ABIEventRecord{
					EventSignatureHash: signatureHash,
					EventName:          e.Name,
					ABIEventJSON:       string(eventJSON),
				}
				if err := db.Create(&newRecord).Error; err != nil {
					fmt.Println("Failed to add DataBase: ", err)
					continue
				}

			} else if err != nil {
				fmt.Println("Failed to get from database: ", err)
				continue
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk ABI directory: %w", err)
	}

	fmt.Printf("Loaded %d event signatures from %s\n", counter, abiDir)

	return nil
}

func BuildABIJSONArray(records []ABIEventRecord) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteByte('[')

	for i, rec := range records {
		buffer.WriteString(rec.ABIEventJSON)
		if i < len(records)-1 {
			buffer.WriteByte(',')
		}
	}

	buffer.WriteByte(']')
	return buffer.Bytes(), nil
}

// Enhanced loadEventSignatures function that includes original ABI information
func loadEventSignatures(db *gorm.DB) (map[string]EventSignatureInfo, error) {
	eventSigs := make(map[string]EventSignatureInfo)

	var records []ABIEventRecord
	if err := db.Find(&records).Error; err != nil {
		log.Fatal("failed to load ABI events:", err)
	}

	abiData, err := BuildABIJSONArray(records)
	if err != nil {
		return nil, fmt.Errorf("failed to get recoreds from database: %w", err)
	}

	// Then parse with go-ethereum library for signature generation
	parsedABI, err := abi.JSON(strings.NewReader(string(abiData)))
	if err != nil {
		return nil, fmt.Errorf("failed to Parsed the abi")
	}

	for _, event := range parsedABI.Events {
		sigHash := event.ID.Hex()

		// Find the corresponding event in our parsed JSON
		var abiEvent *ABIEvent
		abiEvent, err := GetABIEventBySignatureHash(db, sigHash)
		if err != nil {
			fmt.Println("failed to get abi from database: %w", err)
			continue
		}

		var inputParams []string
		for _, input := range event.Inputs {
			inputParams = append(inputParams, input.Type.String())
		}

		eventSigs[sigHash] = EventSignatureInfo{
			Name:        event.Name,
			Signature:   fmt.Sprintf("%s(%s)", event.Name, strings.Join(inputParams, ",")),
			Inputs:      event.Inputs,
			OriginalABI: abiEvent, // Store the original ABI event information
		}

		fmt.Printf("Loaded event: %s with signature: %s\n", event.Name, sigHash)
	}

	return eventSigs, nil
}
