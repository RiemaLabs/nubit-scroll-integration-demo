package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/cosmos/cosmos-sdk/server"
	"github.com/spf13/cobra"
)

var chainIDToSha256 = map[string]string{}

func downloadGenesisCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "download-genesis [chain-id]",
		Short: "Download genesis file from https://github.com/RiemaLabs/networks",
		Long: "Download genesis file from https://github.com/RiemaLabs/networks.\n" +
			"The first argument should be a known chain-id. Ex. nubit.\n" +
			"If no argument is provided, defaults to nubit.\n",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			chainID := getChainIDOrDefault(args)
			if !isKnownChainID(chainID) {
				return fmt.Errorf("unknown chain-id: %s. Must be: nubit", chainID)
			}
			outputFile := server.GetServerContextFromCmd(cmd).Config.GenesisFile()
			fmt.Printf("Downloading genesis file for %s to %s\n", chainID, outputFile)

			url := fmt.Sprintf("https://raw.githubusercontent.com/RiemaLabs/networks/master/%s/genesis.json", chainID)
			if err := downloadFile(outputFile, url); err != nil {
				return fmt.Errorf("error downloading / persisting the genesis file: %s", err)
			}
			fmt.Printf("Downloaded genesis file for %s to %s\n", chainID, outputFile)

			// Compute SHA-256 hash of the downloaded file
			hash, err := computeSha256(outputFile)
			if err != nil {
				return fmt.Errorf("error computing sha256 hash: %s", err)
			}

			// Compare computed hash against known hash
			knownHash, ok := chainIDToSha256[chainID]
			if !ok {
				return fmt.Errorf("unknown chain-id: %s", chainID)
			}

			if hash != knownHash {
				return fmt.Errorf("sha256 hash mismatch: got %s, expected %s", hash, knownHash)
			}

			fmt.Printf("SHA-256 hash verified for %s\n", chainID)
			return nil
		},
	}

	return cmd
}

// getChainIDOrDefault returns the chainID from the command line arguments. If
// none is provided, defaults to nubit (mainnet).
func getChainIDOrDefault(args []string) string {
	if len(args) == 1 {
		return args[0]
	}
	return "nubit"
}

// isKnownChainID returns true if the chainID is known.
func isKnownChainID(chainID string) bool {
	knownChainIDs := []string{
		"nubit", // mainnet
	}
	return contains(knownChainIDs, chainID)
}

// contains checks if a string is present in a slice.
func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// downloadFile will download a URL to a local file.
func downloadFile(filepath string, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// computeSha256 computes the SHA-256 hash of a file.
func computeSha256(filepath string) (string, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
