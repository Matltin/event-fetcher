package eventsdb

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
)

// connectWithRetry attempts to connect to the RPC endpoint with retries
func connectWithRetry(rpcURL string, maxRetries int, retryDelay time.Duration) (*ethclient.Client, error) {
	var client *ethclient.Client
	var err error

	for i := 0; i < maxRetries; i++ {
		log.Printf("Connection attempt %d to %s...\n", i+1, rpcURL)

		ctx, cancel := context.WithTimeout(context.Background(), DefaultConnectionTimeout)
		client, err = ethclient.DialContext(ctx, rpcURL)
		cancel()

		if err != nil {
			log.Printf("Dial failed on attempt %d: %v\n", i+1, err)
			if i < maxRetries-1 {
				fmt.Printf("Retrying in %v...\n", retryDelay)
				time.Sleep(retryDelay)
			}
			continue
		}

		log.Printf("Connection established, testing with HeaderByNumber...\n")
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		header, testErr := client.HeaderByNumber(ctx, nil)
		cancel()

		if testErr != nil {
			log.Printf("Connection test failed on attempt %d: %v\n", i+1, testErr)
			client.Close()
			err = testErr
			if i < maxRetries-1 {
				fmt.Printf("Retrying in %v...\n", retryDelay)
				time.Sleep(retryDelay)
			}
			continue
		}

		if header == nil {
			log.Printf("Connection test returned nil header on attempt %d\n", i+1)
			client.Close()
			err = fmt.Errorf("nil header returned")
			if i < maxRetries-1 {
				fmt.Printf("Retrying in %v...\n", retryDelay)
				time.Sleep(retryDelay)
			}
			continue
		}

		log.Printf("Successfully connected to RPC endpoint on attempt %d!\n", i+1)
		log.Printf("Current block number: %s\n", header.Number.String())
		return client, nil
	}

	return nil, fmt.Errorf("failed to connect after %d attempts: %w", maxRetries, err)
}
