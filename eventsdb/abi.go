package eventsdb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

// Enhanced EventSignatureInfo with original ABI information
type EventSignatureInfo struct {
	Name        string
	Signature   string
	Inputs      []abi.Argument
	ABI         abi.ABI
	OriginalABI *ABIEvent // Added: Original ABI event information
}

// Struct to parse the original ABI JSON to get component names
type ABIInput struct {
	Name         string     `json:"name"`
	Type         string     `json:"type"`
	Indexed      bool       `json:"indexed"`
	Components   []ABIInput `json:"components,omitempty"`
	InternalType string     `json:"internalType,omitempty"`
}

type ABIEvent struct {
	Name      string     `json:"name"`
	Type      string     `json:"type"`
	Anonymous bool       `json:"anonymous"`
	Inputs    []ABIInput `json:"inputs"`
}

// Parse the original ABI JSON to extract component information
func parseABIJSON(abiData []byte) ([]ABIEvent, error) {
	var abiArray []json.RawMessage
	if err := json.Unmarshal(abiData, &abiArray); err != nil {
		return nil, err
	}

	var events []ABIEvent
	for _, item := range abiArray {
		var entry map[string]interface{}
		if err := json.Unmarshal(item, &entry); err != nil {
			continue
		}

		if entry["type"] == "event" {
			var event ABIEvent
			if err := json.Unmarshal(item, &event); err != nil {
				continue
			}
			events = append(events, event)
		}
	}

	return events, nil
}

// Get flattened component names and types
func getFlattenedComponents(input ABIInput) map[string]string {
	components := make(map[string]string)

	if len(input.Components) > 0 {
		for _, comp := range input.Components {
			components[comp.Name] = comp.Type
			// Recursively handle nested components
			nestedComponents := getFlattenedComponents(comp)
			for name, typ := range nestedComponents {
				components[fmt.Sprintf("%s.%s", comp.Name, name)] = typ
			}
		}
	}

	return components
}

// Helper function to access specific nested component by path
func getComponentByPath(input ABIInput, path string) *ABIInput {
	parts := strings.Split(path, ".")
	current := &input

	for _, part := range parts {
		found := false
		for _, comp := range current.Components {
			if comp.Name == part {
				current = &comp
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}

	return current
}

// Enhanced loadEventSignatures function that includes original ABI information
func loadEventSignatures(abiDir string) (map[string]EventSignatureInfo, error) {
	eventSigs := make(map[string]EventSignatureInfo)

	err := filepath.Walk(abiDir, func(path string, info os.FileInfo, err error) error {
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

		// Then parse with go-ethereum library for signature generation
		parsedABI, err := abi.JSON(strings.NewReader(string(abiData)))
		if err != nil {
			fmt.Printf("Warning: Could not parse ABI file %s: %v\n", path, err)
			return nil
		}

		for _, event := range parsedABI.Events {
			sigHash := event.ID.Hex()

			// Find the corresponding event in our parsed JSON
			var abiEvent *ABIEvent
			for _, e := range abiEvents {
				if e.Name == event.Name {
					abiEvent = &e
					break
				}
			}

			var inputParams []string
			for _, input := range event.Inputs {
				inputParams = append(inputParams, input.Type.String())
			}

			eventSigs[sigHash] = EventSignatureInfo{
				Name:        event.Name,
				Signature:   fmt.Sprintf("%s(%s)", event.Name, strings.Join(inputParams, ",")),
				Inputs:      event.Inputs,
				ABI:         parsedABI,
				OriginalABI: abiEvent, // Store the original ABI event information
			}

			fmt.Printf("Loaded event: %s with signature: %s\n", event.Name, sigHash)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk ABI directory: %w", err)
	}

	fmt.Printf("Loaded %d event signatures from %s\n", len(eventSigs), abiDir)
	return eventSigs, nil
}
