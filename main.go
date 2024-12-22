package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// VPG struct represents the VPG details returned by the Zerto API
type VPG struct {
	ActualRPO int `json:"ActualRPO"`
}

// Config struct holds the Zerto API credentials
type Config struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// TokenResponse struct represents the response from the token endpoint
type TokenResponse struct {
	AccessToken string `json:"access_token"`
}

func main() {
	// Accept command-line arguments for server IP and config file path
	serverIP := flag.String("server", "localhost", "Zerto server IP")
	configFile := flag.String("config", "", "Path to the config file")
	flag.Parse()

	if *configFile == "" {
		log.Fatal("Config file path is required")
	}

	// Read config file
	config, err := readConfig(*configFile)
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	// Create HTTP client with custom transport to skip TLS verification
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // Skip certificate validation
		},
	}

	// Authenticate with Zerto API and get access token
	accessToken, err := authenticate(client, *serverIP, config.Username, config.Password)
	if err != nil {
		log.Fatalf("Error authenticating with Zerto API: %v", err)
	}

	// Query VPGs from Zerto API using access token
	err = queryVPGs(client, *serverIP, accessToken)
	if err != nil {
		log.Fatalf("Error querying VPGs: %v", err)
	}
}

// readConfig reads the config file and returns the username and password
func readConfig(configFile string) (*Config, error) {
	bytes, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(bytes, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// authenticate handles logging into the Zerto API and returning an access token
func authenticate(client *http.Client, serverIP, username, password string) (string, error) {
	authURL := fmt.Sprintf("https://%s/auth/realms/zerto/protocol/openid-connect/token", serverIP)

	// Form data for authentication
	data := fmt.Sprintf("grant_type=password&client_id=zerto-client&username=%s&password=%s", username, password)
	req, err := http.NewRequest("POST", authURL, bytes.NewBufferString(data))
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("authentication failed, status code: %d, response: %s", resp.StatusCode, string(bodyBytes))
	}

	var tokenResponse TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return "", fmt.Errorf("error decoding token response: %v", err)
	}

	return tokenResponse.AccessToken, nil
}

// queryVPGs queries the VPGs and returns the average Actual RPO as an integer
func queryVPGs(client *http.Client, serverIP, accessToken string) error {
	apiURL := fmt.Sprintf("https://%s/v1/vpgs", serverIP)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error querying VPGs, status code: %d, response: %s", resp.StatusCode, string(body))
	}

	// Unmarshal the response into a slice of VPGs
	var vpgs []VPG
	if err := json.Unmarshal(body, &vpgs); err != nil {
		return fmt.Errorf("error unmarshalling JSON: %v", err)
	}

	// Calculate the average RPO
	totalRPO := 0
	for _, vpg := range vpgs {
		totalRPO += vpg.ActualRPO
	}

	if len(vpgs) > 0 {
		averageRPO := totalRPO / len(vpgs)
		fmt.Printf("Average RPO: %d seconds\n", averageRPO)
	} else {
		fmt.Println("No VPGs found.")
	}

	return nil
}
