// Command sri prints the SRI sha384 hash for every pinned CDN script and flags
// any that drifted from the value recorded in internal/web/cdn.go. Run it after
// bumping a script version, then paste the new hash into CDNScripts.
package main

import (
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/shanehull/housecycle/internal/web"
)

func main() {
	client := &http.Client{}
	failed := false
	for _, s := range web.CDNScripts {
		hash, err := fetchHash(client, s.URL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", s.Name, err)
			failed = true
			continue
		}
		status := "ok"
		if hash != s.Integrity {
			status = "MISMATCH"
			failed = true
		}
		fmt.Printf("%-20s %s  %s\n", s.Name, hash, status)
	}
	if failed {
		os.Exit(1)
	}
}

func fetchHash(client *http.Client, url string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	sum := sha512.Sum384(body)
	return "sha384-" + base64.StdEncoding.EncodeToString(sum[:]), nil
}
